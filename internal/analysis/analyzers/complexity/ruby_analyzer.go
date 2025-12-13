package complexity

import (
	"context"
	"fmt"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/ruby"
)

type RubyAnalyzer struct {
	thresholds models.ComplexityThresholds
}

func NewRubyAnalyzer(thresholds models.ComplexityThresholds) *RubyAnalyzer {
	return &RubyAnalyzer{
		thresholds: thresholds,
	}
}

func (a *RubyAnalyzer) Language() string {
	return "Ruby"
}

func (a *RubyAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	var metrics []models.ComplexityMetric

	functions, err := findRubyMethods(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ruby methods: %w", err)
	}

	for _, fn := range functions {
		cyclomatic := calculateRubyCyclomatic(fn.node, content)

		cognitive := calculateRubyCognitive(fn.node, content)
		nesting := calculateRubyNesting(fn.node)
		loc := strings.Count(fn.body, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)
		debtMinutes := estimateTechnicalDebt(cyclomatic, cognitive, loc)
		suggestions := generateRubyRefactoringSuggestions(cyclomatic, cognitive, nesting, fn.paramCount, loc)

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

type rubyFunctionInfo struct {
	name       string
	line       int
	endLine    int
	body       string
	paramCount int
	node       *sitter.Node
}

func findRubyMethods(content []byte) ([]rubyFunctionInfo, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(ruby.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}

	queryStr := `
		(method
			name: (identifier) @name
			parameters: (method_parameters)? @params
			body: (_)? @body
		) @method

		(singleton_method
			object: (_)
			name: (identifier) @name
			parameters: (method_parameters)? @params
			body: (_)? @body
		) @method
	`

	q, err := sitter.NewQuery([]byte(queryStr), ruby.GetLanguage())
	if err != nil {
		return nil, err
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(q, tree.RootNode())

	var functions []rubyFunctionInfo

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}

		var fnName string
		var fnBodyNode *sitter.Node
		var fnNode *sitter.Node
		var paramNode *sitter.Node

		for _, c := range m.Captures {
			captureName := q.CaptureNameForId(c.Index)
			switch captureName {
			case "method":
				fnNode = c.Node
			case "name":
				fnName = c.Node.Content(content)
			case "params":
				paramNode = c.Node
			case "body":
				fnBodyNode = c.Node
			}
		}

		if fnNode != nil && fnName != "" {
			bodyContent := ""
			if fnBodyNode != nil {
				bodyContent = fnNode.Content(content)
			} else {
				bodyContent = fnNode.Content(content)
			}

			paramCount := countRubyParameters(paramNode)

			functions = append(functions, rubyFunctionInfo{
				name:       fnName,
				line:       int(fnNode.StartPoint().Row) + 1,
				endLine:    int(fnNode.EndPoint().Row) + 1,
				body:       bodyContent,
				paramCount: paramCount,
				node:       fnNode,
			})
		}
	}

	return functions, nil
}

func countRubyParameters(paramNode *sitter.Node) int {
	if paramNode == nil {
		return 0
	}
	count := 0
	for i := 0; i < int(paramNode.ChildCount()); i++ {
		child := paramNode.Child(i)
		if child.Type() == "(" || child.Type() == ")" || child.Type() == "," {
			continue
		}
		count++
	}
	return count
}

func calculateRubyCyclomatic(node *sitter.Node, content []byte) int {
	complexity := 1
	cursor := sitter.NewTreeCursor(node)
	defer cursor.Close()

	for {
		n := cursor.CurrentNode()

		if n.IsNamed() {
			nodeType := n.Type()
			switch nodeType {
			case "if", "if_modifier", "unless", "unless_modifier", "elsif":
				complexity++
			case "while", "while_modifier", "until", "until_modifier", "for":
				complexity++
			case "when":
				complexity++
			case "rescue":
				complexity++
			case "conditional":
				complexity++
			case "binary":
				for i := 0; i < int(n.ChildCount()); i++ {
					child := n.Child(i)
					op := child.Content(content)
					switch op {
					case "&&", "||", "and", "or":
						complexity++
					}
				}
			case "block", "do_block", "brace_block":
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

func calculateRubyCognitive(node *sitter.Node, content []byte) int {
	complexity := 0

	WalkTree(node, func(n *sitter.Node) {
		nodeType := n.Type()
		switch nodeType {
		case "if", "unless", "case", "while", "until", "for", "rescue":
			complexity += 2
		case "binary":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				op := child.Content(content)
				switch op {
				case "&&", "||", "and", "or":
					complexity += 1
				}
			}
		}
	})

	return complexity
}

func calculateRubyNesting(node *sitter.Node) int {
	maxDepth := 0
	var visit func(*sitter.Node, int)
	visit = func(n *sitter.Node, depth int) {
		if n == nil {
			return
		}

		newDepth := depth
		t := n.Type()
		switch t {
		case "if", "unless", "case", "while", "until", "for", "rescue", "begin":
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

func generateRubyRefactoringSuggestions(cyclomatic, cognitive, nesting, paramCount, loc int) []models.RefactoringSuggestion {
	var suggestions []models.RefactoringSuggestion

	if cyclomatic > 15 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Extract Methods",
			Description: "Break down this method into smaller, focused methods using Ruby's expressive syntax",
		})
	}

	if nesting > 3 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Reduce Nesting Depth",
			Description: "Use Ruby's guard clauses, early returns, or extract nested logic into separate methods",
		})
	}

	if paramCount > 4 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Introduce Parameter Object",
			Description: "Consider using a hash or creating a parameter object to group related parameters",
		})
	}

	if loc > 50 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "high",
			Title:       "Method Too Long",
			Description: "Split this method into smaller methods. Consider using Ruby modules or service objects",
		})
	}

	if cognitive > 20 {
		suggestions = append(suggestions, models.RefactoringSuggestion{
			Priority:    "medium",
			Title:       "Simplify Logic",
			Description: "Use Ruby idioms like safe navigation (&.), try, or early returns to simplify logic",
		})
	}

	return suggestions
}

func estimateTechnicalDebt(cyclomatic, cognitive, loc int) int {
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
