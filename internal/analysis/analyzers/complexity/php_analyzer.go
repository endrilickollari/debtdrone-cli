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
		cyclomatic := calculatePHPCyclomatic(fn.node, content)

		cognitive := calculatePHPCognitive(fn.node, content)
		nesting := calculatePHPNesting(fn.node)
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
			case "function":
				fnNode = c.Node
			case "name":
				fnName = c.Node.Content(content)
			case "body":
				fnBodyNode = c.Node
			}
		}

		if fnNode != nil && fnName != "" && fnBodyNode != nil {
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

func calculatePHPCyclomatic(node *sitter.Node, content []byte) int {
	complexity := 1
	cursor := sitter.NewTreeCursor(node)
	defer cursor.Close()

	for {
		n := cursor.CurrentNode()
		nodeType := n.Type()

		switch nodeType {
		case "if_statement", "while_statement", "do_statement", "for_statement", "foreach_statement", "switch_statement":
			complexity++
		case "case_statement", "default_statement":
			complexity++
		case "catch_clause":
			complexity++
		case "conditional_expression":
			complexity++
		case "null_coalescing_expression":
			complexity++
		case "binary_expression":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				op := child.Content(content)
				switch op {
				case "&&", "||", "and", "or", "xor":
					complexity++
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

func calculatePHPCognitive(node *sitter.Node, content []byte) int {
	complexity := 0

	WalkTree(node, func(n *sitter.Node) {
		nodeType := n.Type()
		switch nodeType {
		case "if_statement", "for_statement", "foreach_statement", "while_statement", "do_statement", "case_statement", "catch_clause", "else_if_clause":
			complexity += 2
		case "binary_expression":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				op := child.Content(content)
				switch op {
				case "&&", "||", "and", "or", "xor":
					complexity += 1
				}
			}
		}
	})

	return complexity
}

func calculatePHPNesting(node *sitter.Node) int {
	maxDepth := 0
	var visit func(*sitter.Node, int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		t := n.Type()
		switch t {
		case "if_statement", "for_statement", "foreach_statement", "while_statement", "do_statement", "switch_statement", "catch_clause":
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
