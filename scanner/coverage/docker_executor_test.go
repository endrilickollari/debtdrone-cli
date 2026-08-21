package coverage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedDockerCall struct {
	binary string
	args   []string
}

type fakeDockerRunner struct {
	calls []recordedDockerCall
	run   func(context.Context, []string) error
}

func (runner *fakeDockerRunner) Run(ctx context.Context, binary string, args ...string) error {
	runner.calls = append(runner.calls, recordedDockerCall{binary: binary, args: append([]string(nil), args...)})
	if runner.run != nil {
		return runner.run(ctx, args)
	}
	return nil
}

func TestDockerExecutorBoundsExecutionCollectsArtifactsAndCleansUp(t *testing.T) {
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "go.mod"), []byte("module example.com/test\n"), 0o600))
	runner := &fakeDockerRunner{}
	var workspace string
	runner.run = func(_ context.Context, args []string) error {
		if args[0] != "run" {
			return nil
		}
		workspace = mountSource(t, args)
		require.FileExists(t, filepath.Join(workspace, "go.mod"))
		return os.WriteFile(filepath.Join(workspace, "coverage.out"), []byte("mode: atomic\nexample.com/test/main.go:1.1,1.5 1 1\n"), 0o600)
	}
	executor, err := NewDockerExecutor(DockerExecutorOptions{ContainerUser: "123:456"})
	require.NoError(t, err)
	executor.runner = runner
	executor.newName = func() string { return "debtdrone-coverage-test" }

	artifacts, err := executor.Execute(context.Background(), ExecutionRequest{
		SourceDir: source, SuggestedImage: "golang:1.25-alpine",
		Command:       []string{"go", "test", "-coverprofile=coverage.out", "./..."},
		ArtifactPaths: []string{"coverage.out"},
	})
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	assert.Equal(t, "coverage.out", artifacts[0].Name)
	require.Len(t, runner.calls, 2)
	runArgs := runner.calls[0].args
	assert.Equal(t, "run", runArgs[0])
	assertContainsPair(t, runArgs, "--memory", "2147483648")
	assertContainsPair(t, runArgs, "--cpus", "2")
	assertContainsPair(t, runArgs, "--pids-limit", "512")
	assertContainsPair(t, runArgs, "--network", "none")
	assertContainsPair(t, runArgs, "--cap-drop", "ALL")
	assertContainsPair(t, runArgs, "--security-opt", "no-new-privileges")
	assertContainsPair(t, runArgs, "--ulimit", "fsize=536870912:536870912")
	assertContainsPair(t, runArgs, "--user", "123:456")
	assertContainsPair(t, runArgs, "--env", "HOME=/tmp")
	assertContainsPair(t, runArgs, "--name", "debtdrone-coverage-test")
	assert.NotContains(t, runArgs, "--rm")
	assert.Equal(t, []string{"rm", "-f", "debtdrone-coverage-test"}, runner.calls[1].args)
	assert.NoFileExists(t, workspace)
	assert.NoFileExists(t, filepath.Join(source, "coverage.out"))
	for _, call := range runner.calls {
		assert.NotEqual(t, "image", call.args[0], "the executor must not remove shared images")
	}
}

func TestDockerExecutorCleansContainerAfterCommandFailure(t *testing.T) {
	executor, err := NewDockerExecutor(DockerExecutorOptions{})
	require.NoError(t, err)
	runner := &fakeDockerRunner{run: func(_ context.Context, args []string) error {
		if args[0] == "run" {
			return errors.New("exit 1")
		}
		return nil
	}}
	executor.runner = runner
	executor.newName = func() string { return "debtdrone-coverage-failed" }

	_, err = executor.Execute(context.Background(), ExecutionRequest{
		SourceDir: t.TempDir(), SuggestedImage: "golang:1.25-alpine", Command: []string{"go", "test"},
	})
	require.ErrorContains(t, err, "exit 1")
	require.Len(t, runner.calls, 2)
	assert.Equal(t, []string{"rm", "-f", "debtdrone-coverage-failed"}, runner.calls[1].args)
}

