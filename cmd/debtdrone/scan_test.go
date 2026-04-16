package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func createRootWithScan() *cobra.Command {
	root := &cobra.Command{Use: "debtdrone"}
	root.AddCommand(newScanCmd())
	return root
}

func TestScanCmd_QualityGate(t *testing.T) {
	testRepo := setupTestRepo(t)

	tests := []struct {
		name           string
		failOn         string
		expectError    bool
		errorSubstring string
	}{
		{
			name:        "No --fail-on flag (default behavior, exit 0)",
			failOn:      "",
			expectError: false,
		},
		{
			name:           "--fail-on=critical (should fail because testRepo is dirty)",
			failOn:         "critical",
			expectError:    true,
			errorSubstring: "quality gate failed: found issues matching or exceeding severity 'critical'",
		},
		{
			name:           "--fail-on=high (should fail because testRepo is dirty)",
			failOn:         "high",
			expectError:    true,
			errorSubstring: "quality gate failed: found issues matching or exceeding severity 'high'",
		},
		{
			name:           "--fail-on=low (should fail because testRepo is dirty)",
			failOn:         "low",
			expectError:    true,
			errorSubstring: "quality gate failed: found issues matching or exceeding severity 'low'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := createRootWithScan()
			args := []string{"scan", testRepo}
			if tt.failOn != "" {
				args = append(args, "--fail-on", tt.failOn)
			}

			output, err := executeCommand(root, args...)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s, but got nil. Output:\n%s", tt.name, output)
				} else if !strings.Contains(err.Error(), tt.errorSubstring) {
					t.Errorf("Expected error to contain %q, but got %q", tt.errorSubstring, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for %s, but got: %v. Output:\n%s", tt.name, err, output)
				}
			}
		})
	}
}

func TestScanCmd_OutputFormats(t *testing.T) {
	testRepo := setupTestRepo(t)

	t.Run("--format=json", func(t *testing.T) {
		root := createRootWithScan()
		output, err := executeCommand(root, "scan", testRepo, "--format", "json")
		
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if !json.Valid([]byte(output)) {
			t.Errorf("Output is not valid JSON:\n%s", output)
		}

		// Basic sanity check that it's an array of issues
		if !strings.HasPrefix(strings.TrimSpace(output), "[") {
			t.Errorf("JSON output should be an array, got:\n%s", output)
		}
	})

	t.Run("--format=text", func(t *testing.T) {
		root := createRootWithScan()
		output, err := executeCommand(root, "scan", testRepo, "--format", "text")
		
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		headers := []string{"SEVERITY", "FILE:LINE", "RULE", "MESSAGE"}
		for _, header := range headers {
			if !strings.Contains(output, header) {
				t.Errorf("Text output missing expected header %q. Got:\n%s", header, output)
			}
		}

		// Assert it found the issue in our dirty file
		if !strings.Contains(output, "CRITICAL") && !strings.Contains(output, "HIGH") {
			t.Errorf("Text output should contain found issues. Got:\n%s", output)
		}
	})
}
