package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
)

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
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

func isEditableChar(s string) bool {
	if len(s) != 1 {
		return false
	}
	r := rune(s[0])
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == ' ' || r == '-' || r == '_' ||
		r == '.' || r == '/' || r == ':' || r == '%'
}

func splitHeight(totalHeight int) (listH, detailH int) {
	const chrome = 8
	available := totalHeight - chrome
	if available < 10 {
		available = 10
	}
	listH = available * 6 / 10
	detailH = available - listH
	if listH < 4 {
		listH = 4
	}
	if detailH < 3 {
		detailH = 3
	}
	return
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
	b.WriteString(label("Full Path") + lipgloss.NewStyle().Foreground(colorText).Width(wrapW).Render(issue.FilePath) + "\n")
	b.WriteString(label("Line") + value(lineNum) + "\n")
	b.WriteString(label("Severity") + sevRendered + "\n")
	b.WriteString(label("Category") + value(issue.Category) + "\n")
	b.WriteString(label("Issue Type") + value(issue.IssueType) + "\n")
	b.WriteString(label("Rule ID") + value(ruleID) + "\n")
	b.WriteString(label("Tool") + value(issue.ToolName) + "\n")

	if issue.TechnicalDebtHours > 0 {
		b.WriteString(label("Debt Hours") + value(fmt.Sprintf("%.1fh", issue.TechnicalDebtHours)) + "\n")
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
		b.WriteString(accentStyle.Render("Description") + "\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorText).
			Width(wrapW+labelW).
			Render(*issue.Description) + "\n")
	}

	if issue.CodeSnippet != nil && *issue.CodeSnippet != "" {
		b.WriteString("\n")
		b.WriteString(accentStyle.Render("Code Snippet") + "\n")
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorFilePath).
			Render(*issue.CodeSnippet) + "\n")
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
