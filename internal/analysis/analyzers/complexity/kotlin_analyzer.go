package complexity

import (
	"regexp"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

// KotlinAnalyzer analyzes Kotlin code for complexity metrics
type KotlinAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewKotlinAnalyzer creates a new Kotlin complexity analyzer
func NewKotlinAnalyzer(thresholds models.ComplexityThresholds) *KotlinAnalyzer {
	return &KotlinAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *KotlinAnalyzer) Language() string {
	return "Kotlin"
}

// AnalyzeFile analyzes a Kotlin file and returns complexity metrics
func (a *KotlinAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	code := string(content)
	var metrics []models.ComplexityMetric

	functions := findKotlinFunctions(code)

	for _, fn := range functions {
		cyclomatic := calculateKotlinCyclomatic(fn.body)
		cognitive := calculateKotlinCognitive(fn.body)
		nesting := calculatePatternBasedNesting(fn.body)
		loc := strings.Count(fn.body, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		debtMinutes := estimateKotlinTechnicalDebt(cyclomatic, cognitive, loc)
		suggestions := generateKotlinRefactoringSuggestions(cyclomatic, cognitive, nesting, fn.paramCount, loc)

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

// findKotlinFunctions finds function definitions in Kotlin code
func findKotlinFunctions(code string) []functionInfo {
	var functions []functionInfo
	lines := strings.Split(code, "\n")

	funcPattern := regexp.MustCompile(`^\s*(?:(?:public|private|protected|internal)\s+)?(?:suspend\s+)?(?:inline\s+)?(?:infix\s+)?(?:operator\s+)?fun\s+(?:<[^>]+>\s+)?(?:\w+\.)?(\w+)\s*(\([^)]*\))`)

	for i, line := range lines {
		if matches := funcPattern.FindStringSubmatch(line); len(matches) > 1 {
			funcName := matches[1]
			paramCount := 0

			if len(matches) > 2 && matches[2] != "" {
				params := strings.Trim(matches[2], "()")
				paramCount = countKotlinParameters(params)
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

// countKotlinParameters counts parameters in a Kotlin parameter list
func countKotlinParameters(params string) int {
	if strings.TrimSpace(params) == "" {
		return 0
	}

	params = regexp.MustCompile(`=\s*[^,)]+`).ReplaceAllString(params, "")

	depth := 0
	count := 0
	lastSplit := 0

	for i, char := range params {
		if char == '<' || char == '(' || char == '{' {
			depth++
		} else if char == '>' || char == ')' || char == '}' {
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

// calculateKotlinCyclomatic calculates cyclomatic complexity for Kotlin code
func calculateKotlinCyclomatic(code string) int {
	complexity := 1

	patterns := []string{
		`\bif\b`,         // if statement
		`\belse\s+if\b`,  // else if
		`\bwhen\b`,       // when expression (similar to switch)
		`\bwhile\b`,      // while loop
		`\bfor\b`,        // for loop
		`\bcatch\b`,      // catch block
		`&&`,             // logical AND
		`\|\|`,           // logical OR
		`\?:`,            // Elvis operator
		`\?\.`,           // Safe call operator
		`!!`,             // Not-null assertion (adds risk)
		`\.let\b`,        // let scope function
		`\.also\b`,       // also scope function
		`\.takeIf\b`,     // takeIf
		`\.takeUnless\b`, // takeUnless
		`->`,             // lambda or when branch
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		complexity += len(matches)
	}

	return complexity
}

// calculateKotlinCognitive calculates cognitive complexity for Kotlin code
func calculateKotlinCognitive(code string) int {
	cognitive := 0

	nestingPatterns := []string{
		`\bif\b`, `\bwhen\b`, `\bwhile\b`, `\bfor\b`, `\btry\b`,
	}

	for _, pattern := range nestingPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		cognitive += len(matches) * 2
	}

	logicalOps := regexp.MustCompile(`&&|\|\|`)
	cognitive += len(logicalOps.FindAllString(code, -1))

	whenBranches := regexp.MustCompile(`->`)
	cognitive += len(whenBranches.FindAllString(code, -1))

	nullSafety := regexp.MustCompile(`\?\.|\?:|!!`)
	cognitive += len(nullSafety.FindAllString(code, -1))

	coroutineKeywords := regexp.MustCompile(`\bsuspend\b|\blaunch\b|\basync\b|\bawait\b`)
	cognitive += len(coroutineKeywords.FindAllString(code, -1)) * 2

	return cognitive
}

// generateKotlinRefactoringSuggestions generates refactoring suggestions for Kotlin functions
func generateKotlinRefactoringSuggestions(cyclomatic, cognitive, nesting, paramCount, loc int) []models.RefactoringSuggestion {
	var suggestions []models.RefactoringSuggestion

	if cyclomatic > 15 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Extract Functions",
			Description: "Break down this function into smaller, focused functions. Consider using extension functions or sealed classes",
		})
	}

	if nesting > 3 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Reduce Nesting Depth",
			Description: "Use Kotlin's safe call operators (?.), let, also, or early returns to reduce nesting",
		})
	}

	if paramCount > 4 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Use Data Class or Builder Pattern",
			Description: "Too many parameters. Consider using a data class with named parameters or default values",
		})
	}

	if loc > 50 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Function Too Long",
			Description: "Split this function into smaller functions. Consider using extension functions or separating concerns",
		})
	}

	if cognitive > 20 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Simplify Logic",
			Description: "Use Kotlin's scope functions (let, run, apply), when expressions, or sealed classes to simplify logic",
		})
	}

	return suggestions
}

// estimateKotlinTechnicalDebt estimates technical debt in minutes for Kotlin code
func estimateKotlinTechnicalDebt(cyclomatic, cognitive, loc int) int {
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
