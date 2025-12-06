package complexity

import (
	"regexp"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

// CSharpAnalyzer analyzes C# code for complexity metrics
type CSharpAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewCSharpAnalyzer creates a new C# complexity analyzer
func NewCSharpAnalyzer(thresholds models.ComplexityThresholds) *CSharpAnalyzer {
	return &CSharpAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *CSharpAnalyzer) Language() string {
	return "C#"
}

// AnalyzeFile analyzes a C# file and returns complexity metrics
func (a *CSharpAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	code := string(content)
	var metrics []models.ComplexityMetric

	functions := findCSharpFunctions(code)

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

// findCSharpFunctions finds method definitions in C# code
func findCSharpFunctions(code string) []functionInfo {
	var functions []functionInfo
	lines := strings.Split(code, "\n")

	methodPattern := regexp.MustCompile(`^\s*(?:(?:public|protected|private|internal|static|virtual|override|abstract|async|sealed|readonly|unsafe|extern|new|volatile)\s+)*(?:.+?)\s+(\w+)\s*\((.*?)\)\s*\{?`)

	for i, line := range lines {
		if strings.Contains(line, "class ") || strings.Contains(line, "interface ") || strings.Contains(line, "enum ") || strings.Contains(line, "namespace ") || strings.Contains(line, "struct ") {
			continue
		}
		if strings.Contains(line, "get;") || strings.Contains(line, "set;") {
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(line), "return ") {
			continue
		}

		if matches := methodPattern.FindStringSubmatch(line); len(matches) > 1 {
			funcName := matches[1]

			if isControlStructure(funcName) {
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

func isControlStructure(name string) bool {
	switch name {
	case "if", "for", "foreach", "while", "switch", "catch", "using", "lock", "fixed":
		return true
	default:
		return false
	}
}
