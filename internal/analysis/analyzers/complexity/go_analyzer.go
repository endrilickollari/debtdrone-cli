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
		fn, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		metric := a.analyzeFunction(fset, fn, filePath)
		if metric != nil {
			metrics = append(metrics, *metric)
		}

		return true
	})

	return metrics, nil
}

// analyzeFunction analyzes a single function and returns its complexity metrics
func (a *GoAnalyzer) analyzeFunction(fset *token.FileSet, fn *ast.FuncDecl, filePath string) *models.ComplexityMetric {
	if fn.Body == nil {
		return nil
	}

	funcName := fn.Name.Name
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recvType := extractReceiverType(fn.Recv.List[0].Type)
		funcName = fmt.Sprintf("(%s).%s", recvType, fn.Name.Name)
	}

	startPos := fset.Position(fn.Pos())
	endPos := fset.Position(fn.End())

	cyclomaticComplexity := calculateCyclomaticComplexity(fn)
	cognitiveComplexity := calculateCognitiveComplexity(fn)
	nestingDepth := calculateNestingDepth(fn.Body)
	paramCount := countParameters(fn)
	loc := endPos.Line - startPos.Line + 1

	codeSnippet := extractCodeSnippet(fset, fn)

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
