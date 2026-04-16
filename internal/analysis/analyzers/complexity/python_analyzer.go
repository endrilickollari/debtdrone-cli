package complexity

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

type PythonAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewPythonAnalyzer(thresholds models.ComplexityThresholds) *PythonAnalyzer {
	return &PythonAnalyzer{
		thresholds: thresholds,
	}
}

func (a *PythonAnalyzer) Language() string {
	return "Python"
}
func (a *PythonAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())

	tree, _ := parser.ParseCtx(context.Background(), nil, content)
	if tree == nil {
		return metrics, nil
	}
	defer tree.Close()

	root := tree.RootNode()

	functions := findPythonFunctions(root, content)

	for _, fn := range functions {
		nodes := mapPythonNodes(fn.Node)
		cyclomatic, cognitive, nesting := CalculateComplexity(nodes)

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)

		cognitivePtr := cognitive
		// Use full function code for AI fixes - extract up to 10000 chars
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

func findPythonFunctions(root *sitter.Node, content []byte) []functionInfo {
	var functions []functionInfo

	traverseForFunctions(root, content, &functions)

	return functions
}

func traverseForFunctions(node *sitter.Node, content []byte, functions *[]functionInfo) {
	if node == nil {
		return
	}

	nodeType := node.Type()

	if nodeType == "function_definition" {
		fn := extractPythonFunction(node, content)
		if fn.name != "" {
			*functions = append(*functions, fn)
		}
	} else if nodeType == "lambda" {
		fn := extractPythonLambda(node, content)
		*functions = append(*functions, fn)
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		traverseForFunctions(child, content, functions)
	}
}

func extractPythonFunction(node *sitter.Node, content []byte) functionInfo {
	var fn functionInfo

	fn.line = int(node.StartPoint().Row) + 1
	fn.endLine = int(node.EndPoint().Row) + 1
	fn.Node = node

	fn.body = node.Content(content)

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			fn.name = child.Content(content)
			break
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "parameters" {
			fn.paramCount = countPythonParameters(child, content)
			break
		}
	}

	return fn
}

func extractPythonLambda(node *sitter.Node, content []byte) functionInfo {
	var fn functionInfo

	fn.line = int(node.StartPoint().Row) + 1
	fn.endLine = int(node.EndPoint().Row) + 1
	fn.Node = node
	fn.name = "<lambda>"
	fn.body = node.Content(content)

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "lambda_parameters" {
			fn.paramCount = countPythonParameters(child, content)
			break
		}
	}

	return fn
}

func mapPythonNodes(node *sitter.Node) []Node {
	var nodes []Node
	var visit func(n *sitter.Node, depth int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		t := n.Type()

		switch t {
		case "if_statement", "elif_clause", "except_clause", "case_clause", "conditional_expression", "match_statement":
			nodes = append(nodes, Node{Type: Branch, Depth: depth})
			newDepth++
		case "for_statement", "while_statement":
			nodes = append(nodes, Node{Type: Loop, Depth: depth})
			newDepth++
		case "try_statement":
			// try block adds nesting but not a branch by itself usually, but we can treat as nesting.
			nodes = append(nodes, Node{Type: Nesting, Depth: depth})
			newDepth++
		case "boolean_operator":
			nodes = append(nodes, Node{Type: Operator, Depth: depth})
		}

		for i := 0; i < int(n.ChildCount()); i++ {
			visit(n.Child(i), newDepth)
		}
	}

	visit(node, 0)
	return nodes
}

func countPythonParameters(paramsNode *sitter.Node, content []byte) int {
	count := 0

	for i := 0; i < int(paramsNode.ChildCount()); i++ {
		child := paramsNode.Child(i)
		nodeType := child.Type()

		if nodeType == "identifier" {
			paramName := child.Content(content)
			if paramName != "self" && paramName != "cls" {
				count++
			}
		} else if nodeType == "typed_parameter" || nodeType == "default_parameter" ||
			nodeType == "typed_default_parameter" || nodeType == "list_splat_pattern" ||
			nodeType == "dictionary_splat_pattern" {
			paramText := child.Content(content)
			for j := 0; j < int(child.ChildCount()); j++ {
				subChild := child.Child(j)
				if subChild.Type() == "identifier" {
					paramName := subChild.Content(content)
					if paramName != "self" && paramName != "cls" {
						count++
					}
					break
				}
			}
			if child.ChildCount() > 0 && !strings.Contains(paramText, "self") && !strings.Contains(paramText, "cls") {
				hasIdentifier := false
				for j := 0; j < int(child.ChildCount()); j++ {
					if child.Child(j).Type() == "identifier" {
						hasIdentifier = true
						break
					}
				}
				if !hasIdentifier {
					count++
				}
			}
		}
	}

	return count
}
