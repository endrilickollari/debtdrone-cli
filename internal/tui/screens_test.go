package tui

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localhistory"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/service"
	"github.com/google/uuid"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

// Fixed inputs shared by every rendered screen. Nothing here is derived from
// the clock, the working directory, or the host terminal, so a screen renders
// identically on any machine and in CI.
const (
	fixtureRepositoryPath = "/Users/dev/projects/api-gateway"
	fixtureSpinnerFrame   = 0
)

var fixtureNow = time.Date(2026, time.September, 4, 9, 30, 0, 0, time.UTC)

// fixtureDisplayOptions mirrors the resolved defaults a real session uses, so
// the recorded screens show what a reader actually sees rather than the zero
// value of the options struct.
func fixtureDisplayOptions() scanDisplayOptions {
	defaults := localconfig.Defaults()
	return scanDisplayOptions{
		outputFormat:    defaults.OutputFormat,
		showLineNumbers: defaults.ShowLineNumbers,
		maxResults:      defaults.MaxResults,
	}
}

// withColorProfile pins the lipgloss colour profile for one test and restores
// the package default afterwards. The profile is global state, so a test that
// changes it must not leak that choice into the rest of the suite.
func withColorProfile(t *testing.T, profile termenv.Profile) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(profile)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}

func fixtureRecords() []localhistory.Record {
	return []localhistory.Record{
		{
			ID:          "01J8Z2K4S9",
			Repository:  "api-gateway",
			StartedAt:   fixtureNow.Add(-4 * time.Minute),
			CompletedAt: fixtureNow.Add(-3 * time.Minute),
			Outcome:     localhistory.OutcomeCompleted,
			Summary: localhistory.Summary{
				Findings: 42, Critical: 3, High: 11, Medium: 19, Low: 9, TechnicalDebtHours: 27.5,
			},
		},
		{
			ID:          "01J8Z1A7B2",
			Repository:  "billing-worker",
			StartedAt:   fixtureNow.Add(-2 * time.Hour),
			CompletedAt: fixtureNow.Add(-2 * time.Hour).Add(90 * time.Second),
			Outcome:     localhistory.OutcomePartial,
			Summary: localhistory.Summary{
				Findings: 8, Critical: 0, High: 2, Medium: 4, Low: 2, TechnicalDebtHours: 4.25,
				AnalyzerFailures: 1,
			},
		},
	}
}

// fixtureIssues covers the shapes the results workspace has to render: several
// severities, more than one category, and a finding carrying the optional
// detail fields.
func fixtureIssues() []models.TechnicalDebtIssue {
	description := "Extract the credential into an environment variable and rotate the exposed value."
	snippet := "const apiKey = \"sk-live-2f9c...\""
	surrounding := "12 | func newClient() *Client {\n13 |   const apiKey = \"sk-live-2f9c...\"\n14 |   return &Client{key: apiKey}"
	rule := "SEC-0142"

	return []models.TechnicalDebtIssue{
		{
			FingerprintHash: "fp-critical", FilePath: "cmd/gateway/main.go", LineNumber: intPtr(42),
			ColumnNumber: intPtr(9), Severity: "critical", Category: "security",
			IssueType: "hardcoded-secret", ToolName: "trivy", ToolRuleID: &rule,
			Message: "hardcoded credential detected", Description: &description,
			CodeSnippet: &snippet, SurroundingContext: &surrounding,
			TechnicalDebtHours: 4, EffortMultiplier: 1.5, ConfidenceScore: 0.95,
		},
		{
			FingerprintHash: "fp-high", FilePath: "internal/routing/handler.go", LineNumber: intPtr(118),
			Severity: "high", Category: "complexity", IssueType: "cyclomatic-complexity",
			ToolName: "complexity", Message: "cyclomatic complexity 24 exceeds threshold 15",
			TechnicalDebtHours: 2.5, ConfidenceScore: 0.8,
		},
		{
			FingerprintHash: "fp-medium", FilePath: "internal/routing/middleware.go", LineNumber: intPtr(63),
			Severity: "medium", Category: "complexity", IssueType: "deep-nesting",
			ToolName: "complexity", Message: "nesting depth 6 exceeds threshold 4",
			TechnicalDebtHours: 1,
		},
		{
			FingerprintHash: "fp-low", FilePath: "internal/store/postgres.go", LineNumber: intPtr(7),
			Severity: "low", Category: "coverage", IssueType: "low-coverage",
			ToolName: "coverage", Message: "file has low test coverage",
			TechnicalDebtHours: 0.5,
		},
	}
}

// dashboardScreen builds the menu model in a deterministic state.
func dashboardScreen(t *testing.T, width, height int, records []localhistory.Record, historyErr error) *MenuModel {
	t.Helper()
	menu := newMenuModelWithHistory(fixtureRepositoryPath, func(context.Context) ([]localhistory.Record, error) {
		return records, historyErr
	})
	menu.width, menu.height = width, height

	command := menu.RefreshHistory()
	require.NotNil(t, command)
	menu.Update(command())
	return menu
}

// scanningScreen builds a running scan without executing one.
func scanningScreen(t *testing.T, width, height int, stage string, completed, total int) *ScanModel {
	t.Helper()
	model := newScanModel()
	model.width, model.height = width, height
	model.Start(fixtureRepositoryPath, service.ScanOptions{}, fixtureDisplayOptions(), false)
	model.spinnerFrame = fixtureSpinnerFrame
	model.elapsed = 7 * time.Second
	if total > 0 {
		model.Update(scanProgressMsg{runID: model.runID, stage: stage, completed: completed, total: total})
	}
	return model
}

// resultsScreen builds a completed scan showing its findings.
func resultsScreen(t *testing.T, width, height int, issues []models.TechnicalDebtIssue) *ScanModel {
	t.Helper()
	model := newScanModel()
	model.width, model.height = width, height
	model.Start(fixtureRepositoryPath, service.ScanOptions{}, fixtureDisplayOptions(), false)
	model.Update(scanCompleteMsg{runID: model.runID, path: fixtureRepositoryPath, issues: issues})
	model.elapsed = 12 * time.Second
	return model
}

func historyScreen(t *testing.T, width, height int) *HistoryModel {
	t.Helper()
	model := newHistoryModel()
	model.width, model.height = width, height

	completed := fixtureNow
	duration := 84
	run := models.AnalysisRun{
		// A real run always carries an id; the zero value is the sentinel the
		// detail pane uses for "nothing selected".
		ID:                      uuid.MustParse("3f2b1c04-9a7d-4f18-9c2e-6b5a0d7e1234"),
		StartedAt:               fixtureNow.Add(-84 * time.Second),
		CompletedAt:             &completed,
		DurationSeconds:         &duration,
		Status:                  "completed",
		TotalIssuesFound:        4,
		CriticalIssuesCount:     1,
		HighIssuesCount:         1,
		MediumIssuesCount:       1,
		LowIssuesCount:          1,
		TotalTechnicalDebtHours: 8,
	}
	issues := fixtureIssues()
	model.SetEntries([]historyEntry{{
		run: run, path: fixtureRepositoryPath, issues: issues, summary: summarizeIssues(issues),
	}})
	return model
}

func configScreen(t *testing.T, width, height int) *ConfigModel {
	t.Helper()
	model := newConfigModelWithValues(localconfig.Defaults())
	model.width, model.height = width, height
	return model
}
