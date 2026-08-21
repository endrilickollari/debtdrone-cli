package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/analysis/analyzers"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/analysis/analyzers/security"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/filepolicy"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/scancore"
	coveragecore "github.com/endrilickollari/debtdrone-cli/v2/scanner/coverage"
	"github.com/endrilickollari/debtdrone-cli/v2/scanner/repostructure"
	"github.com/go-git/go-billy/v5/util"
	"github.com/google/uuid"
)

// Scan opens path once, constructs the enabled core analyzers, and returns a
// neutral report suitable for CLI, SaaS, or other Go consumers.
func Scan(ctx context.Context, path string, options Options) (Report, error) {
	if options.Scope.Mode == "" {
		options.Scope = FullScan()
	}
	if err := validateScope(options.Scope); err != nil {
		return Report{}, err
	}
	if options.Scope.Mode == ScopeNoChanges {
		return Report{Metrics: make(map[string][]Metric)}, nil
	}
	repo, err := git.NewService().OpenLocal(path)
	if err != nil {
		return Report{}, fmt.Errorf("open repository: %w", err)
	}

	ctx = context.WithValue(ctx, "analysisRunID", uuid.New())
	ctx = context.WithValue(ctx, "repositoryID", uuid.New())
	ctx = context.WithValue(ctx, "userID", uuid.New())
	ctx = context.WithValue(ctx, "isCLI", true)
	structure := repostructure.Detect(ctx, repo.Path)
	ctx = repostructure.WithContext(ctx, structure)
	targetFiles, filterWarnings, err := prepareTargetFiles(ctx, repo, options.Scope)
	if err != nil {
		return Report{}, err
	}
	// Presence of targetFiles is intentional even when the slice is empty: an
	// empty incremental scan after filtering must never become a full scan.
	ctx = scancore.WithTargetFiles(ctx, targetFiles, options.Scope.Mode == ScopeIncremental)

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
	if options.Coverage.Enabled {
		artifacts := make([]coveragecore.Artifact, len(options.Coverage.Artifacts))
		for index, artifact := range options.Coverage.Artifacts {
			artifacts[index] = coveragecore.Artifact{Name: artifact.Name, Root: artifact.Root, Content: append([]byte(nil), artifact.Content...)}
		}
		coreAnalyzers = append(coreAnalyzers, coverageAnalyzer{
			repoRoot: repo.Path,
			roots:    structure.BuildRoots,
			options: coveragecore.Options{
				Artifacts:     artifacts,
				RunLocalTests: options.Coverage.RunLocalTests,
			},
		})
	}

	report, err := (Runner{MaxParallel: options.MaxParallel, OnProgress: options.OnProgress}).Run(ctx, coreAnalyzers)
	report.Metrics["repository_structure"] = repositoryStructureMetrics(structure)
	for _, message := range structure.Warnings {
		report.Warnings = append(report.Warnings, Warning{AnalyzerID: "repository_structure", Message: message})
	}
	report.Warnings = append(filterWarnings, report.Warnings...)
	return report, err
}

func repositoryStructureMetrics(structure *repostructure.Structure) []Metric {
	buildRoots := make([]repostructure.BuildRoot, len(structure.BuildRoots))
	for index, root := range structure.BuildRoots {
		buildRoots[index] = root
		buildRoots[index].Dir = repositoryRelativePath(structure.RepoRoot, root.Dir)
	}
	return []Metric{
		{Name: "build_roots", Value: buildRoots},
		{Name: "doc_dirs", Value: repositoryRelativePaths(structure.RepoRoot, structure.DocDirs)},
		{Name: "entry_points", Value: repositoryRelativePaths(structure.RepoRoot, structure.EntryPoints)},
		{Name: "is_monorepo", Value: structure.IsMonorepo},
		{Name: "source_roots", Value: repositoryRelativePaths(structure.RepoRoot, structure.SourceRoots)},
		{Name: "test_dirs", Value: repositoryRelativePaths(structure.RepoRoot, structure.TestDirs)},
	}
}

