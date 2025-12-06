package complexity

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

// PythonAnalyzer analyzes Python code for complexity metrics
type PythonAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewPythonAnalyzer creates a new Python complexity analyzer
func NewPythonAnalyzer(thresholds models.ComplexityThresholds) *PythonAnalyzer {
	return &PythonAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *PythonAnalyzer) Language() string {
	return "Python"
}

// AnalyzeFile analyzes a Python file and returns complexity metrics
func (a *PythonAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	functions := findPythonFunctions(content)

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

// findPythonFunctions finds function definitions in Python code using Tree-sitter
func findPythonFunctions(content []byte) []functionInfo {
	var functions []functionInfo

	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())

	tree, _ := parser.ParseCtx(context.Background(), nil, content)
	if tree == nil {
		return functions
	}
	defer tree.Close()

	root := tree.RootNode()

	traverseForFunctions(root, content, &functions)

	return functions
}

// traverseForFunctions recursively traverses the AST to find function definitions
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
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		traverseForFunctions(child, content, functions)
	}
}

// extractPythonFunction extracts function information from a function_definition node
func extractPythonFunction(node *sitter.Node, content []byte) functionInfo {
	var fn functionInfo

	fn.line = int(node.StartPoint().Row) + 1
	fn.endLine = int(node.EndPoint().Row) + 1

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

// countPythonParameters counts the number of parameters, excluding 'self' and 'cls'
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
