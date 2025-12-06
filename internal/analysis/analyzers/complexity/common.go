package complexity

import (
	"regexp"
	"strings"
)

type functionInfo struct {
	name       string
	line       int
	endLine    int
	body       string
	paramCount int
}

// extractFunctionBody extracts the body of a brace-based function (JS, Java, etc.)
func extractFunctionBody(lines []string, startLine, maxLines int) string {
	var body []string
	braceCount := 0
	inFunction := false

	for i := startLine; i < len(lines) && i < startLine+maxLines; i++ {
		line := lines[i]
		body = append(body, line)

		for _, char := range line {
			switch char {
			case '{':
				braceCount++
				inFunction = true
			case '}':
				braceCount--
				if inFunction && braceCount == 0 {
					return strings.Join(body, "\n")
				}
			}
		}
	}

	return strings.Join(body, "\n")
}

// calculatePatternBasedCyclomatic calculates cyclomatic complexity using pattern matching
func calculatePatternBasedCyclomatic(code string) int {
	complexity := 1

	patterns := []string{
		`\bif\b`, `\belse\s+if\b`, `\belif\b`, // conditionals
		`\bfor\b`, `\bforeach\b`, `\bwhile\b`, // loops
		`\bcase\b`, `\bcatch\b`, // switch/try-catch
		`&&`, `\|\|`, // logical operators
		`\?\s*:`, // ternary operator
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		complexity += len(matches)
	}

	return complexity
}

// calculatePatternBasedCognitive estimates cognitive complexity
func calculatePatternBasedCognitive(code string) int {
	cognitive := 0

	nestingPatterns := []string{
		`\bif\b`, `\bfor\b`, `\bwhile\b`, `\bswitch\b`, `\btry\b`,
		`\belif\b`, // Python
	}

	for _, pattern := range nestingPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(code, -1)
		cognitive += len(matches) * 2
	}

	logicalOps := regexp.MustCompile(`\b\&\&\b|\b\|\|\b`)
	cognitive += len(logicalOps.FindAllString(code, -1))

	return cognitive
}

// calculatePatternBasedNesting estimates maximum nesting depth
func calculatePatternBasedNesting(code string) int {
	maxDepth := 0
	currentDepth := 0

	for _, char := range code {
		switch char {
		case '{':
			currentDepth++
			if currentDepth > maxDepth {
				maxDepth = currentDepth
			}
		case '}':
			currentDepth--
		}
	}

	return maxDepth
}

// classifyComplexitySeverity determines severity based on metrics
func classifyComplexitySeverity(cyclomatic, cognitive, nesting int) string {
	if cyclomatic > 20 || cognitive > 25 || nesting > 5 {
		return "critical"
	} else if cyclomatic > 15 || cognitive > 20 || nesting > 4 {
		return "high"
	} else if cyclomatic > 10 || cognitive > 15 || nesting > 3 {
		return "medium"
	}
	return "low"
}

// truncateSnippet truncates code snippet to specified length
func truncateSnippet(code string, maxLen int) string {
	if len(code) <= maxLen {
		return code
	}
	return code[:maxLen] + "..."
}

// countParameterString counts the number of parameters in a parameter string
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
