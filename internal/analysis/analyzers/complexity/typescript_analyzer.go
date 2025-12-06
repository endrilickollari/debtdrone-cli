package complexity

import (
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

// TypeScriptAnalyzer analyzes TypeScript/TSX code for complexity metrics
type TypeScriptAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewTypeScriptAnalyzer creates a new TypeScript complexity analyzer
func NewTypeScriptAnalyzer(thresholds models.ComplexityThresholds) *TypeScriptAnalyzer {
	return &TypeScriptAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *TypeScriptAnalyzer) Language() string {
	return "TypeScript"
}

// AnalyzeFile analyzes a TypeScript file and returns complexity metrics
func (a *TypeScriptAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	functions := findJavaScriptFunctions(content)

	for _, fn := range functions {
		cyclomatic := calculatePatternBasedCyclomatic(fn.body)
		cognitive := calculatePatternBasedCognitive(fn.body)
		nesting := calculatePatternBasedNesting(fn.body)

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)

		cognitivePtr := cognitive
		snippetStr := truncateSnippet(fn.body, 300)

		metric := models.ComplexityMetric{
			ID:                   uuid.New(),
			FilePath:             filePath,
			FunctionName:         fn.name,
			StartLine:            fn.line,
			EndLine:              fn.endLine,
			CyclomaticComplexity: cyclomatic,
			CognitiveComplexity:  &cognitivePtr,
			NestingDepth:         nesting,
			ParameterCount:       fn.paramCount,
			Severity:             severity,
			CodeSnippet:          &snippetStr,
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}
