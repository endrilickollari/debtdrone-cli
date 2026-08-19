package analysis

import (
	"context"
	"fmt"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/filepolicy"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/go-git/go-billy/v5"
)

const (
	MaxFileSize   = filepolicy.MaxFileSize
	MaxLineCount  = filepolicy.MaxLineCount
	MaxLineLength = filepolicy.MaxLineLength
)

func executeAnalyzerSafe(ctx context.Context, analyzer Analyzer, repo *git.Repository) (result *Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in analyzer %s: %v", analyzer.Name(), r)
		}
	}()

	result, err = analyzer.Analyze(ctx, repo)
	return
}

func isAnalyzable(fs billy.Filesystem, filePath string) (bool, string) {
	return filepolicy.CheckAnalyzable(fs, filePath)
}

// CheckAnalyzable exposes the scanner's bounded file-safety policy to the
// public package without exporting implementation details outside the module.
func CheckAnalyzable(fs billy.Filesystem, filePath string) (bool, string) {
	return isAnalyzable(fs, filePath)
}

// IsGeneratedDirectory reports whether a directory is excluded from source
// discovery. Matching by base name applies consistently at any nesting depth.
func IsGeneratedDirectory(name string) bool {
	return filepolicy.IsGeneratedDirectory(name)
}

// IsGeneratedPath reports whether any component is a generated directory.
func IsGeneratedPath(filePath string) bool {
	return filepolicy.IsGeneratedPath(filePath)
}

// IsSilentSkipReason identifies expected assets and directories that should be
// ignored without presenting a warning to scanner consumers.
func IsSilentSkipReason(reason string) bool {
	return filepolicy.IsSilentSkipReason(reason)
}
