package complexity

import (
	"regexp"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

// CCppAnalyzer analyzes C/C++ code for complexity metrics
type CCppAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewCCppAnalyzer creates a new C/C++ complexity analyzer
func NewCCppAnalyzer(thresholds models.ComplexityThresholds) *CCppAnalyzer {
	return &CCppAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *CCppAnalyzer) Language() string {
	return "C/C++"
}

// AnalyzeFile analyzes a C/C++ file and returns complexity metrics
func (a *CCppAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	code := string(content)
	var metrics []models.ComplexityMetric

	// Remove comments first to avoid false positives
	code = removeComments(code)

	// Find all C/C++ function definitions
	functions := findCCppFunctions(code)

	for _, fn := range functions {
		cyclomatic := calculateCCppCyclomatic(fn.body)
		cognitive := calculateCCppCognitive(fn.body)
		nesting := calculatePatternBasedNesting(fn.body)
		loc := strings.Count(fn.body, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		debtMinutes := estimateCCppTechnicalDebt(cyclomatic, cognitive, loc)
		suggestions := generateCCppRefactoringSuggestions(cyclomatic, cognitive, nesting, fn.paramCount, loc)

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

// removeComments removes C/C++ style comments to avoid false positives
func removeComments(code string) string {
	// Remove multi-line comments /* ... */
	multiLineComment := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	code = multiLineComment.ReplaceAllString(code, "")

	// Remove single-line comments //
	singleLineComment := regexp.MustCompile(`//.*`)
	code = singleLineComment.ReplaceAllString(code, "")

	return code
}

// findCCppFunctions finds function definitions in C/C++ code
func findCCppFunctions(code string) []functionInfo {
	var functions []functionInfo
	lines := strings.Split(code, "\n")

	funcPattern := regexp.MustCompile(`^\s*(?:template\s*<[^>]*>\s*)?(?:(?:inline|static|virtual|explicit|extern|friend)\s+)*(?:(?:const|unsigned|signed|long|short)\s+)*(?:\w+(?:::\w+)*(?:\s*<[^>]*>)?)\s*[\*&]*\s+(\w+)\s*\(([^)]*)\)\s*(?:const\s*)?(?:override\s*)?(?:noexcept\s*)?(?:\{|$)`)

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		if matches := funcPattern.FindStringSubmatch(line); len(matches) > 1 {
			funcName := matches[1]

			if isKeyword(funcName) {
				continue
			}

			paramCount := 0

			if len(matches) > 2 && matches[2] != "" {
				paramCount = countCCppParameters(matches[2])
			}
			body := extractFunctionBody(lines, i, 1000)

			if strings.Count(body, "\n") < 2 || !strings.Contains(body, "{") {
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

func isKeyword(name string) bool {
	keywords := []string{
		"if", "else", "while", "for", "switch", "case", "default",
		"return", "break", "continue", "goto", "do", "try", "catch",
		"throw", "class", "struct", "enum", "union", "namespace",
		"template", "typename", "typedef", "using", "public", "private",
		"protected", "virtual", "override", "final", "static", "const",
		"volatile", "inline", "extern", "register", "auto", "mutable",
	}

	for _, keyword := range keywords {
		if name == keyword {
			return true
		}
	}
	return false
}

func countCCppParameters(params string) int {
	params = strings.TrimSpace(params)

	if params == "" || params == "void" {
		return 0
	}
	params = regexp.MustCompile(`=\s*[^,)]+`).ReplaceAllString(params, "")

	depth := 0
	count := 0
	lastSplit := 0

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
		}
	}

	if strings.TrimSpace(params[lastSplit:]) != "" {
		count++
	}

	return count
}

func calculateCCppCyclomatic(code string) int {
	complexity := 1

	patterns := []string{
		`\bif\b`,        // if statement
		`\belse\s+if\b`, // else if
		`\bwhile\b`,     // while loop
		`\bfor\b`,       // for loop
		`\bdo\b`,        // do-while loop
		`\bcase\b`,      // switch case
		`\bcatch\b`,     // exception handling
		`&&`,            // logical AND
		`\|\|`,          // logical OR
		`\?`,            // ternary operator
		`\bgoto\b`,      // goto statement (adds complexity)
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		complexity += len(matches)
	}

	return complexity
}

func calculateCCppCognitive(code string) int {
	cognitive := 0
	nestingPatterns := []string{
		`\bif\b`, `\bwhile\b`, `\bfor\b`, `\bswitch\b`, `\bdo\b`, `\btry\b`,
	}

	for _, pattern := range nestingPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		cognitive += len(matches) * 2
	}

	logicalOps := regexp.MustCompile(`&&|\|\|`)
	cognitive += len(logicalOps.FindAllString(code, -1))

	gotoPattern := regexp.MustCompile(`\bgoto\b`)
	cognitive += len(gotoPattern.FindAllString(code, -1)) * 4

	pointerOps := regexp.MustCompile(`\*\w+|\w+\*|->|->\*|\.\*`)
	cognitive += len(pointerOps.FindAllString(code, -1)) / 3

	templatePattern := regexp.MustCompile(`template\s*<`)
	cognitive += len(templatePattern.FindAllString(code, -1)) * 3

	macroPattern := regexp.MustCompile(`#define`)
	cognitive += len(macroPattern.FindAllString(code, -1)) * 2

	return cognitive
}

func generateCCppRefactoringSuggestions(cyclomatic, cognitive, nesting, paramCount, loc int) []models.RefactoringSuggestion {
	var suggestions []models.RefactoringSuggestion

	if cyclomatic > 15 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Extract Functions",
			Description: "Break down this function into smaller, focused functions. Consider using inline functions for performance-critical paths",
		})
	}

	if nesting > 3 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Reduce Nesting Depth",
			Description: "Use early returns, guard clauses, or extract nested logic into helper functions",
		})
	}

	if paramCount > 5 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Too Many Parameters",
			Description: "Consider using a struct/class to group related parameters or use parameter objects",
		})
	}

	if loc > 50 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Function Too Long",
			Description: "Split this function into smaller functions. Consider separating algorithm from data structure manipulation",
		})
	}

	if cognitive > 20 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Simplify Logic",
			Description: "Simplify control flow, reduce pointer complexity, or use RAII patterns to improve readability",
		})
	}

	return suggestions
}

func estimateCCppTechnicalDebt(cyclomatic, cognitive, loc int) int {
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

	cppTax := 3

	return baseMinutes + complexityMinutes + cognitiveMinutes + locMinutes + cppTax
}
