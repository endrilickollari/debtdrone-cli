package complexity

import (
	"context"
	"fmt"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/php"
)

type PHPAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewPHPAnalyzer(thresholds models.ComplexityThresholds) *PHPAnalyzer {
	return &PHPAnalyzer{
		thresholds: thresholds,
	}
}

func (a *PHPAnalyzer) Language() string {
	return "PHP"
}

func (a *PHPAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	parser := sitter.NewParser()
	parser.SetLanguage(php.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	if tree == nil {
		return metrics, nil
	}
	defer tree.Close()

	root := tree.RootNode()
	functions, err := findPHPFunctions(root, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse php functions: %w", err)
	}

	for _, fn := range functions {
		nodes := mapPHPNodes(fn.node, content)
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

type phpFunctionInfo struct {
	name       string
	line       int
	endLine    int
	body       string
	paramCount int
	node       *sitter.Node
}

func findPHPFunctions(root *sitter.Node, content []byte) ([]phpFunctionInfo, error) {

	queryStr := `
		(function_definition
			name: (name) @name
			body: (compound_statement) @body
		) @function

		(method_declaration
			name: (name) @name
			body: (compound_statement) @body
		) @function

		(anonymous_function_creation_expression
			body: (compound_statement) @body
		) @lambda

		(arrow_function
			body: (_) @body
		) @lambda
	`

	q, err := sitter.NewQuery([]byte(queryStr), php.GetLanguage())
	if err != nil {
		return nil, err
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, root)

	var functions []phpFunctionInfo

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

		if fnNode != nil && fnBodyNode != nil {
			if fnName == "" {
				fnName = "<anonymous>"
			}

			paramCount = countPHPParameters(fnNode)

			functions = append(functions, phpFunctionInfo{
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

func countPHPParameters(fnNode *sitter.Node) int {
	count := 0
	for i := 0; i < int(fnNode.ChildCount()); i++ {
		child := fnNode.Child(i)
		if child.Type() == "formal_parameters" {
			for j := 0; j < int(child.ChildCount()); j++ {
				grandChild := child.Child(j)
				switch grandChild.Type() {
				case "simple_parameter", "variadic_parameter", "property_promotion_parameter":
					count++
				}
			}
			break
		}
	}
	return count
}

func mapPHPNodes(node *sitter.Node, content []byte) []Node {
	var nodes []Node
	var visit func(n *sitter.Node, depth int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		nodeType := n.Type()

		switch nodeType {
		case "if_statement", "else_if_clause", "case_statement", "default_statement", "catch_clause", "conditional_expression":
			nodes = append(nodes, Node{Type: Branch, Depth: depth})
			newDepth++
		case "switch_statement":
			nodes = append(nodes, Node{Type: Nesting, Depth: depth})
			newDepth++
		case "while_statement", "do_statement", "for_statement", "foreach_statement":
			nodes = append(nodes, Node{Type: Loop, Depth: depth})
			newDepth++
		case "binary_expression":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				op := child.Content(content)
				switch op {
				case "&&", "||", "and", "or", "xor", "??": // ?? coalescing is binary too usually
					nodes = append(nodes, Node{Type: Operator, Depth: depth})
				}
			}
		case "null_coalescing_expression":
			nodes = append(nodes, Node{Type: Operator, Depth: depth})
		case "anonymous_function_creation_expression", "arrow_function":
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


