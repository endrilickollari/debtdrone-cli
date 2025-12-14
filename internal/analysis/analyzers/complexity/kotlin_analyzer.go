package complexity

import (
	"context"
	"fmt"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/kotlin"
)

type KotlinAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewKotlinAnalyzer(thresholds models.ComplexityThresholds) *KotlinAnalyzer {
	return &KotlinAnalyzer{
		thresholds: thresholds,
	}
}

func (a *KotlinAnalyzer) Language() string {
	return "Kotlin"
}
func (a *KotlinAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	parser := sitter.NewParser()
	parser.SetLanguage(kotlin.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	if tree == nil {
		return metrics, nil
	}
	defer tree.Close()

	root := tree.RootNode()

	functions, err := findKotlinFunctions(root, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse kotlin functions: %w", err)
	}

	for _, fn := range functions {
		cyclomatic := calculateKotlinCyclomatic(fn.node, content)

		cognitive := calculateKotlinCognitive(fn.node, content)
		nesting := calculateKotlinNesting(fn.node)
		loc := strings.Count(fn.body, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		debtMinutes := estimateKotlinTechnicalDebt(cyclomatic, cognitive, loc)
		suggestions := generateKotlinRefactoringSuggestions(cyclomatic, cognitive, nesting, fn.paramCount, loc)

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

type kotlinFunctionInfo struct {
	name       string
	line       int
	endLine    int
	body       string
	paramCount int
	node       *sitter.Node
}

func findKotlinFunctions(root *sitter.Node, content []byte) ([]kotlinFunctionInfo, error) {

	queryStr := `
		(function_declaration
			(simple_identifier) @name
			(function_body) @body
		) @function
	`

	q, err := sitter.NewQuery([]byte(queryStr), kotlin.GetLanguage())
	if err != nil {
		return nil, err
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, root)

	var functions []kotlinFunctionInfo

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
			paramCount = countKotlinParameters(fnNode)

			functions = append(functions, kotlinFunctionInfo{
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

func countKotlinParameters(fnNode *sitter.Node) int {
	count := 0
	for i := 0; i < int(fnNode.ChildCount()); i++ {
		child := fnNode.Child(i)
		if child.Type() == "function_value_parameters" {
			for j := 0; j < int(child.ChildCount()); j++ {
				grandChild := child.Child(j)
				if grandChild.Type() == "parameter" || grandChild.Type() == "class_parameter" {
					count++
				}
			}
			break
		}
	}
	return count
}

func calculateKotlinCyclomatic(node *sitter.Node, content []byte) int {
	complexity := 1
	cursor := sitter.NewTreeCursor(node)
	defer cursor.Close()

	for {
		n := cursor.CurrentNode()
		nodeType := n.Type()

		switch nodeType {
		case "if_expression", "for_statement", "while_statement", "do_while_statement", "catch_block", "when_expression":
			complexity++
		case "when_entry":
			complexity++
		case "binary_expression":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				text := child.Content(content)
				if text == "&&" || text == "||" {
					complexity++
				}
			}
		case "call_expression":
			text := n.Content(content)
			if strings.Contains(text, ".let") || strings.Contains(text, ".run") || strings.Contains(text, ".apply") || strings.Contains(text, ".also") {
				complexity++
			}
		case "navigation_expression", "safe_navigation_expression":
			complexity++
			if nodeType == "safe_navigation_expression" {
				complexity++
			} else if nodeType == "navigation_expression" {
				for i := 0; i < int(n.ChildCount()); i++ {
					if n.Child(i).Type() == "safe_navigation_operator" {
						complexity++
						break
					}
				}
			}
		case "elvis_expression":
			complexity++
		case "lambda_literal":
			complexity++
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

func calculateKotlinCognitive(node *sitter.Node, content []byte) int {
	complexity := 0

	WalkTree(node, func(n *sitter.Node) {
		nodeType := n.Type()
		switch nodeType {
		case "if_expression", "when_expression", "while_statement", "for_statement", "do_while_statement", "catch_block":
			complexity += 2
		case "binary_expression":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				text := child.Content(content)
				if text == "&&" || text == "||" {
					complexity += 1
				}
			}
		case "call_expression":
			text := n.Content(content)
			if strings.Contains(text, ".let") || strings.Contains(text, ".run") || strings.Contains(text, ".apply") || strings.Contains(text, ".also") {
				complexity += 1
			}
		}
	})

	return complexity
}

func calculateKotlinNesting(node *sitter.Node) int {
	maxDepth := 0
	var visit func(*sitter.Node, int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		t := n.Type()
		switch t {
		case "if_expression", "for_statement", "while_statement", "do_while_statement", "when_expression", "catch_block":
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

func estimateKotlinTechnicalDebt(cyclomatic, cognitive, loc int) int {
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

func generateKotlinRefactoringSuggestions(cyclomatic, cognitive, nesting, paramCount, loc int) []models.RefactoringSuggestion {
	var suggestions []models.RefactoringSuggestion

	if cyclomatic > 15 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Extract Functions",
			Description: "Break down this function into smaller, focused functions. Consider using extension functions or sealed classes",
		})
	}

	if nesting > 3 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Reduce Nesting Depth",
			Description: "Use Kotlin's safe call operators (?.), let, also, or early returns to reduce nesting",
		})
	}

	if paramCount > 4 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Use Data Class or Builder Pattern",
			Description: "Too many parameters. Consider using a data class with named parameters or default values",
		})
	}

	if loc > 50 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Function Too Long",
			Description: "Split this function into smaller functions. Consider using extension functions or separating concerns",
		})
	}

	if cognitive > 20 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Simplify Logic",
			Description: "Use Kotlin's scope functions (let, run, apply), when expressions, or sealed classes to simplify logic",
		})
	}

	return suggestions
}