func TestDockerExecutorCancellationStopsExecutionAndCleansContainer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	executor, err := NewDockerExecutor(DockerExecutorOptions{Timeout: time.Minute})
	require.NoError(t, err)
	runner := &fakeDockerRunner{run: func(runCtx context.Context, args []string) error {
		if args[0] == "run" {
			cancel()
			<-runCtx.Done()
			return runCtx.Err()
		}
		return nil
	}}
	executor.runner = runner
	executor.newName = func() string { return "debtdrone-coverage-canceled" }

	_, err = executor.Execute(ctx, ExecutionRequest{
		SourceDir: t.TempDir(), SuggestedImage: "golang:1.25-alpine", Command: []string{"go", "test"},
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, runner.calls, 2)
	assert.Equal(t, []string{"rm", "-f", "debtdrone-coverage-canceled"}, runner.calls[1].args)
}

func TestDockerExecutorTimeoutStopsExecutionAndCleansContainer(t *testing.T) {
	executor, err := NewDockerExecutor(DockerExecutorOptions{Timeout: 5 * time.Millisecond})
	require.NoError(t, err)
	runner := &fakeDockerRunner{run: func(runCtx context.Context, args []string) error {
		if args[0] == "run" {
			<-runCtx.Done()
			return runCtx.Err()
		}
		return nil
	}}
	executor.runner = runner
	executor.newName = func() string { return "debtdrone-coverage-timeout" }

	_, err = executor.Execute(context.Background(), ExecutionRequest{
		SourceDir: t.TempDir(), SuggestedImage: "golang:1.25-alpine", Command: []string{"go", "test"},
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Len(t, runner.calls, 2)
	assert.Equal(t, []string{"rm", "-f", "debtdrone-coverage-timeout"}, runner.calls[1].args)
}

func TestDockerExecutorRejectsArtifactTraversal(t *testing.T) {
	executor, err := NewDockerExecutor(DockerExecutorOptions{})
	require.NoError(t, err)
	_, err = executor.Execute(context.Background(), ExecutionRequest{
		SourceDir: t.TempDir(), SuggestedImage: "golang:1.25-alpine", Command: []string{"go", "test"}, ArtifactPaths: []string{"../secret"},
	})
	require.ErrorContains(t, err, "must remain inside")
}

func TestDockerExecutorRejectsCrossPlatformArtifactTraversal(t *testing.T) {
	executor, err := NewDockerExecutor(DockerExecutorOptions{})
	require.NoError(t, err)
	_, err = executor.Execute(context.Background(), ExecutionRequest{
		SourceDir: t.TempDir(), SuggestedImage: "golang:1.25-alpine", Command: []string{"go", "test"}, ArtifactPaths: []string{`..\secret`},
	})
	require.ErrorContains(t, err, "must remain inside")
}

func TestDockerExecutorRequiresConstructor(t *testing.T) {
	_, err := (&DockerExecutor{}).Execute(context.Background(), ExecutionRequest{})
	require.ErrorContains(t, err, "NewDockerExecutor")
}

func TestDockerExecutorStopsWhenWorkspaceOutputExceedsLimit(t *testing.T) {
	executor, err := NewDockerExecutor(DockerExecutorOptions{MaxWorkspaceBytes: 16, MaxWorkspaceFiles: 10})
	require.NoError(t, err)
	runner := &fakeDockerRunner{run: func(runCtx context.Context, args []string) error {
		if args[0] != "run" {
			return nil
		}
		workspace := mountSource(t, args)
		require.NoError(t, os.WriteFile(filepath.Join(workspace, "oversized.out"), make([]byte, 32), 0o600))
		<-runCtx.Done()
		return runCtx.Err()
	}}
	executor.runner = runner
	executor.newName = func() string { return "debtdrone-coverage-limited" }
	executor.checkInterval = time.Millisecond

	_, err = executor.Execute(context.Background(), ExecutionRequest{
		SourceDir: t.TempDir(), SuggestedImage: "prepared:latest", Command: []string{"coverage"},
	})
	require.ErrorContains(t, err, "16-byte limit")
	require.Len(t, runner.calls, 2)
	assert.Equal(t, []string{"rm", "-f", "debtdrone-coverage-limited"}, runner.calls[1].args)
}

func TestDockerExecutorStopsWhenWorkspaceEntryCountExceedsLimit(t *testing.T) {
	executor, err := NewDockerExecutor(DockerExecutorOptions{MaxWorkspaceBytes: 1024, MaxWorkspaceFiles: 1})
	require.NoError(t, err)
	runner := &fakeDockerRunner{run: func(runCtx context.Context, args []string) error {
		if args[0] != "run" {
			return nil
		}
		workspace := mountSource(t, args)
		require.NoError(t, os.WriteFile(filepath.Join(workspace, "one"), nil, 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(workspace, "two"), nil, 0o600))
		<-runCtx.Done()
		return runCtx.Err()
	}}
	executor.runner = runner
	executor.newName = func() string { return "debtdrone-coverage-entry-limited" }
	executor.checkInterval = time.Millisecond

	_, err = executor.Execute(context.Background(), ExecutionRequest{
		SourceDir: t.TempDir(), SuggestedImage: "prepared:latest", Command: []string{"coverage"},
	})
	require.ErrorContains(t, err, "1-entry limit")
}

func TestDockerExecutorReportsContainerCleanupFailure(t *testing.T) {
	executor, err := NewDockerExecutor(DockerExecutorOptions{})
	require.NoError(t, err)
	runner := &fakeDockerRunner{run: func(_ context.Context, args []string) error {
		if args[0] == "rm" {
			return errors.New("daemon unavailable")
		}
		workspace := mountSource(t, args)
		return os.WriteFile(filepath.Join(workspace, "coverage.out"), []byte("mode: atomic\n"), 0o600)
	}}
	executor.runner = runner
	executor.newName = func() string { return "debtdrone-coverage-cleanup" }

	artifacts, err := executor.Execute(context.Background(), ExecutionRequest{
		SourceDir: t.TempDir(), SuggestedImage: "prepared:latest", Command: []string{"coverage"}, ArtifactPaths: []string{"coverage.out"},
	})
	require.Len(t, artifacts, 1)
	require.ErrorContains(t, err, "remove isolated coverage container")
	require.ErrorContains(t, err, "daemon unavailable")
}

func TestDockerExecutorRemovesPermissionRestrictedWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission behavior is not available on Windows")
	}
	executor, err := NewDockerExecutor(DockerExecutorOptions{})
	require.NoError(t, err)
	var workspace string
	runner := &fakeDockerRunner{run: func(_ context.Context, args []string) error {
		if args[0] != "run" {
			return nil
		}
		workspace = mountSource(t, args)
		locked := filepath.Join(workspace, "locked")
		require.NoError(t, os.Mkdir(locked, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(locked, "output"), []byte("data"), 0o600))
		require.NoError(t, os.Chmod(locked, 0))
		return errors.New("test failed")
	}}
	executor.runner = runner
	executor.newName = func() string { return "debtdrone-coverage-permissions" }

	_, err = executor.Execute(context.Background(), ExecutionRequest{
		SourceDir: t.TempDir(), SuggestedImage: "prepared:latest", Command: []string{"coverage"},
	})
	require.Error(t, err)
	assert.NoDirExists(t, workspace)
}