func repositoryRelativePaths(repoRoot string, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = repositoryRelativePath(repoRoot, value)
	}
	return result
}

func repositoryRelativePath(repoRoot, value string) string {
	relative, err := filepath.Rel(repoRoot, value)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(relative)
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
		original := file
		file = strings.TrimSpace(file)
		if file == "" {
			return nil, fmt.Errorf("incremental file path is empty")
		}
		if filepath.IsAbs(file) {
			relative, err := filepath.Rel(absoluteRoot, filepath.Clean(file))
			if err != nil {
				return nil, fmt.Errorf("incremental file %q is outside repository root: %w", original, err)
			}
			file = relative
		} else if isWindowsAbsolutePath(file) {
			return nil, fmt.Errorf("incremental file %q uses an absolute path for a different platform", original)
		}
		file = strings.ReplaceAll(file, "\\", "/")
		file = path.Clean(file)
		if file == "." || file == ".." || strings.HasPrefix(file, "../") {
			return nil, fmt.Errorf("incremental file %q is outside repository root", original)
		}
		file = "/" + strings.TrimPrefix(file, "/")
		if err := ensurePathWithinRoot(absoluteRoot, file); err != nil {
			return nil, err
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

func prepareTargetFiles(ctx context.Context, repo *git.Repository, scope Scope) ([]string, []Warning, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	var candidates []string
	if scope.Mode == ScopeIncremental {
		var err error
		candidates, err = normalizeTargetFiles(repo.Path, scope.Files)
		if err != nil {
			return nil, nil, err
		}
	} else {
		err := util.Walk(repo.FS, "/", func(filePath string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("visit %s: %w", filePath, walkErr)
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			if info.IsDir() {
				if filePath != "/" && filepolicy.IsGeneratedDirectory(info.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			candidates = append(candidates, canonicalRepositoryPath(filePath))
			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("discover repository files: %w", err)
		}
	}

	targets := make([]string, 0, len(candidates))
	warnings := make([]Warning, 0)
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if filepolicy.IsGeneratedPath(candidate) {
			continue
		}
		ok, reason := filepolicy.CheckAnalyzableContext(ctx, repo.FS, candidate)
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if ok {
			targets = append(targets, candidate)
			continue
		}
		if !filepolicy.IsSilentSkipReason(reason) {
			warnings = append(warnings, Warning{AnalyzerID: "scanner", Message: fmt.Sprintf("%s skipped: %s", candidate, reason)})
		}
	}
	sort.Strings(targets)
	sort.Slice(warnings, func(i, j int) bool { return warnings[i].Message < warnings[j].Message })
	return targets, warnings, nil
}

func canonicalRepositoryPath(file string) string {
	file = strings.ReplaceAll(file, "\\", "/")
	file = path.Clean("/" + strings.TrimPrefix(file, "/"))
	return file
}

func isWindowsAbsolutePath(file string) bool {
	normalized := strings.ReplaceAll(file, "\\", "/")
	if strings.HasPrefix(normalized, "//") {
		return true
	}
	return len(normalized) >= 3 && ((normalized[0] >= 'A' && normalized[0] <= 'Z') || (normalized[0] >= 'a' && normalized[0] <= 'z')) && normalized[1] == ':' && normalized[2] == '/'
}

func ensurePathWithinRoot(absoluteRoot, repositoryPath string) error {
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		resolvedRoot = absoluteRoot
	}
	candidate := filepath.Join(absoluteRoot, filepath.FromSlash(strings.TrimPrefix(repositoryPath, "/")))
	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		// Missing or inaccessible paths are handled by the analyzability policy
		// and surfaced as scanner warnings.
		return nil
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedCandidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("incremental file %q resolves outside repository root", repositoryPath)
	}
	return nil
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
	for _, message := range result.Warnings {
		converted.Warnings = append(converted.Warnings, Warning{AnalyzerID: a.id, Message: message})
	}
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
