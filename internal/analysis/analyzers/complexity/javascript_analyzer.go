package complexity

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

// JavaScriptAnalyzer analyzes JavaScript/JSX code for complexity metrics
type JavaScriptAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewJavaScriptAnalyzer creates a new JavaScript complexity analyzer
func NewJavaScriptAnalyzer(thresholds models.ComplexityThresholds) *JavaScriptAnalyzer {
	return &JavaScriptAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *JavaScriptAnalyzer) Language() string {
	return "JavaScript"
}

// AnalyzeFile analyzes a JavaScript file and returns complexity metrics
func (a *JavaScriptAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	functions := findJavaScriptFunctions(content)

	for _, fn := range functions {
		cyclomatic := calculatePatternBasedCyclomatic(fn.body)
		cognitive := calculatePatternBasedCognitive(fn.body)
		nesting := calculatePatternBasedNesting(fn.body)

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

// findJavaScriptFunctions finds function declarations in JavaScript/TypeScript code using Tree-sitter
func findJavaScriptFunctions(content []byte) []functionInfo {
	var functions []functionInfo

	parser := sitter.NewParser()
	parser.SetLanguage(javascript.GetLanguage())

	tree := parser.Parse(nil, content)
	if tree == nil {
		return functions
	}
	defer tree.Close()

	root := tree.RootNode()

	traverseForJSFunctions(root, content, &functions)

	return functions
}

// traverseForJSFunctions recursively traverses the AST to find function definitions
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

// extractJSFunctionDeclaration extracts function information from a function_declaration node
func extractJSFunctionDeclaration(node *sitter.Node, content []byte) functionInfo {
	var fn functionInfo

	fn.line = int(node.StartPoint().Row) + 1
	fn.endLine = int(node.EndPoint().Row) + 1
	fn.body = node.Content(content)

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

// extractJSFunction extracts function information from an anonymous function node
func extractJSFunction(node *sitter.Node, content []byte) functionInfo {
	var fn functionInfo

	fn.line = int(node.StartPoint().Row) + 1
	fn.endLine = int(node.EndPoint().Row) + 1
	fn.body = node.Content(content)

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

// extractJSArrowFunction extracts function information from an arrow_function node
func extractJSArrowFunction(node *sitter.Node, content []byte) functionInfo {
	var fn functionInfo

	fn.line = int(node.StartPoint().Row) + 1
	fn.endLine = int(node.EndPoint().Row) + 1
	fn.body = node.Content(content)

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

// extractJSMethodDefinition extracts function information from a method_definition node
func extractJSMethodDefinition(node *sitter.Node, content []byte) functionInfo {
	var fn functionInfo

	fn.line = int(node.StartPoint().Row) + 1
	fn.endLine = int(node.EndPoint().Row) + 1
	fn.body = node.Content(content)

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

// countJSParameters counts the number of parameters in a formal_parameters node
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
