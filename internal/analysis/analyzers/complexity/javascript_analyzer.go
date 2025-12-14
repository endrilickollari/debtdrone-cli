package complexity

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

type JavaScriptAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewJavaScriptAnalyzer(thresholds models.ComplexityThresholds) *JavaScriptAnalyzer {
	return &JavaScriptAnalyzer{
		thresholds: thresholds,
	}
}

func (a *JavaScriptAnalyzer) Language() string {
	return "JavaScript"
}

func (a *JavaScriptAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	parser := sitter.NewParser()
	parser.SetLanguage(javascript.GetLanguage())

	tree := parser.Parse(nil, content)
	if tree == nil {
		return metrics, nil
	}
	defer tree.Close()

	root := tree.RootNode()

	functions := findJavaScriptFunctions(root, content)

	for _, fn := range functions {
		cyclomatic := calculateJSCyclomatic(fn.Node, content)
		cognitive := calculateJSCognitive(fn.Node, content)
		nesting := calculateJSNesting(fn.Node)

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
			Severity:             severity,
			CodeSnippet:          &snippetStr,
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

func findJavaScriptFunctions(root *sitter.Node, content []byte) []functionInfo {
	var functions []functionInfo

	traverseForJSFunctions(root, content, &functions)

	return functions
}

func traverseForJSFunctions(node *sitter.Node, content []byte, functions *[]functionInfo) {
	if node == nil {
		return
	}

	nodeType := node.Type()

	switch nodeType {
	case "function_declaration":
		fn := extractJSFunctionDeclaration(node, content)
		if fn.name != "" {
			*functions = append(*functions, fn)
		}
	case "function":
		fn := extractJSFunction(node, content)
		if fn.name != "" {
			*functions = append(*functions, fn)
		}
	case "arrow_function":
		fn := extractJSArrowFunction(node, content)
		if fn.name != "" {
			*functions = append(*functions, fn)
		}
	case "method_definition":
		fn := extractJSMethodDefinition(node, content)
		if fn.name != "" {
			*functions = append(*functions, fn)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		traverseForJSFunctions(child, content, functions)
	}
}

func extractJSFunctionDeclaration(node *sitter.Node, content []byte) functionInfo {
	var fn functionInfo

	fn.line = int(node.StartPoint().Row) + 1
	fn.endLine = int(node.EndPoint().Row) + 1
	fn.body = node.Content(content)
	fn.Node = node

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childType := child.Type()

		if childType == "identifier" && fn.name == "" {
			fn.name = child.Content(content)
		} else if childType == "formal_parameters" {
			fn.paramCount = countJSParameters(child)
		}
	}

	return fn
}

func extractJSFunction(node *sitter.Node, content []byte) functionInfo {
	var fn functionInfo

	fn.line = int(node.StartPoint().Row) + 1
	fn.endLine = int(node.EndPoint().Row) + 1
	fn.body = node.Content(content)
	fn.Node = node

	parent := node.Parent()
	if parent != nil && parent.Type() == "variable_declarator" {
		for i := 0; i < int(parent.ChildCount()); i++ {
			child := parent.Child(i)
			if child.Type() == "identifier" {
				fn.name = child.Content(content)
				break
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "formal_parameters" {
			fn.paramCount = countJSParameters(child)
			break
		}
	}

	return fn
}

func extractJSArrowFunction(node *sitter.Node, content []byte) functionInfo {
	var fn functionInfo

	fn.line = int(node.StartPoint().Row) + 1
	fn.endLine = int(node.EndPoint().Row) + 1
	fn.body = node.Content(content)
	fn.Node = node

	parent := node.Parent()
	if parent != nil {
		if parent.Type() == "variable_declarator" {
			for i := 0; i < int(parent.ChildCount()); i++ {
				child := parent.Child(i)
				if child.Type() == "identifier" {
					fn.name = child.Content(content)
					break
				}
			}
		} else if parent.Type() == "pair" {
			for i := 0; i < int(parent.ChildCount()); i++ {
				child := parent.Child(i)
				if child.Type() == "property_identifier" {
					fn.name = child.Content(content)
					break
				}
			}
		} else if parent.Type() == "assignment_expression" {
			for i := 0; i < int(parent.ChildCount()); i++ {
				child := parent.Child(i)
				if child.Type() == "member_expression" {
					for j := 0; j < int(child.ChildCount()); j++ {
						subChild := child.Child(j)
						if subChild.Type() == "property_identifier" {
							fn.name = subChild.Content(content)
							break
						}
					}
					break
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childType := child.Type()

		switch childType {
		case "formal_parameters":
			fn.paramCount = countJSParameters(child)
		case "identifier":
			fn.paramCount = 1
		}
	}

	return fn
}

func extractJSMethodDefinition(node *sitter.Node, content []byte) functionInfo {
	var fn functionInfo

	fn.line = int(node.StartPoint().Row) + 1
	fn.endLine = int(node.EndPoint().Row) + 1
	fn.body = node.Content(content)
	fn.Node = node

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childType := child.Type()

		if childType == "property_identifier" && fn.name == "" {
			fn.name = child.Content(content)
		} else if childType == "formal_parameters" {
			fn.paramCount = countJSParameters(child)
		}
	}

	return fn
}

func calculateJSCyclomatic(node *sitter.Node, content []byte) int {
	complexity := 1

	WalkTree(node, func(n *sitter.Node) {
		nodeType := n.Type()
		switch nodeType {
		case "if_statement", "for_statement", "for_in_statement", "for_of_statement",
			"while_statement", "do_statement", "switch_case", "catch_clause",
			"ternary_expression":
			complexity++
		case "binary_expression":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				op := child.Content(content)
				if op == "&&" || op == "||" || op == "??" {
					complexity++
					break
				}
			}
		}
	})

	return complexity
}

func countJSParameters(paramsNode *sitter.Node) int {
	count := 0

	for i := 0; i < int(paramsNode.ChildCount()); i++ {
		child := paramsNode.Child(i)
		nodeType := child.Type()

		if nodeType == "identifier" ||
			nodeType == "assignment_pattern" ||
			nodeType == "rest_parameter" ||
			nodeType == "object_pattern" ||
			nodeType == "array_pattern" {
			count++
		}
	}

	return count
}

func calculateJSCognitive(node *sitter.Node, content []byte) int {
	complexity := 0

	WalkTree(node, func(n *sitter.Node) {
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
	})

	return complexity
}

func calculateJSNesting(node *sitter.Node) int {
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
