package analyzers

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/analysis/analyzers/complexity"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/filepolicy"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/scancore"
	"github.com/google/uuid"
)

// ComplexityAnalyzer implements the Analyzer interface for complexity analysis
type ComplexityAnalyzer struct {
	factory *complexity.Factory
}

// NewComplexityAnalyzer creates a new complexity analyzer
func NewComplexityAnalyzer() *ComplexityAnalyzer {
	thresholds := models.DefaultComplexityThresholds()
	factory := complexity.NewFactory(thresholds)

	return &ComplexityAnalyzer{
		factory: factory,
	}
}

func (a *ComplexityAnalyzer) Name() string {
	return "ComplexityAnalyzer"
}

func (a *ComplexityAnalyzer) Analyze(ctx context.Context, repo *git.Repository) (*scancore.Result, error) {
	analysisRunID, ok := ctx.Value("analysisRunID").(uuid.UUID)
	if !ok {
		return nil, fmt.Errorf("analysisRunID not found in context")
	}

	repositoryID, ok := ctx.Value("repositoryID").(uuid.UUID)
	if !ok {
		return nil, fmt.Errorf("repositoryID not found in context")
	}

	userID, ok := ctx.Value("userID").(uuid.UUID)
	if !ok {
		return nil, fmt.Errorf("userID not found in context")
	}

	var targetFilesMap map[string]bool
	if targetFiles, incremental, ok := scancore.TargetFiles(ctx); ok {
		targetFilesMap = make(map[string]bool)
		for _, f := range targetFiles {
			targetFilesMap[f] = true
		}
		if incremental {
			log.Printf("🔬 Incremental analysis: targeting %d changed files", len(targetFiles))
		}
	}

	config, ok := ctx.Value("complexityConfig").(models.ComplexityConfig)
	if !ok {
		config = models.DefaultComplexityConfig()
	}

	allMetrics := []models.ComplexityMetric{}

	err := filepath.Walk(repo.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			return nil
		}
		if info.IsDir() {
			dirName := filepath.Base(path)
			if filepolicy.IsGeneratedDirectory(dirName) {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(repo.Path, path)
		if err != nil {
			relPath = path
		}
		relPath = "/" + strings.TrimPrefix(filepath.ToSlash(relPath), "/")

		if targetFilesMap != nil && !targetFilesMap[relPath] {
			if ctx.Value("isCLI") != true {
				log.Printf("🔍 Skipping %s - not in target files", relPath)
			}
			return nil
		}

		if !a.factory.IsSupported(path) {
			if ctx.Value("isCLI") != true {
				log.Printf("🔍 Skipping %s - unsupported file type", relPath)
			}
			return nil
		}

		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			return nil
		}

		content, err := ioutil.ReadFile(path)
		if err != nil {
			if ctx.Value("isCLI") != true {
				log.Printf("⚠️  Failed to read file %s: %v", path, err)
			}
			return nil
		}

		analyzer, err := a.factory.GetAnalyzer(path)
		if err != nil {
			return nil
		}

		metrics, err := analyzer.AnalyzeFile(relPath, content)
		if err != nil {
			if ctx.Value("isCLI") != true {
				log.Printf("⚠️  Failed to analyze file %s: %v", relPath, err)
			}
			return nil
		}

		if len(metrics) > 0 {
			if ctx.Value("isCLI") != true {
				log.Printf("🔍 Analyzed %s - found %d functions", relPath, len(metrics))
			}
		}

		for i := range metrics {
			metrics[i].ID = uuid.New()
			metrics[i].UserID = userID
			metrics[i].RepositoryID = repositoryID
			metrics[i].AnalysisRunID = analysisRunID

			debtHours := a.CalculateDebt(metrics[i].CyclomaticComplexity, config)
			metrics[i].TechnicalDebtMinutes = int(debtHours * 60)
		}

		allMetrics = append(allMetrics, metrics...)

		return nil
	})

	if err != nil {
		if ctx.Value("isCLI") != true {
			log.Printf("❌ Error walking repository: %v", err)
		}
		return &scancore.Result{
			Issues:  []models.TechnicalDebtIssue{},
			Metrics: map[string]interface{}{},
		}, err
	}

	if ctx.Value("isCLI") != true {
		log.Printf("✅ Analyzed %d functions across repository", len(allMetrics))
	}

	if config.AnalysisMode == "legacy" {
		filtered := allMetrics[:0]
		for _, m := range allMetrics {
			switch m.FunctionName {
			case "<anonymous>", "<lambda>", "<closure>", "<block>":
				continue
			}
			filtered = append(filtered, m)
		}
		if ctx.Value("isCLI") != true {
			log.Printf("🔀 Legacy mode: filtered %d anonymous constructs from scoring", len(allMetrics)-len(filtered))
		}
		allMetrics = filtered
	}

	issues := a.convertToIssues(allMetrics)
	summary := a.calculateSummary(allMetrics)

	return &scancore.Result{
		Issues:  issues,
		Metrics: summary,
	}, nil
}

