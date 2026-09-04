package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/mattn/go-runewidth"
)

// truncate shortens a value to fit maxWidth terminal columns, marking the cut
// with an ellipsis. It measures display width rather than bytes, so accented
// and full-width characters are counted as the terminal draws them and a
// multi-byte character is never split into invalid output.
func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return "…"
	}

	// One column is reserved for the ellipsis.
	var (
		builder strings.Builder
		width   int
	)
	for _, r := range s {
		runeWidth := runewidth.RuneWidth(r)
		if width+runeWidth > maxWidth-1 {
			break
		}
		builder.WriteRune(r)
		width += runeWidth
	}
	return builder.String() + "…"
}

// truncateLeft keeps the end of a value visible. It is used for editable input
// when the cursor has moved beyond the right edge of the available space.
func truncateLeft(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	if maxWidth == 1 {
		return "…"
	}

	runes := []rune(s)
	width := 0
	start := len(runes)
	for start > 0 {
		runeWidth := runewidth.RuneWidth(runes[start-1])
		if width+runeWidth > maxWidth-1 {
			break
		}
		start--
		width += runeWidth
	}
	return "…" + string(runes[start:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}

func isEditableChar(s string) bool {
	runes := []rune(s)
	return len(runes) == 1 && unicode.IsPrint(runes[0])
}

func splitHeight(totalHeight int) (listH, detailH int) {
	return splitHeightWithChrome(totalHeight, 0)
}

// splitHeightWithChrome divides the terminal between the list and detail panes.
// extraChrome reserves rows for screen-specific furniture such as the results
// summary band, so the two panes shrink instead of overflowing the viewport.
func splitHeightWithChrome(totalHeight, extraChrome int) (listH, detailH int) {
	const chrome = 8
	// The panes shrink to whatever the terminal leaves rather than holding a
	// comfortable minimum that would push the screen past the bottom edge.
	available := max(totalHeight-chrome-extraChrome, 2)
	listH = max(available*6/10, 1)
	detailH = max(available-listH, 1)
	return
}

// hangingValue wraps a value beside its label, indenting every continuation
// line to the value column. Without the indent a wrapped path resumes under the
// labels and reads as though it were one.
func hangingValue(text string, labelWidth, wrapWidth int) string {
	wrapped := lipgloss.NewStyle().Foreground(colorText).Width(wrapWidth).Render(text)
	lines := strings.Split(wrapped, "\n")
	for index := 1; index < len(lines); index++ {
		lines[index] = strings.Repeat(" ", labelWidth) + lines[index]
	}
	return strings.Join(lines, "\n")
}

func countBySeverity(issues []models.TechnicalDebtIssue, sev string) int {
	n := 0
	for _, iss := range issues {
		if strings.EqualFold(iss.Severity, sev) {
			n++
		}
	}
	return n
}

func formatIssueDetail(issue *models.TechnicalDebtIssue, width int) string {
	if issue == nil {
		return "\n" + lipgloss.NewStyle().
			Foreground(colorDim).
			PaddingLeft(2).
			Render("Select an issue with j/k to view details here.")
	}

	const labelW = 14
	labelStyle := lipgloss.NewStyle().Foreground(colorDim).Bold(true).Width(labelW)
	valueStyle := lipgloss.NewStyle().Foreground(colorText)
	accentStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)

	label := func(s string) string { return labelStyle.Render(s) }
	value := func(s string) string { return valueStyle.Render(s) }

	sev := issue.Severity
	if sev == "" {
		sev = "unknown"
	}
	sevRendered := lipgloss.NewStyle().
		Foreground(severityColor(sev)).
		Bold(true).
		Render(strings.ToUpper(sev))

	lineNum := "—"
	if issue.LineNumber != nil {
		if issue.ColumnNumber != nil {
			lineNum = fmt.Sprintf("%d:%d", *issue.LineNumber, *issue.ColumnNumber)
		} else {
			lineNum = fmt.Sprintf("%d", *issue.LineNumber)
		}
	}

	ruleID := "—"
	if issue.ToolRuleID != nil && *issue.ToolRuleID != "" {
		ruleID = *issue.ToolRuleID
	}

	wrapW := max(width-labelW-2, 20)

	var b strings.Builder
	b.WriteString("\n")
	// The full path is wrapped rather than truncated: the findings table shows
	// only the file name, so this is where a reader inspects the whole value.
	b.WriteString(label("Full Path") + hangingValue(issue.FilePath, labelW, wrapW) + "\n")
	b.WriteString(label("Line") + value(lineNum) + "\n")
	b.WriteString(label("Severity") + sevRendered + "\n")
	b.WriteString(label("Category") + value(issue.Category) + "\n")
	b.WriteString(label("Issue Type") + value(issue.IssueType) + "\n")
	b.WriteString(label("Rule ID") + value(ruleID) + "\n")
	b.WriteString(label("Tool") + value(issue.ToolName) + "\n")

	if issue.TechnicalDebtHours > 0 {
		b.WriteString(label("Debt Hours") + value(fmt.Sprintf("%.1fh", issue.TechnicalDebtHours)) + "\n")
	}
	if issue.EffortMultiplier > 0 {
		b.WriteString(label("Effort") + value(fmt.Sprintf("%.1f×", issue.EffortMultiplier)) + "\n")
	}
	if issue.ConfidenceScore > 0 {
		b.WriteString(label("Confidence") + value(fmt.Sprintf("%.0f%%", issue.ConfidenceScore*100)) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(accentStyle.Render("Message") + "\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(colorText).
		Width(wrapW+labelW).
		Render(issue.Message) + "\n")

	if issue.Description != nil && *issue.Description != "" {
		b.WriteString("\n")
		b.WriteString(accentStyle.Render("Details") + "\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorText).
			Width(wrapW+labelW).
			Render(*issue.Description) + "\n")
	}

	if issue.CodeSnippet != nil && *issue.CodeSnippet != "" {
		b.WriteString("\n")
		b.WriteString(accentStyle.Render("Evidence") + "\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorFilePath).
			Render(*issue.CodeSnippet) + "\n")
	}

	if issue.SurroundingContext != nil && *issue.SurroundingContext != "" {
		b.WriteString("\n")
		b.WriteString(accentStyle.Render("Surrounding Context") + "\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorFilePath).
			Render(*issue.SurroundingContext) + "\n")
	}

	return b.String()
}

func formatHistoryDetail(e historyEntry, width int) string {
	if e.run.ID.String() == "00000000-0000-0000-0000-000000000000" {
		return "\n" + lipgloss.NewStyle().
			Foreground(colorDim).
			PaddingLeft(2).
			Render("Select a scan with j/k to view details here.")
	}

	run := e.run

	const labelW = 16
	labelStyle := lipgloss.NewStyle().Foreground(colorDim).Bold(true).Width(labelW)
	valueStyle := lipgloss.NewStyle().Foreground(colorText)
	accentStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)

	label := func(s string) string { return labelStyle.Render(s) }
	value := func(s string) string { return valueStyle.Render(s) }

	badge := func(col lipgloss.Color, name string, count int) string {
		return lipgloss.NewStyle().Foreground(col).Bold(true).
			Render(fmt.Sprintf("%-10s", name)) +
			lipgloss.NewStyle().Foreground(col).
				Render(fmt.Sprintf("%d", count))
	}

	duration := "—"
	if run.DurationSeconds != nil {
		d := time.Duration(*run.DurationSeconds) * time.Second
		duration = d.String()
	}

	branch := "—"
	if run.Branch != nil && *run.Branch != "" {
		branch = *run.Branch
	}
	commit := "—"
	if run.CommitHash != nil && *run.CommitHash != "" {
		c := *run.CommitHash
		if len(c) > 12 {
			c = c[:12]
		}
		commit = c
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(label("Repository") +
		lipgloss.NewStyle().Foreground(colorText).Width(width-labelW-2).Render(e.path) + "\n")
	b.WriteString(label("Scan Date") + value(run.StartedAt.Format("2006-01-02  15:04:05")) + "\n")
	b.WriteString(label("Duration") + value(duration) + "\n")
	if branch != "—" || commit != "—" {
		b.WriteString(label("Branch") + value(branch) + "\n")
		b.WriteString(label("Commit") + value(commit) + "\n")
	}
	b.WriteString("\n")

	b.WriteString(accentStyle.Render("Issue Summary") + "\n")
	b.WriteString("\n")

	totalLabel := lipgloss.NewStyle().Foreground(colorDim).Bold(true).Width(labelW).Render("Total Issues")
	totalVal := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true).
		Render(fmt.Sprintf("%d", run.TotalIssuesFound))
	b.WriteString(totalLabel + totalVal + "\n")
	b.WriteString("\n")

	b.WriteString(label("") + badge(colorCritical, "Critical", run.CriticalIssuesCount) + "\n")
	b.WriteString(label("") + badge(colorHigh, "High", run.HighIssuesCount) + "\n")
	b.WriteString(label("") + badge(colorMedium, "Medium", run.MediumIssuesCount) + "\n")
	b.WriteString(label("") + badge(colorLow, "Low", run.LowIssuesCount) + "\n")

	if run.TotalTechnicalDebtHours > 0 {
		b.WriteString("\n")
		b.WriteString(label("Debt Hours") + value(fmt.Sprintf("%.1fh", run.TotalTechnicalDebtHours)) + "\n")
	}

	b.WriteString("\n")
	enterKey := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true).Render("[Enter]")
	promptText := lipgloss.NewStyle().Foreground(colorDim).Render(" to browse full scan results")
	b.WriteString("  " + enterKey + " " + promptText + "\n")

	return b.String()
}
