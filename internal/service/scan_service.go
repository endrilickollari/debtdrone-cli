package service

import (
	"context"
	"fmt"

	"github.com/endrilickollari/debtdrone-cli/internal/analysis"
	"github.com/endrilickollari/debtdrone-cli/internal/analysis/analyzers"
	"github.com/endrilickollari/debtdrone-cli/internal/analysis/analyzers/security"
	"github.com/endrilickollari/debtdrone-cli/internal/git"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/endrilickollari/debtdrone-cli/internal/store/memory"
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

type ScanService struct {
	gitService *git.Service
}

func NewScanService() *ScanService {
	return &ScanService{
		gitService: git.NewService(),
	}
}

func (s *ScanService) Run(ctx context.Context, path string, opts ScanOptions, onProgress func(ScanProgress)) ([]models.TechnicalDebtIssue, error) {
	repo, err := s.gitService.OpenLocal(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	complexityStore := memory.NewInMemoryComplexityStore()
	lineCounter := analyzers.NewLineCounter()
	complexityAnalyzer := analyzers.NewComplexityAnalyzer(complexityStore)

	analyzersList := []analysis.Analyzer{lineCounter, complexityAnalyzer}
	if opts.SecurityScan {
		analyzersList = append(analyzersList, security.NewTrivyAnalyzer())
	}

	// Enrich context
	ctx = context.WithValue(ctx, "analysisRunID", uuid.New())
	ctx = context.WithValue(ctx, "repositoryID", uuid.New())
	ctx = context.WithValue(ctx, "userID", uuid.New())
	ctx = context.WithValue(ctx, "complexityConfig", models.ComplexityConfig{
		CyclomaticThreshold: opts.MaxComplexity,
	})

	var allIssues []models.TechnicalDebtIssue
	total := len(analyzersList)

	for i, analyzer := range analyzersList {
		if onProgress != nil {
			onProgress(ScanProgress{
				AnalyzerName: analyzer.Name(),
				Index:        i,
				Total:        total,
			})
		}

		result, err := analyzer.Analyze(ctx, repo)
		if err != nil {
			continue
		}
		allIssues = append(allIssues, result.Issues...)
	}

	return allIssues, nil
}
