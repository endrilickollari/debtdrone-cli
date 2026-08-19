package complexity

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

func mapGoNodes(body *ast.BlockStmt) []Node {
	var nodes []Node
	var depthStack []bool
	depth := 0

	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			if len(depthStack) > 0 {
				if depthStack[len(depthStack)-1] {
					depth--
				}
				depthStack = depthStack[:len(depthStack)-1]
			}
			return true
		}

		addsDepth := false
		switch node := n.(type) {
		case *ast.IfStmt, *ast.CaseClause, *ast.CommClause:
			nodes = append(nodes, Node{Type: Branch, Depth: depth})
			addsDepth = true
		case *ast.ForStmt, *ast.RangeStmt:
			nodes = append(nodes, Node{Type: Loop, Depth: depth})
			addsDepth = true
		case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			nodes = append(nodes, Node{Type: Nesting, Depth: depth})
			addsDepth = true
		case *ast.BinaryExpr:
			if node.Op == token.LAND || node.Op == token.LOR {
				nodes = append(nodes, Node{Type: Operator, Depth: depth})
			}
		}

		if addsDepth {
			depth++
		}
		depthStack = append(depthStack, addsDepth)

		return true
	})

	return nodes
}

func countParameters(funcType *ast.FuncType) int {
	if funcType.Params == nil {
		return 0
	}

	count := 0
	for _, field := range funcType.Params.List {
		count += len(field.Names)
		if len(field.Names) == 0 {
			count++
		}
	}
	return count
}

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

func extractCodeSnippet(fset *token.FileSet, node ast.Node, content []byte) string {
	startPos := fset.Position(node.Pos())
	endPos := fset.Position(node.End())

	if len(content) == 0 {
		return ""
	}

	lines := strings.Split(string(content), "\n")
	if startPos.Line <= 0 || endPos.Line > len(lines) {
		return ""
	}

	funcLines := lines[startPos.Line-1 : endPos.Line]

	// Bound snippets independently of the repository file-size policy.
	if len(funcLines) > 1000 {
		funcLines = funcLines[:1000]
		funcLines = append(funcLines, "... // Code snippet truncated (exceeded 1000 lines limit)")
	}

	return strings.Join(funcLines, "\n")
}
