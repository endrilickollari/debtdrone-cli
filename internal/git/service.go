package git

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage"
	"github.com/go-git/go-git/v5/storage/memory"
)

type Service struct {
}

func NewService() *Service {
	return &Service{}
}
func (s *Service) OpenLocal(path string) (*Repository, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("path does not exist: %s", path)
	}

	return &Repository{
		FS:   osfs.New(path),
		Path: path,
	}, nil
}

type Repository struct {
	FS   billy.Filesystem
	Path string
}
type CloneOptions struct {
	URL          string
	Branch       string // Defaults to HEAD if empty
	Token        string // Optional auth token
	UseInMemory  bool   // If true, clones to memory
	SingleBranch bool   // If true, clones only the specified branch (or HEAD)
	Depth        int    // 0 for full history, >0 for shallow clone
}

func (s *Service) Clone(ctx context.Context, opts CloneOptions) (*Repository, error) {
	var storer storage.Storer
	var fs billy.Filesystem
	var path string
	var err error

	if opts.UseInMemory {
		storer = memory.NewStorage()
		fs = memfs.New()
	} else {
		// Check disk space before cloning to filesystem
		if err := CheckDiskSpace(os.TempDir()); err != nil {
			log.Printf("❌ [GitService] Disk space check failed: %v", err)
			return nil, err
		}

		path, err = os.MkdirTemp("", "debtdrone-repo-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir: %w", err)
		}
		fs = osfs.New(path)
	}

	cloneOpts := &git.CloneOptions{
		URL:      opts.URL,
		Progress: nil,
		Tags:     git.NoTags,
	}

	if opts.Depth > 0 {
		cloneOpts.Depth = opts.Depth
	}

	if opts.SingleBranch {
		cloneOpts.SingleBranch = true
	}

	if opts.Branch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(opts.Branch)
	}

	if opts.Token != "" {
		log.Printf("🔐 [GitService] Cloning with auth token (len: %d)", len(opts.Token))
		cloneOpts.Auth = &http.BasicAuth{
			Username: "oauth2",   // Use "oauth2" or generic username
			Password: opts.Token, // Token as password is often more reliable
		}
	} else {
		log.Printf("⚠️ [GitService] Cloning WITHOUT auth token")
	}

	if opts.UseInMemory {
		_, err = git.Clone(storer, fs, cloneOpts)
	} else {
		_, err = git.PlainClone(path, false, cloneOpts)
	}

	if err != nil {
		if path != "" {
			os.RemoveAll(path)
		}
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	return &Repository{
		FS:   fs,
		Path: path,
	}, nil
}

func (r *Repository) Cleanup() error {
	if r.Path != "" {
		return os.RemoveAll(r.Path)
	}
	return nil
}

func (r *Repository) GetSizeMB() (float64, error) {
	if r.Path == "" {
		return 0, nil
	}

	var size int64
	err := filepath.Walk(r.Path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return float64(size) / (1024 * 1024), nil
}

func (s *Service) GetFileChurn(ctx context.Context, path string, days int) (map[string]int, error) {
	since := fmt.Sprintf("%d days ago", days)
	cmd := exec.CommandContext(ctx, "git", "-C", path, "log", "--name-only", "--since", since, "--format=")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git log: %w", err)
	}

	churnMap := make(map[string]int)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			churnMap[trimmed]++
		}
	}
	return churnMap, nil
}

func (s *Service) GetCurrentCommitHash(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current commit hash: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (s *Service) GetChangedFiles(ctx context.Context, repoPath, oldCommit, newCommit string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "diff", "--name-only", oldCommit, newCommit)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	var files []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			files = append(files, trimmed)
		}
	}
	return files, nil
}

type CommitContext struct {
	Hash          string
	AuthorName    string
	AuthorEmail   string
	Subject       string
	CommitterName string // Optional
	Date          string // Optional
}

func (s *Service) GetCommitMetadata(ctx context.Context, repoPath string, hash string) (*CommitContext, error) {
	// If hash is empty, default to HEAD
	if hash == "" {
		hash = "HEAD"
	}

	// Format: Hash|AuthorName|AuthorEmail|Subject
	format := "%H|%an|%ae|%s"
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "log", "-1", fmt.Sprintf("--format=%s", format), hash)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit metadata: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) < 4 {
		return nil, fmt.Errorf("unexpected git log output format: %s", string(output))
	}

	return &CommitContext{
		Hash:        parts[0],
		AuthorName:  parts[1],
		AuthorEmail: parts[2],
		Subject:     parts[3],
	}, nil
}
