package complexity

import (
	"strings"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	sitter "github.com/smacker/go-tree-sitter"
)

type functionInfo struct {
	name       string
	line       int
	endLine    int
	body       string
	paramCount int
	Node       *sitter.Node
}

func WalkTree(node *sitter.Node, visitor func(*sitter.Node)) {
	if node == nil {
		return
	}

	cursor := sitter.NewTreeCursor(node)
	defer cursor.Close()

	visitor(node)

	for {

		if cursor.GoToFirstChild() {
			visitor(cursor.CurrentNode())
			continue
		}

		if cursor.GoToNextSibling() {
			visitor(cursor.CurrentNode())
			continue
		}

		for cursor.GoToParent() {

			if cursor.CurrentNode().Equal(node) {
				return
			}
			if cursor.GoToNextSibling() {
				visitor(cursor.CurrentNode())
				goto NextNode
			}
		}

		break

	NextNode:
	}
}

func classifyComplexitySeverity(cyclomatic, cognitive, nesting int, thresholds models.ComplexityThresholds) string {
	if cyclomatic > thresholds.CyclomaticCritical || cognitive > 25 || nesting > 5 {
		return "critical"
	} else if cyclomatic > thresholds.CyclomaticHigh || cognitive > 20 || nesting > 4 {
		return "high"
	} else if cyclomatic > thresholds.CyclomaticHigh*2/3 || cognitive > 15 || nesting > 3 {
		return "medium"
	}
	return "low"
}

func truncateSnippet(code string, maxLen int) string {
	if len(code) <= maxLen {
		return code
	}
	return code[:maxLen] + "..."
}

func fullFunctionCode(code string) string {
	return code
}

func countParameterString(params string) int {
	if strings.TrimSpace(params) == "" {
		return 0
	}

	count := 1
	for _, char := range params {
		if char == ',' {
			count++
		}
	}
	return count
}
