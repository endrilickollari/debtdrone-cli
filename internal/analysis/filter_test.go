package analysis_test

import (
	"strings"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/internal/analysis"
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
			name:           "exact auto-generated file package-lock.json (now allowed in SaaS)",
			filePath:       "frontend/package-lock.json",
			content:        []byte("{ \"name\": \"test\" }"),
			expectedPass:   true,
			expectedReason: "",
		},
		{
			name:           "auto-generated extension .min.js (now allowed in SaaS)",
			filePath:       "public/bundle.min.js",
			content:        []byte("console.log('hi');\n"),
			expectedPass:   true,
			expectedReason: "",
		},
		{
			name:     "file exceeds maximum 10MB size",
			filePath: "large_dump.sql",
			// Let's create an 11MB file to trigger the limit
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
			name:     "file exceeds 100,000 lines",
			filePath: "huge.go",
			// write 100,001 lines
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
