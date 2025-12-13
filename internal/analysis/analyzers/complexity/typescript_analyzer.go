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

	functions, err := findTypeScriptFunctions(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse typescript functions: %w", err)
	}

	for _, fn := range functions {
		cyclomatic := calculateTypeScriptCyclomatic(fn.node, content)
		cognitive := calculateTypeScriptCognitive(fn.node, content)
		nesting := calculateTypeScriptNesting(fn.node)
		loc := strings.Count(fn.body, "\n") + 1
		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		cognitivePtr := cognitive
		snippetStr := truncateSnippet(fn.body, 300)

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

func findTypeScriptFunctions(content []byte) ([]tsFunctionInfo, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}

	queryStr := `
		(function_declaration
			name: (identifier) @name
			parameters: (formal_parameters) @params
			body: (_) @body
		) @function

		(method_definition
			name: (property_identifier) @name
			parameters: (formal_parameters) @params
			body: (_) @body
		) @method

		(variable_declarator
			name: (identifier) @name
			value: (arrow_function
				parameters: (formal_parameters) @params
				body: (_) @body
			)
		) @arrow
	`

	q, err := sitter.NewQuery([]byte(queryStr), typescript.GetLanguage())
	if err != nil {
		return nil, err
	}
	defer q.Close()

	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(q, tree.RootNode())

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

func calculateTypeScriptCyclomatic(node *sitter.Node, content []byte) int {
	complexity := 1
	cursor := sitter.NewTreeCursor(node)
	defer cursor.Close()

	for {
		n := cursor.CurrentNode()

		if n.IsNamed() {
			nodeType := n.Type()
			switch nodeType {
			case "if_statement":
				complexity++
			case "for_statement", "for_in_statement", "for_of_statement":
				complexity++
			case "while_statement", "do_statement":
				complexity++
			case "switch_case":
				complexity++
			case "catch_clause":
				complexity++
			case "binary_expression":
				count := int(n.ChildCount())
				for i := 0; i < count; i++ {
					child := n.Child(i)
					op := child.Content(content)
					if op == "&&" || op == "||" || op == "??" {
						complexity++
					}
				}
			case "ternary_expression":
				complexity++
			case "assignment_expression":
				count := int(n.ChildCount())
				for i := 0; i < count; i++ {
					child := n.Child(i)
					op := child.Content(content)
					if op == "??=" || op == "&&=" || op == "||=" {
						complexity++
					}
				}
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

func calculateTypeScriptCognitive(node *sitter.Node, content []byte) int {
	complexity := 0

	WalkTree(node, func(n *sitter.Node) {
		if n.IsNamed() {
			nodeType := n.Type()
			switch nodeType {
			case "if_statement", "for_statement", "for_in_statement", "for_of_statement",
				"while_statement", "do_statement", "switch_case", "catch_clause":
				complexity += 2
			case "ternary_expression":
				complexity += 1
			case "binary_expression":

				for i := 0; i < int(n.ChildCount()); i++ {
					child := n.Child(i)
					op := child.Content(content)
					if op == "&&" || op == "||" || op == "??" {
						complexity += 1
						break
					}
				}
			}
		}
	})

	return complexity
}

func calculateTypeScriptNesting(node *sitter.Node) int {
	maxDepth := 0
	var visit func(*sitter.Node, int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		t := n.Type()
		switch t {
		case "if_statement", "for_statement", "for_in_statement", "for_of_statement", "while_statement", "do_statement", "switch_statement", "catch_clause":
			newDepth++
			if newDepth > maxDepth {
				maxDepth = newDepth
			}
		}

		for i := 0; i < int(n.ChildCount()); i++ {
			visit(n.Child(i), newDepth)
		}
	}

	visit(node, 0)
	return maxDepth
}
