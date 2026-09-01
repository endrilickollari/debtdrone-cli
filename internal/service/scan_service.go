package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/localhistory"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/scanner"
	"github.com/google/uuid"
)

type ScanOptions struct {
	MaxComplexity int
	SecurityScan  bool
	Coverage      bool
}

type ScanProgress struct {
	AnalyzerName string
	Index        int
	Total        int
}

type ScanResult struct {
	Issues   []models.TechnicalDebtIssue
	Warnings []scanner.Warning
}

type scanFunc func(context.Context, string, scanner.Options) (scanner.Report, error)

type ScanService struct {
	scan    scanFunc
	history HistoryRecorder
	now     func() time.Time
}

type HistoryRecorder interface {
	RecordScan(context.Context, localhistory.RecordInput) (localhistory.Record, error)
}

func NewScanService() *ScanService {
	path, err := localhistory.DefaultPath()
	if err != nil {
		return &ScanService{scan: scanner.Scan, now: time.Now, history: failingHistoryRecorder{err: err}}
	}
	history, err := localhistory.New(path)
	if err != nil {
		return &ScanService{scan: scanner.Scan, now: time.Now, history: failingHistoryRecorder{err: err}}
	}
	return &ScanService{scan: scanner.Scan, history: history, now: time.Now}
}

func NewScanServiceWithHistory(history HistoryRecorder) *ScanService {
	return &ScanService{scan: scanner.Scan, history: history, now: time.Now}
}

type failingHistoryRecorder struct{ err error }

func (r failingHistoryRecorder) RecordScan(context.Context, localhistory.RecordInput) (localhistory.Record, error) {
	return localhistory.Record{}, r.err
}

func IsPartialFailure(err error) bool {
	var partial *scanner.PartialFailureError
	return errors.As(err, &partial)
}

func (s *ScanService) Run(ctx context.Context, path string, opts ScanOptions, onProgress func(ScanProgress)) ([]models.TechnicalDebtIssue, error) {
	result, err := s.RunDetailed(ctx, path, opts, onProgress)
	return result.Issues, err
}

func (s *ScanService) RunDetailed(ctx context.Context, path string, opts ScanOptions, onProgress func(ScanProgress)) (ScanResult, error) {
	now := s.now
	if now == nil {
		now = time.Now
	}
	startedAt := now()

	scan := s.scan
	if scan == nil {
		scan = scanner.Scan
	}
	report, scanErr := scan(ctx, path, scanner.Options{
		Scope:      scanner.FullScan(),
		Complexity: scanner.ComplexityOptions{Enabled: true, MaxCyclomatic: opts.MaxComplexity},
		Security:   scanner.SecurityOptions{Enabled: opts.SecurityScan},
		Coverage:   scanner.CoverageOptions{Enabled: opts.Coverage},
		OnProgress: func(event scanner.ProgressEvent) {
			if onProgress != nil && event.Phase == scanner.ProgressStarted {
				onProgress(ScanProgress{AnalyzerName: event.AnalyzerName, Index: event.Index, Total: event.Total})
			}
		},
	})

	issues := make([]models.TechnicalDebtIssue, 0, len(report.Findings))
	issueTime := now()
	userID, repositoryID, analysisRunID := uuid.New(), uuid.New(), uuid.New()
	for _, finding := range report.Findings {
		issues = append(issues, models.TechnicalDebtIssue{
			ID:                 uuid.New(),
			UserID:             userID,
			RepositoryID:       repositoryID,
			AnalysisRunID:      analysisRunID,
			FilePath:           finding.Location.Path,
			LineNumber:         finding.Location.Line,
			ColumnNumber:       finding.Location.Column,
			IssueType:          finding.Type,
			Severity:           finding.Severity,
			Category:           finding.Category,
			Message:            finding.Message,
			Description:        finding.Description,
			ToolName:           finding.AnalyzerID,
			ToolRuleID:         finding.RuleID,
			ConfidenceScore:    finding.Confidence,
			TechnicalDebtHours: finding.EstimatedDebtHours,
			EffortMultiplier:   finding.EffortMultiplier,
			Status:             "open",
			CodeSnippet:        finding.CodeSnippet,
			FingerprintHash:    finding.Fingerprint,
			CreatedAt:          issueTime,
			UpdatedAt:          issueTime,
		})
	}

	result := ScanResult{Issues: issues, Warnings: report.Warnings}
	if s.history != nil && (scanErr == nil || IsPartialFailure(scanErr)) {
		outcome := localhistory.OutcomeCompleted
		if scanErr != nil {
			outcome = localhistory.OutcomePartial
		}
		_, err := s.history.RecordScan(ctx, localhistory.RecordInput{
			RepositoryPath: path,
			StartedAt:      startedAt,
			CompletedAt:    now(),
			Outcome:        outcome,
			Summary:        historySummary(report),
		})
		if err != nil {
			result.Warnings = append(result.Warnings, scanner.Warning{
				AnalyzerID: "local_history",
				Message:    fmt.Sprintf("could not persist local scan history: %v", err),
			})
		}
	}

	return result, scanErr
}

func historySummary(report scanner.Report) localhistory.Summary {
	summary := localhistory.Summary{
		Findings:         len(report.Findings),
		Warnings:         len(report.Warnings),
		AnalyzerFailures: len(report.Failures),
	}
	for _, finding := range report.Findings {
		summary.TechnicalDebtHours += finding.EstimatedDebtHours
		switch strings.ToLower(finding.Severity) {
		case "critical":
			summary.Critical++
		case "high":
			summary.High++
		case "medium":
			summary.Medium++
		case "low":
			summary.Low++
		}
	}
	return summary
}
