package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localhistory"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/service"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturingScanRunner struct {
	path    string
	options service.ScanOptions
}

func (r *capturingScanRunner) RunDetailed(_ context.Context, path string, options service.ScanOptions, _ func(service.ScanProgress)) (service.ScanResult, error) {
	r.path = path
	r.options = options
	return service.ScanResult{}, nil
}

func createRootWithScan() *cobra.Command {
	root := &cobra.Command{Use: "debtdrone"}
	root.AddCommand(newScanCommand(service.NewScanServiceWithHistory(nil)))
	return root
}

func TestScanCmd_PrintsPartialResultsBeforeReturningAnalyzerError(t *testing.T) {
	testRepo := setupTestRepo(t)
	binDir := t.TempDir()
	trivyPath := filepath.Join(binDir, "trivy")
	require.NoError(t, os.WriteFile(trivyPath, []byte("#!/bin/sh\nprintf 'invalid trivy output'\nexit 1\n"), 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := createRootWithScan()
	output, err := executeCommand(root, "scan", testRepo, "--format", "json", "--security-scan=true")
	require.ErrorContains(t, err, "scan completed with partial results")
	jsonOutput := strings.SplitN(output, "\nError:", 2)[0]
	assert.True(t, json.Valid([]byte(jsonOutput)), "partial scan stdout must remain valid JSON: %s", jsonOutput)
	assert.Contains(t, jsonOutput, "complexity")
}

func TestScanCmdRecordsSuccessfulScanHistory(t *testing.T) {
	testRepo := setupTestRepo(t)
	history, err := localhistory.New(filepath.Join(t.TempDir(), "history.json"))
	require.NoError(t, err)
	root := &cobra.Command{Use: "debtdrone"}
	root.AddCommand(newScanCommand(service.NewScanServiceWithHistory(history)))

	_, _, err = executeCommandWithStreams(root, "scan", testRepo, "--format", "json", "--security-scan=false")
	require.NoError(t, err)
	records, err := history.List(root.Context())
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, filepath.Base(testRepo), records[0].Repository)
	assert.Equal(t, localhistory.OutcomeCompleted, records[0].Outcome)
	assert.Positive(t, records[0].Summary.Findings)
}

func TestScanCommandUsesResolvedConfigurationAndExplicitFlagPrecedence(t *testing.T) {
	var file localconfig.Overrides
	require.NoError(t, file.Set(localconfig.KeyOutputFormat, "json"))
	require.NoError(t, file.Set(localconfig.KeyMaxComplexity, "20"))
	require.NoError(t, file.Set(localconfig.KeySecurityScan, "false"))
	require.NoError(t, file.Set(localconfig.KeyCoverage, "true"))
	require.NoError(t, file.Set(localconfig.KeyHistoryEnabled, "false"))

	runner := &capturingScanRunner{}
	var historyEnabled bool
	command := newScanCommandWithResolver(
		func(flags localconfig.Overrides) (localconfig.Resolved, error) {
			return localconfig.Resolve(file, map[string]string{"DEBTDRONE_MAX_COMPLEXITY": "30"}, flags)
		},
		func(enabled bool) detailedScanRunner {
			historyEnabled = enabled
			return runner
		},
	)
	root := &cobra.Command{Use: "debtdrone"}
	root.AddCommand(command)
	target := t.TempDir()

	stdout, _, err := executeCommandWithStreams(root, "scan", target, "--max-complexity", "40", "--security-scan=true")
	require.NoError(t, err)
	assert.JSONEq(t, "[]", stdout)
	absoluteTarget, err := filepath.Abs(target)
	require.NoError(t, err)
	assert.Equal(t, absoluteTarget, runner.path)
	assert.Equal(t, service.ScanOptions{MaxComplexity: 40, SecurityScan: true, Coverage: true}, runner.options)
	assert.False(t, historyEnabled)
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
		output, _, err := executeCommandWithStreams(root, "scan", testRepo, "--format", "json")

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if !json.Valid([]byte(output)) {
			t.Errorf("Output is not valid JSON:\n%s", output)
		}

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

		if !strings.Contains(output, "CRITICAL") && !strings.Contains(output, "HIGH") {
			t.Errorf("Text output should contain found issues. Got:\n%s", output)
		}
	})
}

