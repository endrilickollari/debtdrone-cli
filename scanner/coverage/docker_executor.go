package coverage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/filepolicy"
	"github.com/google/uuid"
)

const (
	defaultContainerMemoryBytes   int64 = 2 << 30
	defaultContainerCPUs                = 2.0
	defaultContainerPIDs                = 512
	defaultWorkspaceBytes         int64 = 512 << 20
	defaultWorkspaceFiles               = 100_000
	defaultExecutionTimeout             = 10 * time.Minute
	containerCleanupTimeout             = 15 * time.Second
	defaultWorkspaceCheckInterval       = 100 * time.Millisecond
)

// DockerImageResolver selects a prepared runtime image for one execution
// request. It may enforce an allowlist or replace the statically detected image.
type DockerImageResolver func(ExecutionRequest) (string, error)

// DockerCommandResolver selects the command run inside the container. Trusted
// consumers can use it to add dependency setup before the coverage command.
type DockerCommandResolver func(ExecutionRequest) ([]string, error)

// DockerExecutorOptions controls the built-in Docker CLI executor. Zero values
// select conservative defaults; the network remains disabled unless explicitly
// configured otherwise.
type DockerExecutorOptions struct {
	Binary            string
	MemoryBytes       int64
	CPUs              float64
	PIDsLimit         int
	NetworkMode       string
	Timeout           time.Duration
	MaxWorkspaceBytes int64
	MaxWorkspaceFiles int
	ContainerUser     string
	ResolveImage      DockerImageResolver
	ResolveCommand    DockerCommandResolver
}

// DockerExecutor runs a coverage command in a disposable, resource-bounded
// container. It uses existing images and never builds or removes shared images.
type DockerExecutor struct {
	options       DockerExecutorOptions
	runner        dockerCommandRunner
	newName       func() string
	checkInterval time.Duration
}

type dockerCommandRunner interface {
	Run(context.Context, string, ...string) error
}

type osDockerCommandRunner struct{}

func (osDockerCommandRunner) Run(ctx context.Context, binary string, args ...string) error {
	return exec.CommandContext(ctx, binary, args...).Run()
}

// NewDockerExecutor constructs an optional local Docker executor. Constructing
// it does not inspect Docker or start a container.
func NewDockerExecutor(options DockerExecutorOptions) (*DockerExecutor, error) {
	if options.Binary == "" {
		options.Binary = "docker"
	}
	if options.MemoryBytes == 0 {
		options.MemoryBytes = defaultContainerMemoryBytes
	}
	if options.CPUs == 0 {
		options.CPUs = defaultContainerCPUs
	}
	if options.PIDsLimit == 0 {
		options.PIDsLimit = defaultContainerPIDs
	}
	if options.NetworkMode == "" {
		options.NetworkMode = "none"
	}
	if options.Timeout == 0 {
		options.Timeout = defaultExecutionTimeout
	}
	if options.MaxWorkspaceBytes == 0 {
		options.MaxWorkspaceBytes = defaultWorkspaceBytes
	}
	if options.MaxWorkspaceFiles == 0 {
		options.MaxWorkspaceFiles = defaultWorkspaceFiles
	}
	if options.ContainerUser == "" && runtime.GOOS != "windows" {
		current, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("resolve Docker container user: %w", err)
		}
		if _, err := strconv.ParseUint(current.Uid, 10, 32); err != nil {
			return nil, fmt.Errorf("resolve Docker container UID: %w", err)
		}
		if _, err := strconv.ParseUint(current.Gid, 10, 32); err != nil {
			return nil, fmt.Errorf("resolve Docker container GID: %w", err)
		}
		options.ContainerUser = current.Uid + ":" + current.Gid
	}
	if options.MemoryBytes < 1 || options.CPUs <= 0 || options.PIDsLimit < 1 || options.Timeout < 1 || options.MaxWorkspaceBytes < 1 || options.MaxWorkspaceFiles < 1 {
		return nil, fmt.Errorf("Docker executor limits must be positive")
	}
	if strings.HasPrefix(options.Binary, "-") || invalidDockerOption(options.NetworkMode) || invalidDockerOption(options.ContainerUser) {
		return nil, fmt.Errorf("Docker executor contains an invalid command option")
	}
	return &DockerExecutor{
		options:       options,
		runner:        osDockerCommandRunner{},
		newName:       func() string { return "debtdrone-coverage-" + uuid.NewString() },
		checkInterval: defaultWorkspaceCheckInterval,
	}, nil
}

func invalidDockerOption(value string) bool {
	return strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\r\n\x00")
}