func (a *ComplexityAnalyzer) CalculateDebt(complexity int, config models.ComplexityConfig) float64 {
	if complexity <= config.CyclomaticThreshold {
		return 0
	}
	return float64((complexity-config.CyclomaticThreshold)*config.CostPerPoint) / 60.0
}

func (a *ComplexityAnalyzer) convertToIssues(metrics []models.ComplexityMetric) []models.TechnicalDebtIssue {
	issues := []models.TechnicalDebtIssue{}

	for _, metric := range metrics {
		if metric.Severity != "high" && metric.Severity != "critical" {
			continue
		}

		issue := models.TechnicalDebtIssue{
			ID:                 uuid.New(),
			UserID:             metric.UserID,
			RepositoryID:       metric.RepositoryID,
			AnalysisRunID:      metric.AnalysisRunID,
			FilePath:           metric.FilePath,
			LineNumber:         &metric.StartLine,
			IssueType:          "complexity",
			Severity:           metric.Severity,
			Category:           "maintainability",
			Message:            a.formatIssueMessage(metric),
			Description:        a.formatIssueDescription(metric),
			ToolName:           "complexity_analyzer",
			ConfidenceScore:    1.0,
			TechnicalDebtHours: float64(metric.TechnicalDebtMinutes) / 60.0,
			EffortMultiplier:   1.0,
			Status:             "open",
			CodeSnippet:        metric.CodeSnippet,
		}

		issues = append(issues, issue)
	}

	return issues
}

func (a *ComplexityAnalyzer) formatIssueMessage(metric models.ComplexityMetric) string {
	if metric.CyclomaticComplexity > 20 {
		return fmt.Sprintf("Function '%s' has critical cyclomatic complexity of %d (threshold: 20)",
			metric.FunctionName, metric.CyclomaticComplexity)
	}
	if metric.CyclomaticComplexity > 10 {
		return fmt.Sprintf("Function '%s' has high cyclomatic complexity of %d (threshold: 10)",
			metric.FunctionName, metric.CyclomaticComplexity)
	}
	if metric.NestingDepth > 5 {
		return fmt.Sprintf("Function '%s' has deep nesting depth of %d (threshold: 5)",
			metric.FunctionName, metric.NestingDepth)
	}
	return fmt.Sprintf("Function '%s' has complexity issues", metric.FunctionName)
}

func (a *ComplexityAnalyzer) formatIssueDescription(metric models.ComplexityMetric) *string {
	var parts []string

	parts = append(parts, fmt.Sprintf("Function: %s", metric.FunctionName))
	parts = append(parts, fmt.Sprintf("Cyclomatic Complexity: %d", metric.CyclomaticComplexity))

	if metric.CognitiveComplexity != nil {
		parts = append(parts, fmt.Sprintf("Cognitive Complexity: %d", *metric.CognitiveComplexity))
	}

	parts = append(parts, fmt.Sprintf("Nesting Depth: %d", metric.NestingDepth))
	parts = append(parts, fmt.Sprintf("Parameters: %d", metric.ParameterCount))
	parts = append(parts, fmt.Sprintf("Lines of Code: %d", metric.LinesOfCode))
	parts = append(parts, fmt.Sprintf("Estimated Refactoring Time: %d minutes", metric.TechnicalDebtMinutes))

	if len(metric.RefactoringSuggestions) > 0 {
		parts = append(parts, "\nRefactoring Suggestions:")
		for _, suggestion := range metric.RefactoringSuggestions {
			parts = append(parts, fmt.Sprintf("- [%s] %s: %s",
				strings.ToUpper(suggestion.Priority), suggestion.Title, suggestion.Description))
		}
	}

	description := strings.Join(parts, "\n")
	return &description
}

func (a *ComplexityAnalyzer) calculateSummary(metrics []models.ComplexityMetric) map[string]interface{} {
	if len(metrics) == 0 {
		return map[string]interface{}{
			"complexity_functions_analyzed": 0,
		}
	}

	totalComplexity := 0
	maxComplexity := 0
	criticalCount := 0
	highCount := 0
	totalDebtMinutes := 0

	for _, metric := range metrics {
		totalComplexity += metric.CyclomaticComplexity
		if metric.CyclomaticComplexity > maxComplexity {
			maxComplexity = metric.CyclomaticComplexity
		}
		switch metric.Severity {
		case "critical":
			criticalCount++
		case "high":
			highCount++
		}
		totalDebtMinutes += metric.TechnicalDebtMinutes
	}

	return map[string]interface{}{
		"complexity_functions_analyzed": len(metrics),
		"complexity_avg_cyclomatic":     float64(totalComplexity) / float64(len(metrics)),
		"complexity_max_cyclomatic":     maxComplexity,
		"complexity_critical_functions": criticalCount,
		"complexity_high_functions":     highCount,
		"complexity_total_debt_hours":   float64(totalDebtMinutes) / 60.0,
	}
}
