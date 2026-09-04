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

func TestScanProgressReportsCompletedAnalyzersNotJustStarts(t *testing.T) {
	service := &ScanService{scan: func(_ context.Context, _ string, options scanner.Options) (scanner.Report, error) {
		options.OnProgress(scanner.ProgressEvent{
			AnalyzerName: "Complexity", Index: 0, Completed: 0, Total: 2, Phase: scanner.ProgressStarted,
		})
		options.OnProgress(scanner.ProgressEvent{
			AnalyzerName: "Complexity", Index: 0, Completed: 1, Total: 2, Phase: scanner.ProgressFinished,
		})
		options.OnProgress(scanner.ProgressEvent{
			AnalyzerName: "Security", Index: 1, Completed: 1, Total: 2, Phase: scanner.ProgressStarted,
		})
		options.OnProgress(scanner.ProgressEvent{
			AnalyzerName: "Security", Index: 1, Completed: 2, Total: 2, Phase: scanner.ProgressFailed,
		})
		return scanner.Report{}, nil
	}}

	var events []ScanProgress
	_, err := service.RunDetailed(context.Background(), "/repo", ScanOptions{}, func(p ScanProgress) {
		events = append(events, p)
	})
	require.NoError(t, err)

	// Finished and failed events must reach the consumer: without them a
	// caller can only ever observe analyzers starting and can never report
	// that the final analyzer completed.
	require.Len(t, events, 4)
	assert.Equal(t, []int{0, 1, 1, 2}, []int{
		events[0].Completed, events[1].Completed, events[2].Completed, events[3].Completed,
	})
	assert.Equal(t, []bool{true, false, true, false}, []bool{
		events[0].Started, events[1].Started, events[2].Started, events[3].Started,
	})
	assert.Equal(t, 2, events[3].Total)
}

func TestScanServiceToleratesAbsentProgressCallback(t *testing.T) {
	service := &ScanService{scan: func(_ context.Context, _ string, options scanner.Options) (scanner.Report, error) {
		options.OnProgress(scanner.ProgressEvent{AnalyzerName: "Complexity", Total: 1, Phase: scanner.ProgressStarted})
		return scanner.Report{}, nil
	}}

	assert.NotPanics(t, func() {
		_, _ = service.RunDetailed(context.Background(), "/repo", ScanOptions{}, nil)
	})
}
