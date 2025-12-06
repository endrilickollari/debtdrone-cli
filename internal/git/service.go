package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
		cloneOpts.Auth = &http.BasicAuth{
			Username: opts.Token,
			Password: "",
		}
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
