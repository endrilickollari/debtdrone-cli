package complexity

import (
	"context"
	"regexp"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/cpp"
)

// CCppAnalyzer analyzes C/C++ code for complexity metrics
type CCppAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewCCppAnalyzer creates a new C/C++ complexity analyzer
func NewCCppAnalyzer(thresholds models.ComplexityThresholds) *CCppAnalyzer {
	return &CCppAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *CCppAnalyzer) Language() string {
	return "C/C++"
}

// AnalyzeFile analyzes a C/C++ file and returns complexity metrics
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

		cognitive := calculateCCppCognitive(fn.BodyContent)
		nesting := calculatePatternBasedNesting(fn.BodyContent)
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

// findCCppFunctions finds function definitions using Tree-sitter
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

func calculateCCppCognitive(code string) int {
	cognitive := 0
	nestingPatterns := []string{
		`\bif\b`, `\bwhile\b`, `\bfor\b`, `\bswitch\b`, `\bdo\b`, `\btry\b`,
	}

	for _, pattern := range nestingPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		cognitive += len(matches) * 2
	}

	logicalOps := regexp.MustCompile(`&&|\|\|`)
	cognitive += len(logicalOps.FindAllString(code, -1))

	gotoPattern := regexp.MustCompile(`\bgoto\b`)
	cognitive += len(gotoPattern.FindAllString(code, -1)) * 4

	pointerOps := regexp.MustCompile(`\*\w+|\w+\*|->|->\*|\.\*`)
	cognitive += len(pointerOps.FindAllString(code, -1)) / 3

	templatePattern := regexp.MustCompile(`template\s*<`)
	cognitive += len(templatePattern.FindAllString(code, -1)) * 3

	macroPattern := regexp.MustCompile(`#define`)
	cognitive += len(macroPattern.FindAllString(code, -1)) * 2

	return cognitive
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
