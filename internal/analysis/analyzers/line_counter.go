package analyzers

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/analysis"
	"github.com/endrilickollari/debtdrone-cli/internal/git"
)

type LineCounter struct{}

func NewLineCounter() *LineCounter {
	return &LineCounter{}
}

func (a *LineCounter) Name() string {
	return "LineCounter"
}

func (a *LineCounter) Analyze(ctx context.Context, repo *git.Repository) (*analysis.Result, error) {
	var totalLines int64
	var fileCount int64

	err := filepath.Walk(repo.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !isCodeFile(ext) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Count(string(content), "\n")
		totalLines += int64(lines)
		fileCount++

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &analysis.Result{
		Issues: nil,
		Metrics: map[string]interface{}{
			"loc":        totalLines,
			"file_count": fileCount,
		},
	}, nil
}

func isCodeFile(ext string) bool {
	switch ext {
	case ".go", ".js", ".ts", ".tsx", ".jsx", ".py", ".java", ".cs", ".c", ".cpp", ".h", ".rb", ".php":
		return true
	default:
		return false
	}
}
