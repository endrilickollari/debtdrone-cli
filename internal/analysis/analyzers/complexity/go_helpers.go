package complexity

import (
	"fmt"
	"go/ast"
	"go/token"
	"io/ioutil"
	"strings"
)

// calculateCyclomaticComplexity calculates McCabe cyclomatic complexity
func calculateCyclomaticComplexity(fn *ast.FuncDecl) int {
	complexity := 1

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			complexity++
		case *ast.ForStmt:
			complexity++
		case *ast.RangeStmt:
			complexity++
		case *ast.CaseClause:
			complexity++
		case *ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			if node.Op == token.LAND || node.Op == token.LOR {
				complexity++
			}
		}
		return true
	})

	return complexity
}

// calculateCognitiveComplexity calculates cognitive complexity
func calculateCognitiveComplexity(fn *ast.FuncDecl) int {
	complexity := 0

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			complexity++
			if node.Else != nil {
				complexity++
			}
		case *ast.ForStmt:
			complexity++
		case *ast.RangeStmt:
			complexity++
		case *ast.SwitchStmt:
			complexity++
		case *ast.TypeSwitchStmt:
			complexity++
		case *ast.SelectStmt:
			complexity++
		case *ast.BinaryExpr:
			if node.Op == token.LAND || node.Op == token.LOR {
				complexity++
			}
		}
		return true
	})

	return complexity
}

// calculateNestingDepth calculates maximum nesting depth
func calculateNestingDepth(body *ast.BlockStmt) int {
	maxDepth := 0

	var measureDepth func(ast.Node, int)
	measureDepth = func(n ast.Node, depth int) {
		if n == nil {
			return
		}

		if depth > maxDepth {
			maxDepth = depth
		}

		switch node := n.(type) {
		case *ast.IfStmt:
			if node.Body != nil {
				for _, stmt := range node.Body.List {
					measureDepth(stmt, depth+1)
				}
			}
			if node.Else != nil {
				measureDepth(node.Else, depth+1)
			}

		case *ast.ForStmt:
			if node.Body != nil {
				for _, stmt := range node.Body.List {
					measureDepth(stmt, depth+1)
				}
			}

		case *ast.RangeStmt:
			if node.Body != nil {
				for _, stmt := range node.Body.List {
					measureDepth(stmt, depth+1)
				}
			}

		case *ast.SwitchStmt:
			if node.Body != nil {
				for _, stmt := range node.Body.List {
					measureDepth(stmt, depth+1)
				}
			}

		case *ast.TypeSwitchStmt:
			if node.Body != nil {
				for _, stmt := range node.Body.List {
					measureDepth(stmt, depth+1)
				}
			}

		case *ast.SelectStmt:
			if node.Body != nil {
				for _, stmt := range node.Body.List {
					measureDepth(stmt, depth+1)
				}
			}

		case *ast.BlockStmt:
			for _, stmt := range node.List {
				measureDepth(stmt, depth)
			}

		default:
			ast.Inspect(n, func(child ast.Node) bool {
				if child != nil && child != n {
					measureDepth(child, depth)
					return false
				}
				return child == nil
			})
		}
	}

	if body != nil {
		for _, stmt := range body.List {
			measureDepth(stmt, 0)
		}
	}

	return maxDepth
}

// countParameters counts the number of parameters in a function
func countParameters(fn *ast.FuncDecl) int {
	if fn.Type.Params == nil {
		return 0
	}

	count := 0
	for _, field := range fn.Type.Params.List {
		count += len(field.Names)
		if len(field.Names) == 0 {
			count++
		}
	}
	return count
}

// extractReceiverType extracts the receiver type name
func extractReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return fmt.Sprintf("*%s", extractReceiverType(t.X))
	case *ast.Ident:
		return t.Name
	default:
		return "unknown"
	}
}

// extractCodeSnippet extracts a limited code snippet for the function
func extractCodeSnippet(fset *token.FileSet, fn *ast.FuncDecl) string {
	startPos := fset.Position(fn.Pos())
	endPos := fset.Position(fn.End())

	content, err := ioutil.ReadFile(startPos.Filename)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(content), "\n")
	if startPos.Line <= 0 || endPos.Line > len(lines) {
		return ""
	}

	maxLines := 20
	funcLines := lines[startPos.Line-1 : endPos.Line]
	if len(funcLines) > maxLines {
		funcLines = funcLines[:maxLines]
		funcLines = append(funcLines, "... (truncated)")
	}

	return strings.Join(funcLines, "\n")
}
