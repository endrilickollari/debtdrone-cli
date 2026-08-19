package analysis_test

import (
	"strings"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/analysis"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAnalyzable(t *testing.T) {
	fs := memfs.New()

	tests := []struct {
		name           string
		filePath       string
		content        []byte
		setupFS        func()
		expectedPass   bool
		expectedReason string
	}{
		{
			name:           "valid regular go file",
			filePath:       "main.go",
			content:        []byte("package main\n\nfunc main() {}\n"),
			expectedPass:   true,
			expectedReason: "",
		},
		{
			name:           "lock file remains analyzable",
			filePath:       "frontend/package-lock.json",
			content:        []byte("{ \"name\": \"test\" }"),
			expectedPass:   true,
			expectedReason: "",
		},
		{
			name:           "minified filename is allowed when content is bounded",
			filePath:       "public/bundle.min.js",
			content:        []byte("console.log('hi');\n"),
			expectedPass:   true,
			expectedReason: "",
		},
		{
			name:           "common asset extension",
			filePath:       "assets/logo.PNG",
			content:        []byte("not actually an image"),
			expectedPass:   false,
			expectedReason: "common binary or asset file",
		},
		{
			name:           "binary content",
			filePath:       "generated/data.bin",
			content:        []byte{'a', 0, 'b'},
			expectedPass:   false,
			expectedReason: "binary file",
		},
		{
			name:           "file exceeds maximum 10MB size",
			filePath:       "large_dump.sql",
			content:        make([]byte, 11*1024*1024),
			expectedPass:   false,
			expectedReason: "file exceeds maximum size limit (10MB)",
		},
		{
			name:           "file contains severely minified single line (10k+ chars)",
			filePath:       "bundle.js",
			content:        []byte("var a=1;" + strings.Repeat("b=2;", 5000) + "c=3;"),
			expectedPass:   false,
			expectedReason: "file contains severely minified or elongated lines",
		},
		{
			name:           "file exceeds 100,000 lines",
			filePath:       "huge.go",
			content:        []byte(strings.Repeat("var x int\n", 100001)),
			expectedPass:   false,
			expectedReason: "file exceeds maximum line count (100000)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := fs.Create(tt.filePath)
			require.NoError(t, err)

			_, err = file.Write(tt.content)
			require.NoError(t, err)
			file.Close()

			pass, reason := analysis.IsAnalyzableTest(fs, tt.filePath)

			assert.Equal(t, tt.expectedPass, pass)
			if !tt.expectedPass {
				assert.Equal(t, tt.expectedReason, reason)
			}
		})
	}
}

func TestGeneratedDirectoryPolicy(t *testing.T) {
	for _, name := range []string{".git", "node_modules", "vendor", ".venv", "venv", "__pycache__", "dist", "build", "target", "bin"} {
		assert.True(t, analysis.IsGeneratedDirectory(name), name)
	}
	assert.False(t, analysis.IsGeneratedDirectory("src"))
	assert.True(t, analysis.IsGeneratedPath(`frontend\node_modules\package\index.js`))
	assert.True(t, analysis.IsGeneratedPath("backend/target/release/app"))
	assert.False(t, analysis.IsGeneratedPath("src/binning/model.go"))
}
