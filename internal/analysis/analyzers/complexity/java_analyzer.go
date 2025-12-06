package complexity

import (
	"regexp"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

// JavaAnalyzer analyzes Java code for complexity metrics
type JavaAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewJavaAnalyzer creates a new Java complexity analyzer
func NewJavaAnalyzer(thresholds models.ComplexityThresholds) *JavaAnalyzer {
	return &JavaAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *JavaAnalyzer) Language() string {
	return "Java"
}

// AnalyzeFile analyzes a Java file and returns complexity metrics
func (a *JavaAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	code := string(content)
	var metrics []models.ComplexityMetric

	functions := findJavaFunctions(code)

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
			EndLine:              fn.line + strings.Count(fn.body, "\n"),
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

// findJavaFunctions finds method definitions in Java code
func findJavaFunctions(code string) []functionInfo {
	var functions []functionInfo
	lines := strings.Split(code, "\n")

	methodPattern := regexp.MustCompile(`^\s*(?:(?:public|protected|private|static|final|native|synchronized|abstract|transient)\s+)*[\w<>[\]]+\s+(\w+)\s*\((.*?)\)(?:\s*throws\s+[\w,\s]+)?\s*\{?`)

	for i, line := range lines {
		if strings.Contains(line, "class ") || strings.Contains(line, "interface ") || strings.Contains(line, "enum ") {
			continue
		}

		if matches := methodPattern.FindStringSubmatch(line); len(matches) > 1 {
			funcName := matches[1]
			paramCount := 0
			if matches[2] != "" {
				paramCount = countParameterString(matches[2])
			}

			body := extractFunctionBody(lines, i, 100)

			functions = append(functions, functionInfo{
				name:       funcName,
				line:       i + 1,
				endLine:    i + 1 + strings.Count(body, "\n"),
				body:       body,
				paramCount: paramCount,
			})
		}
	}

	return functions
}
