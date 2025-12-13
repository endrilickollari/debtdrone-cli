package complexity

import (
	"context"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/cpp"
)

type CCppAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewCCppAnalyzer(thresholds models.ComplexityThresholds) *CCppAnalyzer {
	return &CCppAnalyzer{
		thresholds: thresholds,
	}
}

func (a *CCppAnalyzer) Language() string {
	return "C/C++"
}

func (a *CCppAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	ctx := context.Background()
	parser := sitter.NewParser()
	parser.SetLanguage(cpp.GetLanguage())

	tree, err := parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, err
	}

	root := tree.RootNode()
	functions := findCCppFunctions(root, content)

	for _, fn := range functions {
		cyclomatic := calculateCCppCyclomatic(fn.Node, content)

		cognitive := calculateCCppCognitive(fn.Node, content)
		nesting := calculateCCppNesting(fn.Node)
		loc := strings.Count(fn.BodyContent, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		debtMinutes := estimateCCppTechnicalDebt(cyclomatic, cognitive, loc)
		suggestions := generateCCppRefactoringSuggestions(cyclomatic, cognitive, nesting, fn.ParamCount, loc)

		cognitivePtr := cognitive
		snippetStr := truncateSnippet(fn.BodyContent, 300)

		metric := models.ComplexityMetric{
			ID:                     uuid.New(),
			FilePath:               filePath,
			FunctionName:           fn.Name,
			StartLine:              int(fn.Node.StartPoint().Row) + 1,
			EndLine:                int(fn.Node.EndPoint().Row) + 1,
			CyclomaticComplexity:   cyclomatic,
			CognitiveComplexity:    &cognitivePtr,
			NestingDepth:           nesting,
			ParameterCount:         fn.ParamCount,
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

type cCppFunctionInfo struct {
	Name        string
	Node        *sitter.Node
	BodyContent string
	ParamCount  int
}

func findCCppFunctions(root *sitter.Node, content []byte) []cCppFunctionInfo {
	var functions []cCppFunctionInfo

	queryStr := `(function_definition declarator: (_) @declarator body: (_) @body)`
	q, _ := sitter.NewQuery([]byte(queryStr), cpp.GetLanguage())
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	defer q.Close()

	qc.Exec(q, root)

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		var declaratorNode, bodyNode *sitter.Node

		for _, c := range match.Captures {
			name := q.CaptureNameForId(c.Index)
			if name == "declarator" {
				declaratorNode = c.Node
			} else if name == "body" {
				bodyNode = c.Node
			}
		}

		if declaratorNode == nil || bodyNode == nil {
			continue
		}

		functions = append(functions, cCppFunctionInfo{
			Name:        extractFunctionName(declaratorNode, content),
			Node:        bodyNode,
			BodyContent: bodyNode.Content(content),
			ParamCount:  countCCppParameters(declaratorNode),
		})
	}

	return functions
}

func extractFunctionName(declarator *sitter.Node, content []byte) string {
	curr := declarator
	for {
		t := curr.Type()
		if t == "identifier" || t == "field_identifier" || t == "destructor_name" || t == "operator_name" {
			return curr.Content(content)
		}
		if t == "qualified_identifier" {
			return curr.Content(content)
		}

		child := curr.ChildByFieldName("declarator")
		if child != nil {
			curr = child
			continue
		}

		found := false
		for i := 0; i < int(curr.NamedChildCount()); i++ {
			c := curr.NamedChild(i)
			if strings.Contains(c.Type(), "declarator") || c.Type() == "identifier" {
				curr = c
				found = true
				break
			}
		}
		if found {
			continue
		}
		break
	}
	return "unknown_function"
}

func countCCppParameters(declarator *sitter.Node) int {
	count := 0
	var visit func(*sitter.Node)
	visit = func(n *sitter.Node) {
		if n.Type() == "parameter_declaration" {
			count++
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			visit(n.NamedChild(i))
		}
	}
	visit(declarator)
	return count
}

func calculateCCppCyclomatic(body *sitter.Node, content []byte) int {
	complexity := 1

	var visit func(*sitter.Node)
	visit = func(n *sitter.Node) {
		t := n.Type()
		switch t {
		case "if_statement", "while_statement", "for_statement", "range_based_for_statement",
			"do_statement", "case_statement", "catch_clause", "conditional_expression":
			complexity++
		case "binary_expression":
			op := n.ChildByFieldName("operator")
			if op != nil {
				opStr := op.Content(content)
				if opStr == "&&" || opStr == "||" {
					complexity++
				}
			}
		}

		for i := 0; i < int(n.NamedChildCount()); i++ {
			visit(n.NamedChild(i))
		}
	}

	if body != nil {
		visit(body)
	}

	return complexity
}

func calculateCCppCognitive(node *sitter.Node, content []byte) int {
	complexity := 0

	WalkTree(node, func(n *sitter.Node) {
		nodeType := n.Type()
		switch nodeType {
		case "if_statement", "while_statement", "for_statement", "switch_statement", "do_statement", "catch_clause", "goto_statement":
			complexity += 2
		case "binary_expression":
			op := n.ChildByFieldName("operator")
			if op != nil {
				opStr := op.Content(content)
				if opStr == "&&" || opStr == "||" {
					complexity += 1
				}
			}
		}
	})

	return complexity
}

func calculateCCppNesting(node *sitter.Node) int {
	maxDepth := 0
	var visit func(*sitter.Node, int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		t := n.Type()
		switch t {
		case "if_statement", "while_statement", "for_statement", "switch_statement", "do_statement", "catch_clause":
			newDepth++
			if newDepth > maxDepth {
				maxDepth = newDepth
			}
		}

		for i := 0; i < int(n.NamedChildCount()); i++ {
			visit(n.NamedChild(i), newDepth)
		}
	}

	visit(node, 0)
	return maxDepth
}

func generateCCppRefactoringSuggestions(cyclomatic, cognitive, nesting, paramCount, loc int) []models.RefactoringSuggestion {
	var suggestions []models.RefactoringSuggestion

	if cyclomatic > 15 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Extract Functions",
			Description: "Break down this function into smaller, focused functions. Consider using inline functions for performance-critical paths",
		})
	}

	if nesting > 3 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Reduce Nesting Depth",
			Description: "Use early returns, guard clauses, or extract nested logic into helper functions",
		})
	}

	if paramCount > 5 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Too Many Parameters",
			Description: "Consider using a struct/class to group related parameters or use parameter objects",
		})
	}

	if loc > 50 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Function Too Long",
			Description: "Split this function into smaller functions. Consider separating algorithm from data structure manipulation",
		})
	}

	if cognitive > 20 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Simplify Logic",
			Description: "Simplify control flow, reduce pointer complexity, or use RAII patterns to improve readability",
		})
	}

	return suggestions
}

func estimateCCppTechnicalDebt(cyclomatic, cognitive, loc int) int {
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

	cppTax := 3

	return baseMinutes + complexityMinutes + cognitiveMinutes + locMinutes + cppTax
}
