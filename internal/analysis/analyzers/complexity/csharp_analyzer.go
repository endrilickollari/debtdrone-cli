package complexity

import (
	"context"
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

	root := tree.RootNode()
	functions := findCSharpFunctions(root, content)

	for _, fn := range functions {
		cyclomatic := calculateCSharpCyclomatic(fn.Node, content)
		cognitive := calculatePatternBasedCognitive(fn.BodyContent)
		nesting := calculatePatternBasedNesting(fn.BodyContent)
		loc := strings.Count(fn.BodyContent, "\n") + 1

		severity := classifyComplexitySeverity(cyclomatic, cognitive, nesting)

		cognitivePtr := cognitive
		snippetStr := truncateSnippet(fn.BodyContent, 300)

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
	`
	q, _ := sitter.NewQuery([]byte(queryStr), csharp.GetLanguage())
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	defer q.Close()

	qc.Exec(q, root)

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		var nameNode, bodyNode *sitter.Node

		for _, c := range match.Captures {
			name := q.CaptureNameForId(c.Index)
			if name == "name" {
				nameNode = c.Node
			} else if name == "body" {
				bodyNode = c.Node
			}
		}

		if nameNode == nil || bodyNode == nil {
			continue
		}

		paramCount := 0
		var parent *sitter.Node
		if nameNode.Parent() != nil {
			parent = nameNode.Parent()
			paramList := parent.ChildByFieldName("parameters")
			if paramList != nil {
				paramCount = countCSharpParameters(paramList)
			}
		}

		functions = append(functions, cSharpFunctionInfo{
			Name:        nameNode.Content(content),
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

func calculateCSharpCyclomatic(node *sitter.Node, content []byte) int {
	complexity := 1
	if node == nil {
		return complexity
	}

	var visit func(*sitter.Node)
	visit = func(n *sitter.Node) {
		t := n.Type()
		switch t {
		case "if_statement", "for_statement", "foreach_statement", "while_statement", "do_statement", "catch_clause", "conditional_expression":
			complexity++
		case "case_switch_label":
			complexity++
		case "binary_expression":
			op := n.ChildByFieldName("operator")
			if op != nil {
				opStr := op.Content(content)
				if opStr == "&&" || opStr == "||" || opStr == "??" {
					complexity++
				}
			}
		}

		for i := 0; i < int(n.NamedChildCount()); i++ {
			visit(n.NamedChild(i))
		}
	}

	visit(node)
	return complexity
}
