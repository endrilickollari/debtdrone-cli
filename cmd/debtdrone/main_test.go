package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err = root.Execute()
	return buf.String(), err
}

func executeCommandWithStreams(root *cobra.Command, args ...string) (stdout string, stderr string, err error) {
	stdoutBuffer := new(bytes.Buffer)
	stderrBuffer := new(bytes.Buffer)
	root.SetOut(stdoutBuffer)
	root.SetErr(stderrBuffer)
	root.SetArgs(args)

	err = root.Execute()
	return stdoutBuffer.String(), stderrBuffer.String(), err
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func setupTestRepo(t *testing.T) string {
	tmpDir := t.TempDir()

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
