package complexity

import (
	"context"
	"fmt"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/kotlin"
)

type KotlinAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewKotlinAnalyzer(thresholds models.ComplexityThresholds) *KotlinAnalyzer {
	return &KotlinAnalyzer{
		thresholds: thresholds,
	}
}

func (a *KotlinAnalyzer) Language() string {
	return "Kotlin"
}
func (a *KotlinAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	parser := sitter.NewParser()
	parser.SetLanguage(kotlin.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	if tree == nil {
		return metrics, nil
	}
	defer tree.Close()

	root := tree.RootNode()

	functions, err := findKotlinFunctions(root, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse kotlin functions: %w", err)
	}

	for _, fn := range functions {
		nodes := mapKotlinNodes(fn.node, content)
		cyclomatic, cognitive, nesting := CalculateComplexity(nodes)
		loc := strings.Count(fn.body, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		debtMinutes := estimateKotlinTechnicalDebt(cyclomatic, cognitive, loc)
		suggestions := generateKotlinRefactoringSuggestions(cyclomatic, cognitive, nesting, fn.paramCount, loc)

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

type kotlinFunctionInfo struct {
	name       string
	line       int
	endLine    int
	body       string
	paramCount int
	node       *sitter.Node
}

func findKotlinFunctions(root *sitter.Node, content []byte) ([]kotlinFunctionInfo, error) {

	queryStr := `
		(function_declaration
			(simple_identifier)? @name
			(function_body)? @body
		) @function
		(anonymous_function
			(function_body)? @body
		) @lambda
		(lambda_literal) @lambda
	`

	q, err := sitter.NewQuery([]byte(queryStr), kotlin.GetLanguage())
	if err != nil {
		return nil, err
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, root)

	var functions []kotlinFunctionInfo

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		var fnName string
		var fnBodyNode *sitter.Node
		var fnNode *sitter.Node
		var paramCount int

		for _, c := range m.Captures {
			captureName := q.CaptureNameForId(c.Index)
			switch captureName {
			case "function", "lambda":
				fnNode = c.Node
			case "name":
				fnName = c.Node.Content(content)
			case "body":
				fnBodyNode = c.Node
			}
		}

		if fnNode != nil {
			if fnName == "" {
				fnName = "<lambda>"
			}
			if fnBodyNode == nil {
				fnBodyNode = fnNode
			}

			paramCount = countKotlinParameters(fnNode)

			functions = append(functions, kotlinFunctionInfo{
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

func countKotlinParameters(fnNode *sitter.Node) int {
	count := 0
	for i := 0; i < int(fnNode.ChildCount()); i++ {
		child := fnNode.Child(i)
		if child.Type() == "function_value_parameters" {
			for j := 0; j < int(child.ChildCount()); j++ {
				grandChild := child.Child(j)
				if grandChild.Type() == "parameter" || grandChild.Type() == "class_parameter" {
					count++
				}
			}
			break
		}
	}
	return count
}

func mapKotlinNodes(node *sitter.Node, content []byte) []Node {
	var nodes []Node
	var visit func(n *sitter.Node, depth int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		t := n.Type()

		switch t {
		case "if_expression", "when_entry", "catch_block", "elvis_expression":
			nodes = append(nodes, Node{Type: Branch, Depth: depth})
			newDepth++
		case "when_expression":
			nodes = append(nodes, Node{Type: Nesting, Depth: depth})
			newDepth++
		case "for_statement", "while_statement", "do_while_statement":
			nodes = append(nodes, Node{Type: Loop, Depth: depth})
			newDepth++
		case "binary_expression":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				text := child.Content(content)
				if text == "&&" || text == "||" {
					nodes = append(nodes, Node{Type: Operator, Depth: depth})
				}
			}
		case "call_expression":
			text := n.Content(content)
			if strings.Contains(text, ".let") || strings.Contains(text, ".run") || strings.Contains(text, ".apply") || strings.Contains(text, ".also") {
				nodes = append(nodes, Node{Type: Branch, Depth: depth})
				newDepth++
			}
		case "navigation_expression", "safe_navigation_expression":
			if t == "safe_navigation_expression" {
				nodes = append(nodes, Node{Type: Branch, Depth: depth})
			} else if t == "navigation_expression" {
				for i := 0; i < int(n.ChildCount()); i++ {
					if n.Child(i).Type() == "safe_navigation_operator" {
						nodes = append(nodes, Node{Type: Branch, Depth: depth})
						break
					}
				}
			}
		case "lambda_literal":
			nodes = append(nodes, Node{Type: Closure, Depth: depth})
			newDepth++
		}

		for i := 0; i < int(n.ChildCount()); i++ {
			visit(n.Child(i), newDepth)
		}
	}

	visit(node, 0)
	return nodes
}



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
