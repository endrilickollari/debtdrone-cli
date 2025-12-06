package complexity

import (
	"regexp"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

// SwiftAnalyzer analyzes Swift code for complexity metrics
type SwiftAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewSwiftAnalyzer creates a new Swift complexity analyzer
func NewSwiftAnalyzer(thresholds models.ComplexityThresholds) *SwiftAnalyzer {
	return &SwiftAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *SwiftAnalyzer) Language() string {
	return "Swift"
}

// AnalyzeFile analyzes a Swift file and returns complexity metrics
func (a *SwiftAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	code := string(content)
	var metrics []models.ComplexityMetric

	functions := findSwiftFunctions(code)

	for _, fn := range functions {
		cyclomatic := calculateSwiftCyclomatic(fn.body)
		cognitive := calculateSwiftCognitive(fn.body)
		nesting := calculatePatternBasedNesting(fn.body)
		loc := strings.Count(fn.body, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		debtMinutes := estimateSwiftTechnicalDebt(cyclomatic, cognitive, loc)
		suggestions := generateSwiftRefactoringSuggestions(cyclomatic, cognitive, nesting, fn.paramCount, loc)

		cognitivePtr := cognitive
		snippetStr := truncateSnippet(fn.body, 300)

		metric := models.ComplexityMetric{
			ID:                     uuid.New(),
			FilePath:               filePath,
			FunctionName:           fn.name,
			StartLine:              fn.line,
			EndLine:                fn.line + strings.Count(fn.body, "\n"),
			CyclomaticComplexity:   cyclomatic,
			CognitiveComplexity:    &cognitivePtr,
			NestingDepth:           nesting,
			ParameterCount:         fn.paramCount,
			LinesOfCode:            loc,
			Severity:               severity,
			TechnicalDebtMinutes:   debtMinutes,
			RefactoringSuggestions: suggestions,
			CodeSnippet:            &snippetStr,
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

func findSwiftFunctions(code string) []functionInfo {
	var functions []functionInfo
	lines := strings.Split(code, "\n")

	funcPattern := regexp.MustCompile(`^\s*(?:@\w+\s+)*(?:(?:public|private|internal|fileprivate|open)\s+)?(?:static\s+)?(?:class\s+)?(?:override\s+)?(?:mutating\s+)?(?:func\s+(\w+)|init)\s*(?:<[^>]+>)?\s*(\([^)]*\))`)

	for i, line := range lines {
		if matches := funcPattern.FindStringSubmatch(line); len(matches) > 0 {
			funcName := "init"
			if len(matches) > 1 && matches[1] != "" {
				funcName = matches[1]
			}
			paramCount := 0

			if len(matches) > 2 && matches[2] != "" {
				params := strings.Trim(matches[2], "()")
				paramCount = countSwiftParameters(params)
			}

			body := extractFunctionBody(lines, i, 500)

			if strings.Count(body, "\n") < 1 {
				continue
			}

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

// countSwiftParameters counts parameters in a Swift parameter list
func countSwiftParameters(params string) int {
	if strings.TrimSpace(params) == "" {
		return 0
	}

	params = regexp.MustCompile(`=\s*[^,)]+`).ReplaceAllString(params, "")

	depth := 0
	count := 0
	lastSplit := 0

	for i, char := range params {
		if char == '<' || char == '(' || char == '{' || char == '[' {
			depth++
		} else if char == '>' || char == ')' || char == '}' || char == ']' {
			depth--
		} else if char == ',' && depth == 0 {
			if strings.TrimSpace(params[lastSplit:i]) != "" {
				count++
			}
			lastSplit = i + 1
		}
	}

	if strings.TrimSpace(params[lastSplit:]) != "" {
		count++
	}

	return count
}

// calculateSwiftCyclomatic calculates cyclomatic complexity for Swift code
func calculateSwiftCyclomatic(code string) int {
	complexity := 1

	patterns := []string{
		`\bif\b`,         // if statement
		`\belse\s+if\b`,  // else if
		`\bguard\b`,      // guard statement
		`\bswitch\b`,     // switch statement
		`\bcase\b`,       // switch case
		`\bwhile\b`,      // while loop
		`\bfor\b`,        // for loop
		`\brepeat\b`,     // repeat-while loop
		`\bcatch\b`,      // catch block
		`&&`,             // logical AND
		`\|\|`,           // logical OR
		`\?\?`,           // nil coalescing operator
		`\?\.`,           // optional chaining
		`\btry\?`,        // optional try
		`\.map\b`,        // map (functional)
		`\.filter\b`,     // filter
		`\.compactMap\b`, // compactMap
		`\.flatMap\b`,    // flatMap
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		complexity += len(matches)
	}

	return complexity
}

// calculateSwiftCognitive calculates cognitive complexity for Swift code
func calculateSwiftCognitive(code string) int {
	cognitive := 0

	nestingPatterns := []string{
		`\bif\b`, `\bguard\b`, `\bswitch\b`, `\bwhile\b`, `\bfor\b`, `\brepeat\b`, `\bdo\b`,
	}

	for _, pattern := range nestingPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		cognitive += len(matches) * 2
	}

	logicalOps := regexp.MustCompile(`&&|\|\|`)
	cognitive += len(logicalOps.FindAllString(code, -1))

	switchCases := regexp.MustCompile(`\bcase\b`)
	cognitive += len(switchCases.FindAllString(code, -1))

	guardPattern := regexp.MustCompile(`\bguard\b`)
	cognitive += len(guardPattern.FindAllString(code, -1))
	optionalHandling := regexp.MustCompile(`\?\?|\?\.|\btry\?|\btry!`)
	cognitive += len(optionalHandling.FindAllString(code, -1))
	closuresAsync := regexp.MustCompile(`\basync\b|\bawait\b|\{\s*\w+\s+in`)
	cognitive += len(closuresAsync.FindAllString(code, -1)) * 2

	return cognitive
}

// generateSwiftRefactoringSuggestions generates refactoring suggestions for Swift functions
func generateSwiftRefactoringSuggestions(cyclomatic, cognitive, nesting, paramCount, loc int) []models.RefactoringSuggestion {
	var suggestions []models.RefactoringSuggestion

	if cyclomatic > 15 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Extract Methods",
			Description: "Break down this function into smaller, focused methods. Consider using extension methods or protocols",
		})
	}

	if nesting > 3 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Reduce Nesting Depth",
			Description: "Use guard statements, optional chaining (?.), or early returns to reduce nesting",
		})
	}

	if paramCount > 4 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Use Struct or Builder Pattern",
			Description: "Too many parameters. Consider using a struct with default values or a builder pattern",
		})
	}

	if loc > 50 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Function Too Long",
			Description: "Split this function into smaller methods. Consider extracting logic into extensions or separate types",
		})
	}

	if cognitive > 20 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Simplify Logic",
			Description: "Use Swift's Result type, optional chaining, guard statements, or functional methods (map, flatMap) to simplify logic",
		})
	}

	return suggestions
}

func estimateSwiftTechnicalDebt(cyclomatic, cognitive, loc int) int {
	baseMinutes := 5

	complexityMinutes := (cyclomatic - 10) * 2
	if complexityMinutes < 0 {
		complexityMinutes = 0
	}

	cognitiveMinutes := (cognitive - 15) * 1
	if cognitiveMinutes < 0 {
		cognitiveMinutes = 0
	}

	locMinutes := (loc - 30) / 5
	if locMinutes < 0 {
		locMinutes = 0
	}

	return baseMinutes + complexityMinutes + cognitiveMinutes + locMinutes
}
