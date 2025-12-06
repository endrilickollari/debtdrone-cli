package complexity

import (
	"regexp"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

// RubyAnalyzer analyzes Ruby code for complexity metrics
type RubyAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewRubyAnalyzer creates a new Ruby complexity analyzer
func NewRubyAnalyzer(thresholds models.ComplexityThresholds) *RubyAnalyzer {
	return &RubyAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *RubyAnalyzer) Language() string {
	return "Ruby"
}

// AnalyzeFile analyzes a Ruby file and returns complexity metrics
func (a *RubyAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	code := string(content)
	var metrics []models.ComplexityMetric

	functions := findRubyMethods(code)

	for _, fn := range functions {
		cyclomatic := calculateRubyCyclomatic(fn.body)
		cognitive := calculateRubyCognitive(fn.body)
		nesting := calculateRubyNesting(fn.body)
		loc := strings.Count(fn.body, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		debtMinutes := estimateTechnicalDebt(cyclomatic, cognitive, loc)
		suggestions := generateRubyRefactoringSuggestions(cyclomatic, cognitive, nesting, fn.paramCount, loc)

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

// findRubyMethods finds method definitions in Ruby code
func findRubyMethods(code string) []functionInfo {
	var functions []functionInfo
	lines := strings.Split(code, "\n")

	funcPattern := regexp.MustCompile(`^\s*def\s+(?:self\.)?(\w+[!?]?)\s*(\(.*?\))?`)

	for i, line := range lines {
		if matches := funcPattern.FindStringSubmatch(line); len(matches) > 1 {
			funcName := matches[1]
			paramCount := 0

			if len(matches) > 2 && matches[2] != "" {
				params := strings.Trim(matches[2], "()")
				paramCount = countRubyParameters(params)
			}
			body := extractRubyMethodBody(lines, i)

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

// extractRubyMethodBody extracts the body of a Ruby method
func extractRubyMethodBody(lines []string, startLine int) string {
	var body []string
	defLine := lines[startLine]

	body = append(body, defLine)

	endCount := 0
	defCount := 1

	for i := startLine + 1; i < len(lines); i++ {
		line := lines[i]

		body = append(body, line)

		if regexp.MustCompile(`^\s*(def|class|module|if|unless|case|while|until|for|begin)\b`).MatchString(line) {
			defCount++
		}
		if regexp.MustCompile(`^\s*end\b`).MatchString(line) {
			endCount++
			if endCount >= defCount {
				break
			}
		}
	}

	return strings.Join(body, "\n")
}

// countRubyParameters counts parameters in a Ruby parameter list
func countRubyParameters(params string) int {
	if strings.TrimSpace(params) == "" {
		return 0
	}

	params = regexp.MustCompile(`=\s*[^,]+`).ReplaceAllString(params, "")
	params = regexp.MustCompile(`&\w+`).ReplaceAllString(params, "")

	parts := strings.Split(params, ",")
	count := 0
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			count++
		}
	}

	return count
}

// calculateRubyCyclomatic calculates cyclomatic complexity for Ruby code
func calculateRubyCyclomatic(code string) int {
	complexity := 1

	patterns := []string{
		`\bif\b`,     // if statement
		`\bunless\b`, // unless statement
		`\belsif\b`,  // elsif
		`\bwhen\b`,   // case when
		`\brescue\b`, // exception handling
		`\bwhile\b`,  // while loop
		`\buntil\b`,  // until loop
		`\bfor\b`,    // for loop
		`\.each\b`,   // .each iterator
		`\.map\b`,    // .map iterator
		`\.select\b`, // .select iterator
		`\.reject\b`, // .reject iterator
		`&&`,         // logical AND
		`\|\|`,       // logical OR
		`\band\b`,    // logical and
		`\bor\b`,     // logical or
		`\?`,         // ternary operator
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		complexity += len(matches)
	}

	return complexity
}

// calculateRubyCognitive calculates cognitive complexity for Ruby code
func calculateRubyCognitive(code string) int {
	cognitive := 0

	nestingPatterns := []string{
		`\bif\b`, `\bunless\b`, `\bcase\b`, `\bwhile\b`, `\buntil\b`,
		`\bfor\b`, `\bbegin\b`, `\.each\b`, `\.map\b`,
	}

	for _, pattern := range nestingPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		cognitive += len(matches) * 2
	}

	logicalOps := regexp.MustCompile(`\b&&\b|\b\|\|\b|\band\b|\bor\b`)
	cognitive += len(logicalOps.FindAllString(code, -1))

	rescuePattern := regexp.MustCompile(`\brescue\b`)
	cognitive += len(rescuePattern.FindAllString(code, -1)) * 2

	return cognitive
}

// calculateRubyNesting estimates maximum nesting depth for Ruby code
func calculateRubyNesting(code string) int {
	maxDepth := 0
	currentDepth := 0

	lines := strings.Split(code, "\n")
	for _, line := range lines {
		if regexp.MustCompile(`^\s*(if|unless|case|while|until|for|begin|def|class|module)\b`).MatchString(line) {
			currentDepth++
			if currentDepth > maxDepth {
				maxDepth = currentDepth
			}
		}

		if regexp.MustCompile(`\bdo\b|\{\s*\|`).MatchString(line) {
			currentDepth++
			if currentDepth > maxDepth {
				maxDepth = currentDepth
			}
		}

		if regexp.MustCompile(`^\s*end\b`).MatchString(line) {
			currentDepth--
			if currentDepth < 0 {
				currentDepth = 0
			}
		}

		if regexp.MustCompile(`^\s*\}`).MatchString(line) {
			currentDepth--
			if currentDepth < 0 {
				currentDepth = 0
			}
		}
	}

	return maxDepth
}

// generateRubyRefactoringSuggestions generates refactoring suggestions for Ruby methods
func generateRubyRefactoringSuggestions(cyclomatic, cognitive, nesting, paramCount, loc int) []models.RefactoringSuggestion {
	var suggestions []models.RefactoringSuggestion

	if cyclomatic > 15 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Extract Methods",
			Description: "Break down this method into smaller, focused methods using Ruby's expressive syntax",
		})
	}

	if nesting > 3 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Reduce Nesting Depth",
			Description: "Use Ruby's guard clauses, early returns, or extract nested logic into separate methods",
		})
	}

	if paramCount > 4 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Introduce Parameter Object",
			Description: "Consider using a hash or creating a parameter object to group related parameters",
		})
	}

	if loc > 50 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Method Too Long",
			Description: "Split this method into smaller methods. Consider using Ruby modules or service objects",
		})
	}

	if cognitive > 20 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Simplify Logic",
			Description: "Use Ruby idioms like safe navigation (&.), try, or early returns to simplify logic",
		})
	}

	return suggestions
}

// estimateTechnicalDebt estimates technical debt in minutes
func estimateTechnicalDebt(cyclomatic, cognitive, loc int) int {
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
