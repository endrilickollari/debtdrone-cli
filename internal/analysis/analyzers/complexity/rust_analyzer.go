package complexity

import (
	"regexp"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

// RustAnalyzer analyzes Rust code for complexity metrics
type RustAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewRustAnalyzer creates a new Rust complexity analyzer
func NewRustAnalyzer(thresholds models.ComplexityThresholds) *RustAnalyzer {
	return &RustAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *RustAnalyzer) Language() string {
	return "Rust"
}

// AnalyzeFile analyzes a Rust file and returns complexity metrics
func (a *RustAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	code := string(content)
	var metrics []models.ComplexityMetric

	functions := findRustFunctions(code)

	for _, fn := range functions {
		cyclomatic := calculateRustCyclomatic(fn.body)
		cognitive := calculateRustCognitive(fn.body)
		nesting := calculateRustNesting(fn.body)
		loc := strings.Count(fn.body, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		debtMinutes := estimateRustTechnicalDebt(cyclomatic, cognitive, loc)
		suggestions := generateRustRefactoringSuggestions(cyclomatic, cognitive, nesting, fn.paramCount, loc)

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

// findRustFunctions finds function definitions in Rust code
func findRustFunctions(code string) []functionInfo {
	var functions []functionInfo
	lines := strings.Split(code, "\n")

	funcPattern := regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:unsafe\s+)?(?:const\s+)?fn\s+(\w+)\s*(?:<[^>]*>)?\s*\(([^)]*)\)`)

	for i, line := range lines {
		if matches := funcPattern.FindStringSubmatch(line); len(matches) > 1 {
			funcName := matches[1]
			paramCount := 0

			if len(matches) > 2 && matches[2] != "" {
				paramCount = countRustParameters(matches[2])
			}
			body := extractRustFunctionBody(lines, i)

			if strings.Count(body, "\n") < 2 {
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

// extractRustFunctionBody extracts the body of a Rust function
func extractRustFunctionBody(lines []string, startLine int) string {
	var body []string
	body = append(body, lines[startLine])

	braceCount := 0
	foundOpenBrace := false

	for i := startLine; i < len(lines); i++ {
		line := lines[i]

		for _, char := range line {
			switch char {
			case '{':
				braceCount++
				foundOpenBrace = true
			case '}':
				braceCount--
			}
		}

		if i > startLine {
			body = append(body, line)
		}

		if foundOpenBrace && braceCount == 0 {
			break
		}
		if i == startLine && strings.TrimSpace(line)[len(strings.TrimSpace(line))-1] == ';' {
			break
		}
	}

	return strings.Join(body, "\n")
}

func countRustParameters(params string) int {
	if strings.TrimSpace(params) == "" {
		return 0
	}
	params = regexp.MustCompile(`\b(?:&mut\s+)?self\b,?\s*`).ReplaceAllString(params, "")
	if strings.TrimSpace(params) == "" {
		return 0
	}
	depth := 0
	count := 0
	lastSplit := 0
	hasParam := false

	for i, char := range params {
		if char == '<' || char == '(' {
			depth++
		} else if char == '>' || char == ')' {
			depth--
		} else if char == ',' && depth == 0 {
			if strings.TrimSpace(params[lastSplit:i]) != "" {
				count++
			}
			lastSplit = i + 1
			hasParam = false
		} else if !hasParam && char != ' ' && char != '\t' && char != '\n' {
			hasParam = true
		}
	}

	if strings.TrimSpace(params[lastSplit:]) != "" {
		count++
	}

	return count
}

// calculateRustCyclomatic calculates cyclomatic complexity for Rust code
func calculateRustCyclomatic(code string) int {
	complexity := 1

	patterns := []string{
		`\bif\b`,        // if statement
		`\belse\s+if\b`, // else if
		`\bmatch\b`,     // match expression
		`\bwhile\b`,     // while loop
		`\bfor\b`,       // for loop
		`\bloop\b`,      // infinite loop
		`=>`,            // match arm
		`&&`,            // logical AND
		`\|\|`,          // logical OR
		`\?`,            // ? operator (error propagation)
		`\.unwrap_or\b`, // unwrap_or
		`\.and_then\b`,  // and_then combinator
		`\.or_else\b`,   // or_else combinator
		`\.map\b`,       // map (functional)
		`\.filter\b`,    // filter
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		complexity += len(matches)
	}

	return complexity
}

// calculateRustCognitive calculates cognitive complexity for Rust code
func calculateRustCognitive(code string) int {
	cognitive := 0

	nestingPatterns := []string{
		`\bif\b`, `\bmatch\b`, `\bwhile\b`, `\bfor\b`, `\bloop\b`,
	}

	for _, pattern := range nestingPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		cognitive += len(matches) * 2
	}

	logicalOps := regexp.MustCompile(`&&|\|\|`)
	cognitive += len(logicalOps.FindAllString(code, -1))

	matchArms := regexp.MustCompile(`=>`)
	cognitive += len(matchArms.FindAllString(code, -1))

	lifetimePattern := regexp.MustCompile(`'[a-z]\w*`)
	cognitive += len(lifetimePattern.FindAllString(code, -1)) / 2

	unsafePattern := regexp.MustCompile(`\bunsafe\b`)
	cognitive += len(unsafePattern.FindAllString(code, -1)) * 3

	return cognitive
}

// calculateRustNesting estimates maximum nesting depth for Rust code
func calculateRustNesting(code string) int {
	maxDepth := 0
	currentDepth := 0

	for _, char := range code {
		switch char {
		case '{':
			currentDepth++
			if currentDepth > maxDepth {
				maxDepth = currentDepth
			}
		case '}':
			currentDepth--
			if currentDepth < 0 {
				currentDepth = 0
			}
		}
	}

	return maxDepth
}

// generateRustRefactoringSuggestions generates refactoring suggestions for Rust functions
func generateRustRefactoringSuggestions(cyclomatic, cognitive, nesting, paramCount, loc int) []models.RefactoringSuggestion {
	var suggestions []models.RefactoringSuggestion

	if cyclomatic > 15 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Extract Functions",
			Description: "Break down this function into smaller, focused functions. Consider using private helper functions or modules",
		})
	}

	if nesting > 3 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Reduce Nesting Depth",
			Description: "Use early returns with ? operator, if let, or match guards to reduce nesting",
		})
	}

	if paramCount > 4 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Consider Builder Pattern or Struct",
			Description: "Too many parameters. Consider using a builder pattern or passing a configuration struct",
		})
	}

	if loc > 50 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Function Too Long",
			Description: "Split this function into smaller functions. Consider extracting logic into separate modules or traits",
		})
	}

	if cognitive > 20 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Simplify Logic",
			Description: "Use Rust's Result and Option combinators (map, and_then, unwrap_or) to simplify control flow",
		})
	}

	return suggestions
}

func estimateRustTechnicalDebt(cyclomatic, cognitive, loc int) int {
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

	rustTax := 2

	return baseMinutes + complexityMinutes + cognitiveMinutes + locMinutes + rustTax
}
