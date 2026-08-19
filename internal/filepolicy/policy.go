// Package filepolicy defines the infrastructure-free repository discovery and
// bounded file-safety rules shared by scanner adapters and legacy analyzers.
package filepolicy

import (
	"bufio"
	"bytes"
	"context"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"
)

const (
	MaxFileSize   = 10 * 1024 * 1024
	MaxLineCount  = 100000
	MaxLineLength = 10000
)

var assetExtensions = map[string]bool{
	".ico": true, ".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".svg": true, ".woff": true, ".woff2": true,
	".ttf": true, ".eot": true, ".pdf": true,
}

var generatedDirectories = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".venv": true,
	"venv": true, "__pycache__": true, "dist": true, "build": true,
	"target": true, "bin": true,
}

// CheckAnalyzable verifies that a file is safe to read into memory and parse.
func CheckAnalyzable(fs billy.Filesystem, filePath string) (bool, string) {
	return CheckAnalyzableContext(context.Background(), fs, filePath)
}

// CheckAnalyzableContext verifies that a file is safe to read while allowing
// repository scans to stop promptly when their context is canceled.
func CheckAnalyzableContext(ctx context.Context, fs billy.Filesystem, filePath string) (bool, string) {
	if err := ctx.Err(); err != nil {
		return false, err.Error()
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	if assetExtensions[ext] {
		return false, "common binary or asset file"
	}

	info, err := fs.Stat(filePath)
	if err != nil {
		return false, "file missing or inaccessible"
	}
	if info.IsDir() {
		return false, "is a directory"
	}
	if info.Size() > MaxFileSize {
		return false, "file exceeds maximum size limit (10MB)"
	}

	file, err := fs.Open(filePath)
	if err != nil {
		return false, "failed to open file for peeking"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, MaxLineLength), MaxLineLength)
	lineCount := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return false, err.Error()
		}
		if bytes.IndexByte(scanner.Bytes(), 0) >= 0 {
			return false, "binary file"
		}
		lineCount++
		if lineCount > MaxLineCount {
			return false, "file exceeds maximum line count (100000)"
		}
	}
	if err := scanner.Err(); err == bufio.ErrTooLong {
		return false, "file contains severely minified or elongated lines"
	} else if err != nil {
		return false, "failed to read lines for validation"
	}
	return true, ""
}

// IsGeneratedDirectory reports whether a directory name is excluded from
// source discovery at any depth.
func IsGeneratedDirectory(name string) bool {
	return generatedDirectories[strings.ToLower(name)]
}

// IsGeneratedPath applies generated-directory exclusion to repository-relative
// paths from either slash convention.
func IsGeneratedPath(filePath string) bool {
	normalized := strings.ReplaceAll(filePath, "\\", "/")
	for _, component := range strings.Split(normalized, "/") {
		if IsGeneratedDirectory(component) {
			return true
		}
	}
	return false
}

// IsSilentSkipReason reports whether a skipped file is expected binary or
// generated content that does not need a user-facing warning.
func IsSilentSkipReason(reason string) bool {
	return reason == "common binary or asset file" || reason == "binary file" || reason == "is a directory"
}
