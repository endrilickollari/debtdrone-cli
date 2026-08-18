package service

import (
	"context"
	"errors"
	"time"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/scanner"
	"github.com/google/uuid"
)

type ScanOptions struct {
	MaxComplexity int
	SecurityScan  bool
}

type ScanProgress struct {
	AnalyzerName string
	Index        int
	Total        int
}

type ScanService struct{}

func NewScanService() *ScanService { return &ScanService{} }

func IsPartialFailure(err error) bool {
	var partial *scanner.PartialFailureError
	return errors.As(err, &partial)
}

func (s *ScanService) Run(ctx context.Context, path string, opts ScanOptions, onProgress func(ScanProgress)) ([]models.TechnicalDebtIssue, error) {
	report, scanErr := scanner.Scan(ctx, path, scanner.Options{
		Scope:      scanner.FullScan(),
		Complexity: scanner.ComplexityOptions{Enabled: true, MaxCyclomatic: opts.MaxComplexity},
		Security:   scanner.SecurityOptions{Enabled: opts.SecurityScan},
		OnProgress: func(event scanner.ProgressEvent) {
			if onProgress != nil && event.Phase == scanner.ProgressStarted {
				onProgress(ScanProgress{AnalyzerName: event.AnalyzerName, Index: event.Index, Total: event.Total})
			}
		},
	})

	issues := make([]models.TechnicalDebtIssue, 0, len(report.Findings))
	now := time.Now()
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
			CreatedAt:          now,
			UpdatedAt:          now,
		})
	}

	return issues, scanErr
}
