package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	fmt.Println("Building CLI binary...")
	cmd := exec.Command("go", "build", "-o", "debtdrone_test", ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Failed to build CLI binary: %v\nOutput: %s\n", err, output)
		os.Exit(1)
	}

	code := m.Run()

	os.Remove("./debtdrone_test")

	os.Exit(code)
}

func runCLI(args ...string) (string, error) {
	cmd := exec.Command("./debtdrone_test", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func copyFile(t *testing.T, src, dst string) {
	sourceFile, err := os.Open(src)
	if err != nil {
		t.Fatalf("Failed to open source file %s: %v", src, err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		t.Fatalf("Failed to create dest file %s: %v", dst, err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		t.Fatalf("Failed to copy file content: %v", err)
	}
}

func TestCLI_Languages_Integration(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		fileName string
		wantFail bool // If true, expects exit code 1 (failure) with -fail-on critical
	}{
		{"Clean Go", "clean_code", "simple.go", false},
		{"Clean Python", "clean_code", "simple.py", false},
		{"Clean JS", "clean_code", "simple.js", false},
		{"Clean TS", "clean_code", "simple.ts", false},
		{"Clean Java", "clean_code", "Simple.java", false},
		{"Clean Kotlin", "clean_code", "simple.kt", false},
		{"Clean Ruby", "clean_code", "simple.rb", false},
		{"Clean Rust", "clean_code", "simple.rs", false},
		{"Clean PHP", "clean_code", "simple.php", false},
		{"Clean Swift", "clean_code", "simple.swift", false},
		{"Clean C++", "clean_code", "simple.cpp", false},
		{"Clean C#", "clean_code", "simple.cs", false},

		{"Dirty Go", "dirty_code", "complex.go", true},
		{"Dirty Python", "dirty_code", "complex.py", true},
		{"Dirty JS", "dirty_code", "complex.js", true},
		{"Dirty TS", "dirty_code", "complex.ts", true},
		{"Dirty Java", "dirty_code", "Complex.java", true},
		{"Dirty Kotlin", "dirty_code", "complex.kt", true},
		{"Dirty Ruby", "dirty_code", "complex.rb", true},
		{"Dirty Rust", "dirty_code", "complex.rs", true},
		{"Dirty PHP", "dirty_code", "complex.php", true},
		{"Dirty Swift", "dirty_code", "complex.swift", true},
		{"Dirty C++", "dirty_code", "complex.cpp", true},
		{"Dirty C#", "dirty_code", "complex.cs", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			tmpDir, err := os.MkdirTemp("", "debtdrone_test_*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			src := filepath.Join("testdata", tt.dir, tt.fileName)
			dst := filepath.Join(tmpDir, tt.fileName)
			copyFile(t, src, dst)

			args := []string{"-fail-on", "medium", tmpDir}
			_, err = runCLI(args...)

			if tt.wantFail {
				if err == nil {
					t.Errorf("Expected failure (exit code 1) for %s, but got success", tt.fileName)
				}
			} else {
				if err != nil {
					t.Errorf("Expected success (exit code 0) for %s, but got error: %v", tt.fileName, err)
				}
			}
		})
	}
}

func TestCLI_FailOn_Critical_WithDirtyCode(t *testing.T) {
	output, err := runCLI("-fail-on", "critical", "./testdata/dirty_code")

	if err == nil {
		t.Error("Expected error (non-zero exit code) for critical debt with -fail-on critical, but got nil")
	}

	if !strings.Contains(output, "Quality Gate failed") {
		t.Errorf("Expected output to contain 'Quality Gate failed', got:\n%s", output)
	}
}

func TestCLI_FailOn_None_WithDirtyCode(t *testing.T) {
	output, err := runCLI("-fail-on", "none", "./testdata/dirty_code")

	if err != nil {
		t.Errorf("Expected no error for -fail-on none, but got: %v\nOutput: %s", err, output)
	}
}

func TestCLI_CleanCode(t *testing.T) {
	output, err := runCLI("./testdata/clean_code")

	if err != nil {
		t.Errorf("Expected success for clean code, but got error: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(output, "Scan passed") {
		t.Errorf("Expected output to contain 'Scan passed', got:\n%s", output)
	}
}

func TestCLI_JSONOutput(t *testing.T) {
	output, err := runCLI("-output", "json", "./testdata/clean_code")

	if err != nil {
		t.Errorf("Expected success for JSON output, but got error: %v\nOutput: %s", err, output)
	}

	var result []interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Errorf("Output is not valid JSON: %v\nOutput content:\n%s", err, output)
	}
}