// Execute copies the build root into a bounded temporary workspace, executes
// the requested command, collects only the requested artifacts, and removes
// the workspace and container on every return path.
func (executor *DockerExecutor) Execute(ctx context.Context, request ExecutionRequest) (artifacts []Artifact, returnErr error) {
	if executor == nil || executor.runner == nil || executor.newName == nil {
		return nil, fmt.Errorf("Docker executor must be created with NewDockerExecutor")
	}
	if executor.options.ResolveImage != nil {
		image, err := executor.options.ResolveImage(request)
		if err != nil {
			return nil, fmt.Errorf("resolve isolated coverage image: %w", err)
		}
		request.SuggestedImage = image
	}
	if executor.options.ResolveCommand != nil {
		command, err := executor.options.ResolveCommand(request)
		if err != nil {
			return nil, fmt.Errorf("resolve isolated coverage command: %w", err)
		}
		request.Command = append([]string(nil), command...)
	}
	if err := validateExecutionRequest(request); err != nil {
		return nil, err
	}
	workspace, err := os.MkdirTemp("", "debtdrone-coverage-workspace-*")
	if err != nil {
		return nil, fmt.Errorf("create isolated coverage workspace: %w", err)
	}
	containerName := ""
	defer func() {
		var cleanupErrors []error
		if containerName != "" {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), containerCleanupTimeout)
			if err := executor.runner.Run(cleanupCtx, executor.options.Binary, "rm", "-f", containerName); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("remove isolated coverage container %s: %w", containerName, err))
			}
			cancel()
		}
		if err := removeWorkspace(workspace); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove isolated coverage workspace: %w", err))
		}
		returnErr = errors.Join(returnErr, errors.Join(cleanupErrors...))
	}()
	if err := copyWorkspace(ctx, request.SourceDir, workspace, executor.options.MaxWorkspaceBytes, executor.options.MaxWorkspaceFiles); err != nil {
		return nil, fmt.Errorf("prepare isolated coverage workspace: %w", err)
	}

	containerName = executor.newName()

	runCtx, cancel := context.WithTimeout(ctx, executor.options.Timeout)
	defer cancel()
	args := []string{
		"run",
		"--name", containerName,
		"--memory", strconv.FormatInt(executor.options.MemoryBytes, 10),
		"--cpus", strconv.FormatFloat(executor.options.CPUs, 'f', -1, 64),
		"--pids-limit", strconv.Itoa(executor.options.PIDsLimit),
		"--network", executor.options.NetworkMode,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--ulimit", fmt.Sprintf("fsize=%d:%d", executor.options.MaxWorkspaceBytes, executor.options.MaxWorkspaceBytes),
		"--mount", "type=bind,src=" + workspace + ",dst=/workspace",
		"--workdir", "/workspace",
	}
	if executor.options.ContainerUser != "" {
		args = append(args, "--user", executor.options.ContainerUser, "--env", "HOME=/tmp")
	}
	args = append(args, request.SuggestedImage)
	args = append(args, request.Command...)
	limitErrors := make(chan error, 1)
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		if err := monitorWorkspace(runCtx, workspace, executor.options.MaxWorkspaceBytes, executor.options.MaxWorkspaceFiles, executor.checkInterval); err != nil {
			limitErrors <- err
			cancel()
		}
	}()
	runErr := executor.runner.Run(runCtx, executor.options.Binary, args...)
	cancel()
	<-monitorDone
	select {
	case limitErr := <-limitErrors:
		return nil, limitErr
	default:
	}
	if err := checkWorkspaceLimits(context.Background(), workspace, executor.options.MaxWorkspaceBytes, executor.options.MaxWorkspaceFiles); err != nil {
		return nil, err
	}
	if runErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("isolated coverage exceeded %s: %w", executor.options.Timeout, context.DeadlineExceeded)
		}
		return nil, fmt.Errorf("run isolated coverage container: %w", runErr)
	}
	return collectArtifacts(workspace, request.ArtifactPaths)
}

func validateExecutionRequest(request ExecutionRequest) error {
	if strings.TrimSpace(request.SourceDir) == "" {
		return fmt.Errorf("isolated coverage source directory is required")
	}
	if strings.TrimSpace(request.SuggestedImage) == "" || strings.HasPrefix(request.SuggestedImage, "-") || strings.ContainsAny(request.SuggestedImage, "\r\n\x00") {
		return fmt.Errorf("isolated coverage image is invalid")
	}
	if len(request.Command) == 0 || strings.TrimSpace(request.Command[0]) == "" {
		return fmt.Errorf("isolated coverage command is required")
	}
	for _, artifactPath := range request.ArtifactPaths {
		if !safeRelativeArtifactPath(artifactPath) {
			return fmt.Errorf("coverage artifact path %q must remain inside the workspace", artifactPath)
		}
	}
	return nil
}

