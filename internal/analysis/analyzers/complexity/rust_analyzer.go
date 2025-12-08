package complexity

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/rust"
)

type RustAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewRustAnalyzer(thresholds models.ComplexityThresholds) *RustAnalyzer {
	return &RustAnalyzer{
		thresholds: thresholds,
	}
}

func (a *RustAnalyzer) Language() string {
	return "Rust"
}
func (a *RustAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	functions, err := findRustFunctions(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rust functions: %w", err)
	}

	for _, fn := range functions {
		cyclomatic := calculateRustCyclomatic(fn.node, content)
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
			EndLine:                fn.endLine,
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

type rustFunctionInfo struct {
	name       string
	line       int
	endLine    int
	body       string
	paramCount int
	node       *sitter.Node
}

func findRustFunctions(content []byte) ([]rustFunctionInfo, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(rust.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}

	queryStr := `
		(function_item
			name: (identifier) @name
			parameters: (parameters)? @params
			body: (block) @body
		) @function
	`

	q, err := sitter.NewQuery([]byte(queryStr), rust.GetLanguage())
	if err != nil {
		return nil, err
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, tree.RootNode())

	var functions []rustFunctionInfo

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		var fnName string
		var fnBodyNode *sitter.Node
		var fnNode *sitter.Node
		var paramNode *sitter.Node

		for _, c := range m.Captures {
			captureName := q.CaptureNameForId(c.Index)
			switch captureName {
			case "function":
				fnNode = c.Node
			case "name":
				fnName = c.Node.Content(content)
			case "params":
				paramNode = c.Node
			case "body":
				fnBodyNode = c.Node
			}
		}

		if fnNode != nil && fnName != "" && fnBodyNode != nil {
			paramCount := countRustParameters(paramNode)

			functions = append(functions, rustFunctionInfo{
				name:       fnName,
				line:       int(fnNode.StartPoint().Row) + 1,
				endLine:    int(fnNode.EndPoint().Row) + 1,
				body:       fnBodyNode.Content(content),
				paramCount: paramCount,
				node:       fnNode,
			})
		}
	}

	return functions, nil
}

func countRustParameters(paramNode *sitter.Node) int {
	if paramNode == nil {
		return 0
	}
	count := 0
	for i := 0; i < int(paramNode.ChildCount()); i++ {
		child := paramNode.Child(i)
		if child.Type() == "parameter" {
			count++
		}
	}
	return count
}

func calculateRustCyclomatic(node *sitter.Node, content []byte) int {
	complexity := 1
	cursor := sitter.NewTreeCursor(node)
	defer cursor.Close()

	for {
		n := cursor.CurrentNode()

		if n.IsNamed() {
			nodeType := n.Type()
			switch nodeType {
			case "if_expression":
				complexity++
			case "match_expression":
				complexity++
			case "match_arm":
				complexity++
			case "while_expression", "for_expression", "loop_expression":
				complexity++
			case "binary_expression":
				for i := 0; i < int(n.ChildCount()); i++ {
					child := n.Child(i)
					op := child.Content(content)
					switch op {
					case "&&", "||":
						complexity++
					}
				}
			case "try_expression":
				complexity++
			case "or_pattern":
				complexity++
			}
		}

		if cursor.GoToFirstChild() {
			continue
		}
		if cursor.GoToNextSibling() {
			continue
		}
		for cursor.GoToParent() {
			if cursor.GoToNextSibling() {
				goto NextSibling
			}
		}
		break
	NextSibling:
	}

	return complexity
}

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
