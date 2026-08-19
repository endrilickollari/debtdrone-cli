package analyzers

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/filepolicy"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/scancore"
)

type LineCounter struct{}

func NewLineCounter() *LineCounter {
	return &LineCounter{}
}

func (a *LineCounter) Name() string {
	return "LineCounter"
}

func (a *LineCounter) Analyze(ctx context.Context, repo *git.Repository) (*scancore.Result, error) {
	var totalLines int64
	var fileCount int64
	var targetFiles map[string]struct{}
	if files, _, ok := scancore.TargetFiles(ctx); ok {
		targetFiles = make(map[string]struct{}, len(files))
		for _, file := range files {
			targetFiles[file] = struct{}{}
		}
	}

	err := filepath.Walk(repo.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if filepolicy.IsGeneratedDirectory(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if targetFiles != nil {
			relative, relErr := filepath.Rel(repo.Path, path)
			if relErr != nil {
				return relErr
			}
			relative = "/" + strings.TrimPrefix(filepath.ToSlash(relative), "/")
			if _, included := targetFiles[relative]; !included {
				return nil
			}
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

	return &scancore.Result{
		Issues: nil,
		Metrics: map[string]interface{}{
			"loc":        totalLines,
			"file_count": fileCount,
		},
	}, nil
}

func isCodeFile(ext string) bool {
	switch ext {
	case ".go", ".js", ".ts", ".tsx", ".jsx", ".py", ".java", ".cs", ".c", ".cpp", ".h", ".rb", ".php",
		".rs", ".kt", ".kts", ".swift", ".scala", ".r", ".m", ".mm", ".sh", ".bash", ".zsh":
		return true
	default:
		return false
	}
}
