package complexity

import (
	"context"
	"fmt"
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

	parser := sitter.NewParser()
	parser.SetLanguage(rust.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	if tree == nil {
		return metrics, nil
	}
	defer tree.Close()

	root := tree.RootNode()

	functions, err := findRustFunctions(root, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rust functions: %w", err)
	}

	for _, fn := range functions {
		nodes := mapRustNodes(fn.node, content)
		cyclomatic, cognitive, nesting := CalculateComplexity(nodes)
		loc := strings.Count(fn.body, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		debtMinutes := estimateRustTechnicalDebt(cyclomatic, cognitive, loc)
		suggestions := generateRustRefactoringSuggestions(cyclomatic, cognitive, nesting, fn.paramCount, loc)

		cognitivePtr := cognitive
		snippetStr := truncateSnippet(fn.body, 10000)

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

func findRustFunctions(root *sitter.Node, content []byte) ([]rustFunctionInfo, error) {

	queryStr := `
		(function_item
			name: (identifier) @name
			parameters: (parameters)? @params
			body: (block) @body
		) @function
		(closure_expression
			body: (_) @body
		) @lambda
	`

	q, err := sitter.NewQuery([]byte(queryStr), rust.GetLanguage())
	if err != nil {
		return nil, err
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, root)

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
			case "function", "lambda":
				fnNode = c.Node
			case "name":
				fnName = c.Node.Content(content)
			case "params":
				paramNode = c.Node
			case "body":
				fnBodyNode = c.Node
			}
		}

		if fnNode != nil && fnBodyNode != nil {
			if fnName == "" {
				fnName = "<closure>"
			}

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

func mapRustNodes(node *sitter.Node, content []byte) []Node {
	var nodes []Node
	var visit func(n *sitter.Node, depth int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		if n.IsNamed() {
			nodeType := n.Type()
			switch nodeType {
			case "if_expression", "match_arm", "try_expression", "or_pattern":
				nodes = append(nodes, Node{Type: Branch, Depth: depth})
				newDepth++
			case "match_expression":
				nodes = append(nodes, Node{Type: Nesting, Depth: depth})
				newDepth++
			case "while_expression", "for_expression", "loop_expression":
				nodes = append(nodes, Node{Type: Loop, Depth: depth})
				newDepth++
			case "closure_expression":
				nodes = append(nodes, Node{Type: Closure, Depth: depth})
				newDepth++
			case "binary_expression":
				for i := 0; i < int(n.ChildCount()); i++ {
					child := n.Child(i)
					op := child.Content(content)
					switch op {
					case "&&", "||":
						nodes = append(nodes, Node{Type: Operator, Depth: depth})
					}
				}
			}
		}

		for i := 0; i < int(n.ChildCount()); i++ {
			visit(n.Child(i), newDepth)
		}
	}

	visit(node, 0)
	return nodes
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