func TestDockerExecutorCanResolvePreparedImage(t *testing.T) {
	executor, err := NewDockerExecutor(DockerExecutorOptions{
		ResolveImage: func(request ExecutionRequest) (string, error) {
			assert.Equal(t, "Python", request.Language)
			return "registry.example/coverage-python:3.12", nil
		},
		ResolveCommand: func(request ExecutionRequest) ([]string, error) {
			assert.Equal(t, []string{"pytest"}, request.Command)
			return []string{"sh", "-lc", "pip install -r requirements.txt pytest-cov && python -m pytest --cov=."}, nil
		},
	})
	require.NoError(t, err)
	runner := &fakeDockerRunner{}
	executor.runner = runner
	executor.newName = func() string { return "debtdrone-coverage-image" }

	_, err = executor.Execute(context.Background(), ExecutionRequest{
		SourceDir: t.TempDir(), Language: "Python", Command: []string{"pytest"},
	})
	require.NoError(t, err)
	require.Len(t, runner.calls, 2)
	assert.Contains(t, runner.calls[0].args, "registry.example/coverage-python:3.12")
	assert.Contains(t, runner.calls[0].args, "pip install -r requirements.txt pytest-cov && python -m pytest --cov=.")
}

func mountSource(t *testing.T, args []string) string {
	t.Helper()
	for index := range args {
		if args[index] != "--mount" || index+1 >= len(args) {
			continue
		}
		for _, part := range strings.Split(args[index+1], ",") {
			if strings.HasPrefix(part, "src=") {
				return strings.TrimPrefix(part, "src=")
			}
		}
	}
	t.Fatal("Docker mount source was not provided")
	return ""
}

func assertContainsPair(t *testing.T, values []string, key, value string) {
	t.Helper()
	for index := 0; index+1 < len(values); index++ {
		if values[index] == key && values[index+1] == value {
			return
		}
	}
	assert.Fail(t, "argument pair not found", "%s %s was not present in %v", key, value, values)
}