func safeRelativeArtifactPath(path string) bool {
	normalized := strings.ReplaceAll(path, "\\", "/")
	if normalized == "" || filepath.IsAbs(filepath.FromSlash(normalized)) || isWindowsAbsolutePath(normalized) || strings.HasPrefix(normalized, "//") {
		return false
	}
	cleaned := filepath.Clean(filepath.FromSlash(normalized))
	return cleaned != "." && cleaned != ".." && !strings.HasPrefix(cleaned, ".."+string(filepath.Separator))
}

func monitorWorkspace(ctx context.Context, workspace string, maxBytes int64, maxFiles int, interval time.Duration) error {
	if err := checkWorkspaceLimits(ctx, workspace, maxBytes, maxFiles); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := checkWorkspaceLimits(ctx, workspace, maxBytes, maxFiles); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

func checkWorkspaceLimits(ctx context.Context, workspace string, maxBytes int64, maxFiles int) error {
	var totalBytes int64
	var totalFiles int
	err := filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == workspace {
			return nil
		}
		totalFiles++
		if totalFiles > maxFiles {
			return fmt.Errorf("isolated coverage workspace exceeds the %d-entry limit", maxFiles)
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if totalBytes > maxBytes || info.Size() > maxBytes-totalBytes {
			return fmt.Errorf("isolated coverage workspace exceeds the %d-byte limit", maxBytes)
		}
		totalBytes += info.Size()
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func copyWorkspace(ctx context.Context, source, destination string, maxBytes int64, maxFiles int) error {
	absoluteSource, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	absoluteSource, err = filepath.EvalSymlinks(absoluteSource)
	if err != nil {
		return err
	}
	info, err := os.Stat(absoluteSource)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source is not a directory")
	}
	var copiedBytes int64
	var copiedFiles int
	return filepath.WalkDir(absoluteSource, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(absoluteSource, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() && filepolicy.IsGeneratedDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		copiedFiles++
		if copiedFiles > maxFiles {
			return fmt.Errorf("workspace exceeds the configured entry limit")
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		fileInfo, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !fileInfo.Mode().IsRegular() {
			return nil
		}
		if fileInfo.Size() > maxArtifactSize || copiedBytes > maxBytes || fileInfo.Size() > maxBytes-copiedBytes {
			return fmt.Errorf("workspace exceeds the configured copy limit")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		copied, err := copyRegularFile(path, target, fileInfo)
		if err != nil {
			return err
		}
		copiedBytes += copied
		return nil
	})
}

func removeWorkspace(workspace string) error {
	permissionErr := makeTreeRemovable(workspace)
	removeErr := os.RemoveAll(workspace)
	if removeErr == nil {
		return nil
	}
	return errors.Join(permissionErr, removeErr)
}

func makeTreeRemovable(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if !info.IsDir() {
		return os.Chmod(path, 0o600)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	var cleanupErrors []error
	for _, entry := range entries {
		if err := makeTreeRemovable(filepath.Join(path, entry.Name())); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func copyRegularFile(source, destination string, expected fs.FileInfo) (int64, error) {
	input, err := os.Open(source)
	if err != nil {
		return 0, err
	}
	defer input.Close()
	opened, err := input.Stat()
	if err != nil {
		return 0, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(expected, opened) {
		return 0, fmt.Errorf("source file changed while preparing the workspace")
	}
	outputMode := fs.FileMode(0o600)
	if opened.Mode().Perm()&0o111 != 0 {
		outputMode = 0o700
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, outputMode)
	if err != nil {
		return 0, err
	}
	copied, copyErr := io.Copy(output, io.LimitReader(input, maxArtifactSize+1))
	closeErr := output.Close()
	if copyErr != nil {
		return copied, copyErr
	}
	if copied > maxArtifactSize || copied != opened.Size() {
		return copied, fmt.Errorf("source file changed while preparing the workspace")
	}
	return copied, closeErr
}

func collectArtifacts(workspace string, paths []string) ([]Artifact, error) {
	artifacts := make([]Artifact, 0)
	for _, relative := range paths {
		content, err := readArtifact(filepath.Join(workspace, filepath.FromSlash(relative)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("collect coverage artifact %s: %w", relative, err)
		}
		artifacts = append(artifacts, Artifact{Name: filepath.ToSlash(relative), Content: content})
	}
	return artifacts, nil
}