func TestScanCmd_MaxComplexityControlsComplexityFindings(t *testing.T) {
	testRepo := t.TempDir()
	content := `package example

func classify(value int) int {
	if value == 1 { return 1 }
	if value == 2 { return 2 }
	if value == 3 { return 3 }
	if value == 4 { return 4 }
	if value == 5 { return 5 }
	if value == 6 { return 6 }
	if value == 7 { return 7 }
	if value == 8 { return 8 }
	if value == 9 { return 9 }
	if value == 10 { return 10 }
	if value == 11 { return 11 }
	return 0
}
`
	require.NoError(t, os.WriteFile(filepath.Join(testRepo, "complex.go"), []byte(content), 0o600))

	complexityIssues := func(maxComplexity string) []models.TechnicalDebtIssue {
		t.Helper()
		root := createRootWithScan()
		stdout, _, err := executeCommandWithStreams(
			root,
			"scan",
			testRepo,
			"--format", "json",
			"--max-complexity", maxComplexity,
			"--security-scan=false",
		)
		require.NoError(t, err)

		var issues []models.TechnicalDebtIssue
		require.NoError(t, json.Unmarshal([]byte(stdout), &issues))
		var complexity []models.TechnicalDebtIssue
		for _, issue := range issues {
			if issue.ToolName == "complexity_analyzer" {
				complexity = append(complexity, issue)
			}
		}
		return complexity
	}

	lowThresholdIssues := complexityIssues("10")
	require.Len(t, lowThresholdIssues, 1)
	assert.Equal(t, "high", lowThresholdIssues[0].Severity)
	assert.Contains(t, lowThresholdIssues[0].Message, "threshold: 10")

	criticalThresholdIssues := complexityIssues("5")
	require.Len(t, criticalThresholdIssues, 1)
	assert.Equal(t, "critical", criticalThresholdIssues[0].Severity)
	assert.Contains(t, criticalThresholdIssues[0].Message, "threshold: 10")

	assert.Empty(t, complexityIssues("100"))
}

func TestScanCmd_CoverageArtifactsAreOptIn(t *testing.T) {
	testRepo := setupTestRepo(t)
	coverageXML := `<coverage><packages><package><classes><class filename="complex.py"><lines><line number="2" hits="0"/></lines></class></classes></package></packages></coverage>`
	require.NoError(t, os.WriteFile(filepath.Join(testRepo, "coverage.xml"), []byte(coverageXML), 0o600))

	tests := []struct {
		name         string
		enable       bool
		wantCoverage bool
	}{
		{name: "disabled by default", enable: false, wantCoverage: false},
		{name: "parses an existing artifact when enabled", enable: true, wantCoverage: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := createRootWithScan()
			args := []string{"scan", testRepo, "--format", "json", "--security-scan=false"}
			if test.enable {
				args = append(args, "--coverage")
			}

			output, _, err := executeCommandWithStreams(root, args...)
			require.NoError(t, err)

			var issues []models.TechnicalDebtIssue
			require.NoError(t, json.Unmarshal([]byte(output), &issues))
			hasCoverage := false
			for _, issue := range issues {
				if issue.ToolName == "coverage_analyzer" {
					hasCoverage = true
					assert.Equal(t, "test-coverage", issue.IssueType)
					assert.Equal(t, "medium", issue.Severity)
				}
			}
			assert.Equal(t, test.wantCoverage, hasCoverage)
		})
	}
}

func TestScanCmd_CoverageWarningsUseStderr(t *testing.T) {
	tests := []struct {
		name        string
		artifact    string
		wantWarning string
	}{
		{name: "missing artifact", wantWarning: "no supported coverage artifact was found"},
		{name: "malformed artifact", artifact: `<coverage`, wantWarning: "parse coverage artifact"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testRepo := setupTestRepo(t)
			if test.artifact != "" {
				require.NoError(t, os.WriteFile(filepath.Join(testRepo, "coverage.xml"), []byte(test.artifact), 0o600))
			}

			root := createRootWithScan()
			stdout, stderr, err := executeCommandWithStreams(root, "scan", testRepo, "--coverage", "--format", "json", "--security-scan=false")
			require.NoError(t, err)
			assert.True(t, json.Valid([]byte(stdout)), "coverage warnings must not contaminate JSON stdout: %s", stdout)
			assert.Contains(t, stderr, "warning [coverage_analyzer]")
			assert.Contains(t, stderr, test.wantWarning)
		})
	}
}
