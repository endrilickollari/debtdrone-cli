package complexity

import (
	"regexp"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

// PHPAnalyzer analyzes PHP code for complexity metrics
type PHPAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewPHPAnalyzer creates a new PHP complexity analyzer
func NewPHPAnalyzer(thresholds models.ComplexityThresholds) *PHPAnalyzer {
	return &PHPAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *PHPAnalyzer) Language() string {
	return "PHP"
}

// AnalyzeFile analyzes a PHP file and returns complexity metrics
func (a *PHPAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	code := string(content)
	var metrics []models.ComplexityMetric

	functions := findPHPFunctions(code)

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

// findPHPFunctions finds function definitions in PHP code
func findPHPFunctions(code string) []functionInfo {
	var functions []functionInfo
	lines := strings.Split(code, "\n")

	funcPattern := regexp.MustCompile(`^\s*(?:(?:public|protected|private|static|abstract|final)\s+)*function\s+(\w+)\s*\((.*?)\)\s*(?::\s*[\w\\]+)?\s*\{?`)

	for i, line := range lines {
		if matches := funcPattern.FindStringSubmatch(line); len(matches) > 1 {
			funcName := matches[1]

			if isPHPControlStructure(funcName) {
				continue
			}

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

func isPHPControlStructure(name string) bool {
	switch name {
	case "if", "for", "foreach", "while", "switch", "catch", "try":
		return true
	default:
		return false
	}
}
