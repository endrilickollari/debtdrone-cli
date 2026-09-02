package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/localhistory"
	"github.com/endrilickollari/debtdrone-cli/v2/scanner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingHistory struct {
	inputs []localhistory.RecordInput
	err    error
}

func (h *recordingHistory) RecordScan(_ context.Context, input localhistory.RecordInput) (localhistory.Record, error) {
	h.inputs = append(h.inputs, input)
	return localhistory.Record{}, h.err
}

func TestScanServiceCoverageDoesNotEnableRepositoryExecution(t *testing.T) {
	var captured scanner.Options
	service := &ScanService{scan: func(_ context.Context, _ string, options scanner.Options) (scanner.Report, error) {
		captured = options
		return scanner.Report{}, nil
	}}

	_, err := service.RunDetailed(context.Background(), "/repo", ScanOptions{Coverage: true}, nil)
	require.NoError(t, err)
	assert.True(t, captured.Coverage.Enabled)
	assert.False(t, captured.Coverage.RunLocalTests)
	assert.Nil(t, captured.Coverage.IsolatedExecutor)
}

func TestNewScanServiceHonorsHistoryPersistenceOptOut(t *testing.T) {
	service := NewScanServiceWithHistoryEnabled(false)
	assert.Nil(t, service.history)
	assert.NotNil(t, service.scan)
}

func TestScanServiceRecordsCompletedAndPartialScans(t *testing.T) {
	now := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		scanError   error
		wantOutcome localhistory.Outcome
	}{
		{name: "completed", wantOutcome: localhistory.OutcomeCompleted},
		{
			name:        "partial",
			scanError:   &scanner.PartialFailureError{Failures: []scanner.AnalyzerFailure{{AnalyzerID: "security", Error: "unavailable"}}},
			wantOutcome: localhistory.OutcomePartial,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			history := &recordingHistory{}
			service := &ScanService{
				history: history,
				now:     func() time.Time { return now },
				scan: func(context.Context, string, scanner.Options) (scanner.Report, error) {
					return scanner.Report{
						Findings: []scanner.Finding{
							{Severity: "critical", EstimatedDebtHours: 2.5},
							{Severity: "HIGH", EstimatedDebtHours: 1.25},
							{Severity: "informational", EstimatedDebtHours: 0.25},
						},
						Warnings: []scanner.Warning{{AnalyzerID: "coverage", Message: "missing artifact"}},
						Failures: []scanner.AnalyzerFailure{{AnalyzerID: "security", Error: "unavailable"}},
					}, test.scanError
				},
			}

			result, err := service.RunDetailed(context.Background(), "/private/work/customer-repo", ScanOptions{}, nil)
			assert.ErrorIs(t, err, test.scanError)
			require.Len(t, result.Issues, 3)
			require.Len(t, history.inputs, 1)
			input := history.inputs[0]
			assert.Equal(t, "/private/work/customer-repo", input.RepositoryPath)
			assert.Equal(t, now, input.StartedAt)
			assert.Equal(t, now, input.CompletedAt)
			assert.Equal(t, test.wantOutcome, input.Outcome)
			assert.Equal(t, localhistory.Summary{
				Findings: 3, Critical: 1, High: 1, TechnicalDebtHours: 4,
				Warnings: 1, AnalyzerFailures: 1,
			}, input.Summary)
		})
	}
}

func TestScanServiceDoesNotRecordFatalScans(t *testing.T) {
	history := &recordingHistory{}
	scanErr := errors.New("repository unavailable")
	service := &ScanService{
		history: history,
		scan: func(context.Context, string, scanner.Options) (scanner.Report, error) {
			return scanner.Report{}, scanErr
		},
	}

	_, err := service.RunDetailed(context.Background(), "/repo", ScanOptions{}, nil)
	assert.ErrorIs(t, err, scanErr)
	assert.Empty(t, history.inputs)
}

func TestScanServiceReportsHistoryFailureAsWarning(t *testing.T) {
	history := &recordingHistory{err: errors.New("disk full")}
	service := &ScanService{
		history: history,
		scan: func(context.Context, string, scanner.Options) (scanner.Report, error) {
			return scanner.Report{}, nil
		},
	}

	result, err := service.RunDetailed(context.Background(), "/repo", ScanOptions{}, nil)
	require.NoError(t, err)
	require.Len(t, result.Warnings, 1)
	assert.Equal(t, "local_history", result.Warnings[0].AnalyzerID)
	assert.Contains(t, result.Warnings[0].Message, "disk full")
}
