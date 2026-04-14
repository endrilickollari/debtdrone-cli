package complexity

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
)

// GoAnalyzer analyzes Go code for complexity metrics
type GoAnalyzer struct {
	thresholds models.ComplexityThresholds
}

// NewGoAnalyzer creates a new Go complexity analyzer
func NewGoAnalyzer(thresholds models.ComplexityThresholds) *GoAnalyzer {
	return &GoAnalyzer{
		thresholds: thresholds,
	}
}

// Language returns the language this analyzer supports
func (a *GoAnalyzer) Language() string {
	return "Go"
}

// AnalyzeFile analyzes a single Go file and returns complexity metrics
func (a *GoAnalyzer) AnalyzeFile(filePath string, content []byte) ([]models.ComplexityMetric, error) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	metrics := []models.ComplexityMetric{}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			metric := a.analyzeFunction(fset, node.Body, node.Name.Name, node.Type, node.Recv, node.Pos(), node.End(), node, filePath, content)
			if metric != nil {
				metrics = append(metrics, *metric)
			}
		case *ast.FuncLit:
			metric := a.analyzeFunction(fset, node.Body, "<anonymous>", node.Type, nil, node.Pos(), node.End(), node, filePath, content)
			if metric != nil {
				metrics = append(metrics, *metric)
			}
		}

		return true
	})

	return metrics, nil
}

// analyzeFunction analyzes a single function and returns its complexity metrics
func (a *GoAnalyzer) analyzeFunction(
	fset *token.FileSet,
	body *ast.BlockStmt,
	name string,
	funcType *ast.FuncType,
	recv *ast.FieldList,
	pos token.Pos,
	end token.Pos,
	node ast.Node,
	filePath string,
	content []byte,
) *models.ComplexityMetric {
	if body == nil {
		return nil
	}

	funcName := name
	if recv != nil && len(recv.List) > 0 {
		recvType := extractReceiverType(recv.List[0].Type)
		funcName = fmt.Sprintf("(%s).%s", recvType, name)
	}

	startPos := fset.Position(pos)
	endPos := fset.Position(end)

	nodes := mapGoNodes(body)
	cyclomaticComplexity, cognitiveComplexity, nestingDepth := CalculateComplexity(nodes)
	paramCount := countParameters(funcType)
	loc := endPos.Line - startPos.Line + 1

	codeSnippet := extractCodeSnippet(fset, node, content)

	debtMinutes := models.CalculateTechnicalDebt(
		cyclomaticComplexity,
		cognitiveComplexity,
		nestingDepth,
		paramCount,
		loc,
	)

	severity := a.thresholds.DetermineSeverity(
		cyclomaticComplexity,
		cognitiveComplexity,
		nestingDepth,
		paramCount,
	)

	suggestions := models.GenerateRefactoringSuggestions(
		cyclomaticComplexity,
		cognitiveComplexity,
		nestingDepth,
		paramCount,
		loc,
	)

	metric := &models.ComplexityMetric{
		FilePath:               filePath,
		FunctionName:           funcName,
		StartLine:              startPos.Line,
		EndLine:                endPos.Line,
		StartColumn:            &startPos.Column,
		EndColumn:              &endPos.Column,
		CyclomaticComplexity:   cyclomaticComplexity,
		CognitiveComplexity:    &cognitiveComplexity,
		NestingDepth:           nestingDepth,
		ParameterCount:         paramCount,
		LinesOfCode:            loc,
		Severity:               severity,
		TechnicalDebtMinutes:   debtMinutes,
		CodeSnippet:            &codeSnippet,
		RefactoringSuggestions: suggestions,
		Language:               "Go",
	}

	return metric
}
