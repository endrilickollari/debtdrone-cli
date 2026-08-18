package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/analysis/analyzers"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/analysis/analyzers/security"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/scancore"
	"github.com/google/uuid"
)

// Scan opens path once, constructs the enabled core analyzers, and returns a
// neutral report suitable for CLI, SaaS, or other Go consumers.
func Scan(ctx context.Context, path string, options Options) (Report, error) {
	if options.Coverage.Enabled {
		return Report{}, fmt.Errorf("coverage scanning is not available until Phase 2")
	}
	if options.Scope.Mode == "" {
		options.Scope = FullScan()
	}
	if options.Scope.Mode == ScopeNoChanges {
		return Report{Metrics: make(map[string][]Metric)}, nil
	}
	if err := validateScope(options.Scope); err != nil {
		return Report{}, err
	}

	repo, err := git.NewService().OpenLocal(path)
	if err != nil {
		return Report{}, fmt.Errorf("open repository: %w", err)
	}

	ctx = context.WithValue(ctx, "analysisRunID", uuid.New())
	ctx = context.WithValue(ctx, "repositoryID", uuid.New())
	ctx = context.WithValue(ctx, "userID", uuid.New())
	ctx = context.WithValue(ctx, "isCLI", true)
	if options.Scope.Mode == ScopeIncremental {
		targetFiles, err := normalizeTargetFiles(repo.Path, options.Scope.Files)
		if err != nil {
			return Report{}, err
		}
		ctx = context.WithValue(ctx, "targetFiles", targetFiles)
	}

	config := models.DefaultComplexityConfig()
	if options.Complexity.MaxCyclomatic > 0 {
		config.CyclomaticThreshold = options.Complexity.MaxCyclomatic
	}
	ctx = context.WithValue(ctx, "complexityConfig", config)

	coreAnalyzers := []Analyzer{
		legacyAnalyzer{id: "line_counter", analyzer: analyzers.NewLineCounter(), repo: repo},
	}
	if options.Complexity.Enabled {
		coreAnalyzers = append(coreAnalyzers, legacyAnalyzer{id: "complexity_analyzer", analyzer: analyzers.NewComplexityAnalyzer(), repo: repo})
	}
	if options.Security.Enabled {
		coreAnalyzers = append(coreAnalyzers, legacyAnalyzer{id: "trivy", analyzer: security.NewTrivyAnalyzer(), repo: repo})
	}

	return (Runner{MaxParallel: options.MaxParallel, OnProgress: options.OnProgress}).Run(ctx, coreAnalyzers)
}

func validateScope(scope Scope) error {
	switch scope.Mode {
	case ScopeFull, ScopeNoChanges:
		return nil
	case ScopeIncremental:
		if len(scope.Files) == 0 {
			return fmt.Errorf("incremental scan requires at least one file")
		}
		return nil
	default:
		return fmt.Errorf("unsupported scan scope %q", scope.Mode)
	}
}

func normalizeTargetFiles(root string, files []string) ([]string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	result := make([]string, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		file = filepath.Clean(file)
		if filepath.IsAbs(file) {
			relative, err := filepath.Rel(absoluteRoot, file)
			if err != nil {
				return nil, fmt.Errorf("resolve incremental file %q: %w", file, err)
			}
			file = relative
		}
		if file == ".." || strings.HasPrefix(file, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("incremental file %q is outside repository root", file)
		}
		file = filepath.ToSlash(file)
		file = strings.TrimPrefix(file, "./")
		if !strings.HasPrefix(file, "/") {
			file = "/" + file
		}
		if _, exists := seen[file]; exists {
			continue
		}
		seen[file] = struct{}{}
		result = append(result, file)
	}
	sort.Strings(result)
	return result, nil
}

type legacyAnalyzer struct {
	id       string
	analyzer scancore.Analyzer
	repo     *git.Repository
}

func (a legacyAnalyzer) ID() string   { return a.id }
func (a legacyAnalyzer) Name() string { return a.analyzer.Name() }

func (a legacyAnalyzer) Analyze(ctx context.Context) (AnalyzerResult, error) {
	result, err := a.analyzer.Analyze(ctx, a.repo)
	if err != nil {
		return AnalyzerResult{}, err
	}
	converted := AnalyzerResult{Metrics: metricsFromMap(result.Metrics)}
	for _, issue := range result.Issues {
		finding := Finding{
			Fingerprint:        issue.FingerprintHash,
			AnalyzerID:         a.id,
			RuleID:             issue.ToolRuleID,
			Type:               issue.IssueType,
			Category:           issue.Category,
			Severity:           issue.Severity,
			Message:            issue.Message,
			Description:        issue.Description,
			Location:           Location{Path: issue.FilePath, Line: issue.LineNumber, Column: issue.ColumnNumber},
			Confidence:         issue.ConfidenceScore,
			EstimatedDebtHours: issue.TechnicalDebtHours,
			EffortMultiplier:   issue.EffortMultiplier,
			CodeSnippet:        issue.CodeSnippet,
		}
		if finding.Fingerprint == "" {
			finding.Fingerprint = fingerprint(finding)
		}
		converted.Findings = append(converted.Findings, finding)
	}
	return converted, nil
}

func fingerprint(finding Finding) string {
	rule := ""
	if finding.RuleID != nil {
		rule = *finding.RuleID
	}
	codeSnippet := ""
	if finding.CodeSnippet != nil {
		codeSnippet = strings.Join(strings.Fields(*finding.CodeSnippet), " ")
	}
	canonical := fmt.Sprintf("%s|%s|%s|%s", filepath.ToSlash(finding.Location.Path), finding.Type, rule, codeSnippet)
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func metricsFromMap(values map[string]any) []Metric {
	if len(values) == 0 {
		return nil
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	metrics := make([]Metric, 0, len(names))
	for _, name := range names {
		metrics = append(metrics, Metric{Name: name, Value: values[name]})
	}
	return metrics
}
