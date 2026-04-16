package complexity

import (
	"context"
	"fmt"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/csharp"
)

type CSharpAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewCSharpAnalyzer(thresholds models.ComplexityThresholds) *CSharpAnalyzer {
	return &CSharpAnalyzer{
		thresholds: thresholds,
	}
}

func (a *CSharpAnalyzer) Language() string {
	return "C#"
}

func (a *CSharpAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	ctx := context.Background()
	parser := sitter.NewParser()
	parser.SetLanguage(csharp.GetLanguage())

	tree, err := parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	functions := findCSharpFunctions(root, content)

	for _, fn := range functions {
		nodes := mapCSharpNodes(fn.Node, content)
		cyclomatic, cognitive, nesting := CalculateComplexity(nodes)
		loc := strings.Count(fn.BodyContent, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)

		cognitivePtr := cognitive
		snippetStr := truncateSnippet(fn.BodyContent, 10000)

		metric := models.ComplexityMetric{
			ID:                   uuid.New(),
			FilePath:             filePath,
			FunctionName:         fn.Name,
			StartLine:            int(fn.Node.StartPoint().Row) + 1,
			EndLine:              int(fn.Node.EndPoint().Row) + 1,
			CyclomaticComplexity: cyclomatic,
			CognitiveComplexity:  &cognitivePtr,
			NestingDepth:         nesting,
			ParameterCount:       fn.ParamCount,
			LinesOfCode:          loc,
			Severity:             severity,
			CodeSnippet:          &snippetStr,
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

type cSharpFunctionInfo struct {
	Name        string
	Node        *sitter.Node
	BodyContent string
	ParamCount  int
}

func findCSharpFunctions(root *sitter.Node, content []byte) []cSharpFunctionInfo {
	var functions []cSharpFunctionInfo
	queryStr := `
	(method_declaration name: (_) @name body: (_) @body)
	(local_function_statement name: (_) @name body: (_) @body)
	(constructor_declaration name: (_) @name body: (_) @body)
	(lambda_expression) @lambda
	(anonymous_method_expression) @lambda
	`
	q, err := sitter.NewQuery([]byte(queryStr), csharp.GetLanguage())
	if err != nil {
		fmt.Printf("Tree-Sitter query parse error: %v\n", err)
		return nil
	}
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	defer q.Close()

	qc.Exec(q, root)

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		var nameNode, bodyNode, parent *sitter.Node

		for _, c := range match.Captures {
			name := q.CaptureNameForId(c.Index)
			if name == "name" {
				nameNode = c.Node
			} else if name == "body" {
				bodyNode = c.Node
			} else if name == "lambda" {
				// lambda node itself
				parent = c.Node
			}
		}

		if bodyNode == nil && parent == nil {
			continue
		}
		if bodyNode == nil {
			bodyNode = parent
		}

		funcName := "<lambda>"
		if nameNode != nil {
			funcName = nameNode.Content(content)
			if nameNode.Parent() != nil {
				parent = nameNode.Parent()
			}
		}

		paramCount := 0
		if parent != nil {
			paramList := parent.ChildByFieldName("parameters")
			if paramList != nil {
				paramCount = countCSharpParameters(paramList)
			}
		}

		functions = append(functions, cSharpFunctionInfo{
			Name:        funcName,
			Node:        parent,
			BodyContent: bodyNode.Content(content),
			ParamCount:  paramCount,
		})
	}

	return functions
}

func countCSharpParameters(paramList *sitter.Node) int {
	count := 0
	for i := 0; i < int(paramList.NamedChildCount()); i++ {
		child := paramList.NamedChild(i)
		if child.Type() == "parameter" {
			count++
		}
	}
	return count
}

func mapCSharpNodes(node *sitter.Node, content []byte) []Node {
	var nodes []Node
	if node == nil {
		return nodes
	}

	var visit func(*sitter.Node, int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		t := n.Type()
		switch t {
		case "if_statement", "catch_clause", "conditional_expression", "case_switch_label":
			nodes = append(nodes, Node{Type: Branch, Depth: depth})
			newDepth++
		case "switch_statement":
			nodes = append(nodes, Node{Type: Nesting, Depth: depth})
			newDepth++
		case "for_statement", "foreach_statement", "while_statement", "do_statement":
			nodes = append(nodes, Node{Type: Loop, Depth: depth})
			newDepth++
		case "binary_expression":
			op := n.ChildByFieldName("operator")
			if op != nil {
				opStr := op.Content(content)
				if opStr == "&&" || opStr == "||" || opStr == "??" {
					nodes = append(nodes, Node{Type: Operator, Depth: depth})
				}
			}
		}

		for i := 0; i < int(n.NamedChildCount()); i++ {
			visit(n.NamedChild(i), newDepth)
		}
	}

	visit(node, 0)
	return nodes
}
