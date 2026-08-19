package analyzers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLineCounterCountsTransferredLanguageExtensions(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"main.rs",
		"Main.kt",
		"build.kts",
		"App.swift",
		"Main.scala",
		"analysis.R",
		"legacy.m",
		"legacy.mm",
		"run.sh",
		"run.bash",
		"run.zsh",
	}
	for _, name := range files {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("first\nsecond\n"), 0o600))
	}

	result, err := NewLineCounter().Analyze(context.Background(), &git.Repository{FS: osfs.New(root), Path: root})
	require.NoError(t, err)
	assert.EqualValues(t, len(files), result.Metrics["file_count"])
	assert.EqualValues(t, len(files)*2, result.Metrics["loc"])
}

func TestLineCounterExcludesGeneratedDirectories(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600))
	generatedDirectories := []string{
		".git", "node_modules", "vendor", ".venv", "venv", "__pycache__", "dist", "build", "target", "bin",
	}
	for _, directory := range generatedDirectories {
		path := filepath.Join(root, directory, "generated.go")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("package generated\n"), 0o600))
	}

	result, err := NewLineCounter().Analyze(context.Background(), &git.Repository{FS: osfs.New(root), Path: root})
	require.NoError(t, err)
	assert.EqualValues(t, 1, result.Metrics["file_count"])
	assert.EqualValues(t, 1, result.Metrics["loc"])
}
