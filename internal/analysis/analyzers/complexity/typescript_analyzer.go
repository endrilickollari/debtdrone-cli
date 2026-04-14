package complexity

import (
	"context"
	"fmt"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

type TypeScriptAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewTypeScriptAnalyzer(thresholds models.ComplexityThresholds) *TypeScriptAnalyzer {
	return &TypeScriptAnalyzer{
		thresholds: thresholds,
	}
}

func (a *TypeScriptAnalyzer) Language() string {
	return "TypeScript"
}

func (a *TypeScriptAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	if tree == nil {
		return metrics, nil
	}
	defer tree.Close()

	root := tree.RootNode()

	functions, err := findTypeScriptFunctions(root, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse typescript functions: %w", err)
	}

	for _, fn := range functions {
		nodes := mapTypeScriptNodes(fn.node, content)
		cyclomatic, cognitive, nesting := CalculateComplexity(nodes)
		loc := strings.Count(fn.body, "\n") + 1
		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		cognitivePtr := cognitive
		snippetStr := truncateSnippet(fn.body, 10000)

		metric := models.ComplexityMetric{
			ID:                   uuid.New(),
			FilePath:             filePath,
			FunctionName:         fn.name,
			StartLine:            fn.line,
			EndLine:              fn.endLine,
			CyclomaticComplexity: cyclomatic,
			CognitiveComplexity:  &cognitivePtr,
			NestingDepth:         nesting,
			ParameterCount:       fn.paramCount,
			LinesOfCode:          loc,
			Severity:             severity,
			CodeSnippet:          &snippetStr,
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

type tsFunctionInfo struct {
	name       string
	line       int
	endLine    int
	body       string
	paramCount int
	node       *sitter.Node
}

func findTypeScriptFunctions(root *sitter.Node, content []byte) ([]tsFunctionInfo, error) {

	queryStr := `
		(function_declaration
			name: (identifier)? @name
			parameters: (formal_parameters)? @params
			body: (_) @body
		) @function

		(function_expression
			name: (identifier)? @name
			parameters: (formal_parameters)? @params
			body: (_) @body
		) @function

		(method_definition
			name: (property_identifier) @name
			parameters: (formal_parameters)? @params
			body: (_) @body
		) @method

		(arrow_function
			parameters: (_)? @params
			body: (_) @body
		) @arrow
	`

	q, err := sitter.NewQuery([]byte(queryStr), typescript.GetLanguage())
	if err != nil {
		return nil, err
	}
	defer q.Close()

	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(q, root)

	var functions []tsFunctionInfo

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
			case "function", "method", "arrow":
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
				parent := fnNode.Parent()
				if parent != nil && parent.Type() == "variable_declarator" {
					for i := 0; i < int(parent.ChildCount()); i++ {
						child := parent.Child(i)
						if child.Type() == "identifier" {
							fnName = child.Content(content)
							break
						}
					}
				}
				if fnName == "" {
					fnName = "<anonymous>"
				}
			}

			paramCount := countTypeScriptParameters(paramNode)

			functions = append(functions, tsFunctionInfo{
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

func countTypeScriptParameters(paramNode *sitter.Node) int {
	if paramNode == nil {
		return 0
	}
	count := 0
	for i := 0; i < int(paramNode.ChildCount()); i++ {
		child := paramNode.Child(i)
		switch child.Type() {
		case "required_parameter", "optional_parameter", "rest_parameter", "parameter_property":
			count++
		case "identifier":
		}
	}
	return count
}

func mapTypeScriptNodes(node *sitter.Node, content []byte) []Node {
	var nodes []Node
	var visit func(n *sitter.Node, depth int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		if n.IsNamed() {
			t := n.Type()
			switch t {
			case "if_statement", "switch_case", "catch_clause", "ternary_expression":
				nodes = append(nodes, Node{Type: Branch, Depth: depth})
				newDepth++
			case "switch_statement":
				nodes = append(nodes, Node{Type: Nesting, Depth: depth})
				newDepth++
			case "for_statement", "for_in_statement", "for_of_statement", "while_statement", "do_statement":
				nodes = append(nodes, Node{Type: Loop, Depth: depth})
				newDepth++
			case "binary_expression", "assignment_expression":
				for i := 0; i < int(n.ChildCount()); i++ {
					child := n.Child(i)
					op := child.Content(content)
					if op == "&&" || op == "||" || op == "??" || op == "??=" || op == "&&=" || op == "||=" {
						nodes = append(nodes, Node{Type: Operator, Depth: depth})
						break
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
