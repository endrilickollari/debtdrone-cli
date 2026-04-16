package complexity

import (
	"context"
	"fmt"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"
)

type JavaAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewJavaAnalyzer(thresholds models.ComplexityThresholds) *JavaAnalyzer {
	return &JavaAnalyzer{
		thresholds: thresholds,
	}
}

func (a *JavaAnalyzer) Language() string {
	return "Java"
}

func (a *JavaAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	parser := sitter.NewParser()
	parser.SetLanguage(java.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	if tree == nil {
		return metrics, nil
	}
	defer tree.Close()

	root := tree.RootNode()
	functions, err := findJavaFunctions(root, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse java functions: %w", err)
	}

	for _, fn := range functions {
		nodes := mapJavaNodes(fn.node, content)
		cyclomatic, cognitive, nesting := CalculateComplexity(nodes)

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
			Severity:             severity,
			CodeSnippet:          &snippetStr,
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

type javaFunctionInfo struct {
	name       string
	line       int
	endLine    int
	body       string
	paramCount int
	node       *sitter.Node
}

func findJavaFunctions(root *sitter.Node, content []byte) ([]javaFunctionInfo, error) {

	queryStr := `
		(method_declaration
			name: (identifier) @name
			body: (block) @body
		) @method
		(constructor_declaration
			name: (identifier) @name
			body: (constructor_body) @body
		) @constructor
		(record_declaration
			name: (identifier) @record_name
			body: (class_body
				(compact_constructor_declaration
					body: (block) @body
				) @compact_constructor
			)
		)
		(lambda_expression
			body: (_) @body
		) @lambda
	`

	q, err := sitter.NewQuery([]byte(queryStr), java.GetLanguage())
	if err != nil {
		return nil, err
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, root)

	var functions []javaFunctionInfo

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
			case "method", "constructor", "lambda":
				fnNode = c.Node
			case "name", "record_name":
				fnName = c.Node.Content(content)
			case "compact_constructor":
				fnNode = c.Node
			case "body":
				fnBodyNode = c.Node
			}
		}

		if fnNode != nil && fnBodyNode != nil {
			if fnName == "" {
				fnName = "<lambda>"
			}
			paramCount = countJavaParameters(fnNode)

			functions = append(functions, javaFunctionInfo{
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

func countJavaParameters(fnNode *sitter.Node) int {
	count := 0
	for i := 0; i < int(fnNode.ChildCount()); i++ {
		child := fnNode.Child(i)
		if child.Type() == "formal_parameters" {
			for j := 0; j < int(child.ChildCount()); j++ {
				grandChild := child.Child(j)
				if grandChild.Type() == "formal_parameter" || grandChild.Type() == "spread_parameter" {
					count++
				}
			}
			break
		}
	}
	return count
}

func mapJavaNodes(node *sitter.Node, content []byte) []Node {
	var nodes []Node
	var visit func(n *sitter.Node, depth int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		nodeType := n.Type()

		switch nodeType {
		case "if_statement", "switch_label", "catch_clause", "ternary_expression":
			nodes = append(nodes, Node{Type: Branch, Depth: depth})
			newDepth++
		case "switch_expression", "switch_statement":
			nodes = append(nodes, Node{Type: Nesting, Depth: depth})
			newDepth++
		case "for_statement", "enhanced_for_statement", "while_statement", "do_statement":
			nodes = append(nodes, Node{Type: Loop, Depth: depth})
			newDepth++
		case "binary_expression":
			count := int(n.ChildCount())
			for i := 0; i < count; i++ {
				child := n.Child(i)
				childContent := child.Content(content)
				if childContent == "&&" || childContent == "||" {
					nodes = append(nodes, Node{Type: Operator, Depth: depth})
					break
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


