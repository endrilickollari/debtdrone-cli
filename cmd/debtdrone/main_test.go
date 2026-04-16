package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// executeCommand is a test helper that runs a Cobra command with specific args
// and captures its stdout and stderr into a single string.
func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err = root.Execute()
	return buf.String(), err
}

func TestMain(m *testing.M) {
	// Standard TestMain for cleanup or setup if needed.
	code := m.Run()
	os.Exit(code)
}

// setupTestRepo creates a temporary directory with some predictable "dirty" code
// to ensure tests have a stable target for analysis.
func setupTestRepo(t *testing.T) string {
	tmpDir := t.TempDir()
	
	// Create a deeply nested Python file (likely to trigger complexity/nesting issues)
	content := `
def complex_function():
    if True:
        if True:
            if True:
                if True:
                    if True:
                        if True:
                            if True:
                                if True:
                                    if True:
                                        if True:
                                            print("Extremely deep nesting - CRITICAL")
`
	err := os.WriteFile(filepath.Join(tmpDir, "complex.py"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	return tmpDir
}
