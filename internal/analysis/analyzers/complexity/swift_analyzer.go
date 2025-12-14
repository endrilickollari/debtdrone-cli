package complexity

import (
	"context"
	"fmt"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/swift"
)

type SwiftAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewSwiftAnalyzer(thresholds models.ComplexityThresholds) *SwiftAnalyzer {
	return &SwiftAnalyzer{
		thresholds: thresholds,
	}
}

func (a *SwiftAnalyzer) Language() string {
	return "Swift"
}
func (a *SwiftAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	parser := sitter.NewParser()
	parser.SetLanguage(swift.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	if tree == nil {
		return metrics, nil
	}
	defer tree.Close()

	root := tree.RootNode()

	functions, err := findSwiftFunctions(root, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse swift functions: %w", err)
	}

	for _, fn := range functions {
		cyclomatic := calculateSwiftCyclomatic(fn.node, content)

		cognitive := calculateSwiftCognitive(fn.node, content)
		nesting := calculateSwiftNesting(fn.node)
		loc := strings.Count(fn.body, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		debtMinutes := estimateSwiftTechnicalDebt(cyclomatic, cognitive, loc)
		suggestions := generateSwiftRefactoringSuggestions(cyclomatic, cognitive, nesting, fn.paramCount, loc)

		cognitivePtr := cognitive
		snippetStr := truncateSnippet(fn.body, 300)

		metric := models.ComplexityMetric{
			ID:                     uuid.New(),
			FilePath:               filePath,
			FunctionName:           fn.name,
			StartLine:              fn.line,
			EndLine:                fn.endLine,
			CyclomaticComplexity:   cyclomatic,
			CognitiveComplexity:    &cognitivePtr,
			NestingDepth:           nesting,
			ParameterCount:         fn.paramCount,
			LinesOfCode:            loc,
			Severity:               severity,
			TechnicalDebtMinutes:   debtMinutes,
			RefactoringSuggestions: suggestions,
			CodeSnippet:            &snippetStr,
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

type swiftFunctionInfo struct {
	name       string
	line       int
	endLine    int
	body       string
	paramCount int
	node       *sitter.Node
}

func findSwiftFunctions(root *sitter.Node, content []byte) ([]swiftFunctionInfo, error) {

	queryStr := `
		(function_declaration
			name: (simple_identifier) @name
			body: (_) @body
		) @function

		(init_declaration
			body: (_) @body
		) @init

		(deinit_declaration
			body: (_) @body
		) @deinit
	`

	q, err := sitter.NewQuery([]byte(queryStr), swift.GetLanguage())
	if err != nil {
		return nil, err
	}
	defer q.Close()

	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(q, root)

	var functions []swiftFunctionInfo

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		var fnName string
		var fnBodyNode *sitter.Node
		var fnNode *sitter.Node

		for _, c := range m.Captures {
			captureName := q.CaptureNameForId(c.Index)
			switch captureName {
			case "function":
				fnNode = c.Node
			case "init":
				fnNode = c.Node
				fnName = "init"
			case "deinit":
				fnNode = c.Node
				fnName = "deinit"
			case "name":
				fnName = c.Node.Content(content)
			case "body":
				fnBodyNode = c.Node
			}
		}

		if fnNode != nil && fnBodyNode != nil {
			if fnName == "" {
				if fnNode.Type() == "init_declaration" {
					fnName = "init"
				} else if fnNode.Type() == "deinit_declaration" {
					fnName = "deinit"
				}
			}

			paramCount := countSwiftParametersManual(fnNode)

			functions = append(functions, swiftFunctionInfo{
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

func countSwiftParametersManual(fnNode *sitter.Node) int {
	count := 0

	for i := 0; i < int(fnNode.ChildCount()); i++ {
		child := fnNode.Child(i)

		if child.Type() == "parameter" {
			count++
		} else if strings.Contains(child.Type(), "parameter_clause") {
			count += countParamsInClause(child)
		} else if strings.Contains(child.Type(), "signature") {
			for j := 0; j < int(child.ChildCount()); j++ {
				subChild := child.Child(j)
				if strings.Contains(subChild.Type(), "parameter_clause") {
					count += countParamsInClause(subChild)
				} else if subChild.Type() == "parameter" {
					count++
				}
			}
		}
	}
	return count
}

func countParamsInClause(node *sitter.Node) int {
	c := 0
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "parameter" {
			c++
		}
	}
	return c
}

func calculateSwiftCyclomatic(node *sitter.Node, content []byte) int {
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
			case "guard_statement":
				complexity++
			case "for_statement":
				complexity++
			case "while_statement", "repeat_while_statement":
				complexity++
			case "switch_entry":
				complexity++
			case "catch_clause":
				complexity++
			case "conjunction_expression":
				complexity++
			case "disjunction_expression":
				complexity++
			case "ternary_expression":
				complexity++
			case "nil_coalescing_expression":
				complexity++
			case "optional_chaining_expression":
				complexity++
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

func calculateSwiftCognitive(node *sitter.Node, content []byte) int {
	complexity := 0

	WalkTree(node, func(n *sitter.Node) {
		nodeType := n.Type()
		switch nodeType {
		case "if_statement", "guard_statement", "switch_statement", "while_statement", "repeat_while_statement", "for_statement", "do_statement", "catch_clause":
			complexity += 2
		case "conjunction_expression", "disjunction_expression":
			complexity += 1
		}
	})

	return complexity
}

func calculateSwiftNesting(node *sitter.Node) int {
	maxDepth := 0
	var visit func(*sitter.Node, int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		t := n.Type()
		switch t {
		case "if_statement", "for_statement", "while_statement", "do_statement", "catch_clause", "switch_statement":
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

func generateSwiftRefactoringSuggestions(cyclomatic, cognitive, nesting, paramCount, loc int) []models.RefactoringSuggestion {
	var suggestions []models.RefactoringSuggestion

	if cyclomatic > 15 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Extract Methods",
			Description: "Break down this function into smaller, focused methods. Consider using extension methods or protocols",
		})
	}

	if nesting > 3 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Reduce Nesting Depth",
			Description: "Use guard statements, optional chaining (?.), or early returns to reduce nesting",
		})
	}

	if paramCount > 4 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Use Struct or Builder Pattern",
			Description: "Too many parameters. Consider using a struct with default values or a builder pattern",
		})
	}

	if loc > 50 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Function Too Long",
			Description: "Split this function into smaller methods. Consider extracting logic into extensions or separate types",
		})
	}

	if cognitive > 20 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Simplify Logic",
			Description: "Use Swift's Result type, optional chaining, guard statements, or functional methods (map, flatMap) to simplify logic",
		})
	}

	return suggestions
}

func estimateSwiftTechnicalDebt(cyclomatic, cognitive, loc int) int {
	baseMinutes := 5

	complexityMinutes := (cyclomatic - 10) * 2
	if complexityMinutes < 0 {
		complexityMinutes = 0
	}

	cognitiveMinutes := (cognitive - 15) * 1
	if cognitiveMinutes < 0 {
		cognitiveMinutes = 0
	}

	locMinutes := (loc - 30) / 5
	if locMinutes < 0 {
		locMinutes = 0
	}

	return baseMinutes + complexityMinutes + cognitiveMinutes + locMinutes
}
