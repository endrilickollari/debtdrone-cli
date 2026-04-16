package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
)

type issueList struct {
	items  []models.TechnicalDebtIssue
	cursor int
	offset int
	height int
	width  int
}

func newIssueList(issues []models.TechnicalDebtIssue, width, height int) issueList {
	return issueList{items: issues, width: width, height: height}
}

func (l *issueList) selected() *models.TechnicalDebtIssue {
	if len(l.items) == 0 || l.cursor >= len(l.items) {
		return nil
	}
	return &l.items[l.cursor]
}

func (l *issueList) moveDown() {
	if l.cursor < len(l.items)-1 {
		l.cursor++
		if l.cursor >= l.offset+l.height {
			l.offset++
		}
	}
}

func (l *issueList) moveUp() {
	if l.cursor > 0 {
		l.cursor--
		if l.cursor < l.offset {
			l.offset--
		}
	}
}

func (l *issueList) pageDown() {
	l.cursor = min(l.cursor+l.height, len(l.items)-1)
	if l.cursor >= l.offset+l.height {
		l.offset = l.cursor - l.height + 1
	}
}

func (l *issueList) pageUp() {
	l.cursor = max(l.cursor-l.height, 0)
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
}

func (l *issueList) goTop() {
	l.cursor = 0
	l.offset = 0
}

func (l *issueList) goBottom() {
	if len(l.items) == 0 {
		return
	}
	l.cursor = len(l.items) - 1
	l.offset = max(0, l.cursor-l.height+1)
}

func (l issueList) view() string {
	headerStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)

	const sevW = 10
	const fileW = 38
	const gap = 2
	msgW := max(l.width-sevW-fileW-(gap*4)-2, 8)

	hSev := headerStyle.Width(sevW).Render("Severity")
	hFile := headerStyle.Width(fileW).Render("File")
	hMsg := headerStyle.Render("Message")
	header := fmt.Sprintf("  %s  %s  %s", hSev, hFile, hMsg)
	sep := dimStyle.Render(strings.Repeat("─", l.width))

	lines := []string{header, sep}

	end := min(l.offset+l.height, len(l.items))
	for i := l.offset; i < end; i++ {
		issue := l.items[i]
		sev := issue.Severity
		if sev == "" {
			sev = "low"
		}

		sevStr := lipgloss.NewStyle().
			Foreground(severityColor(sev)).
			Bold(true).
			Width(sevW).
			Render(sev)

		base := filepath.Base(issue.FilePath)
		if issue.LineNumber != nil {
			base = fmt.Sprintf("%s:%d", base, *issue.LineNumber)
		}
		fileStr := lipgloss.NewStyle().
			Foreground(colorFilePath).
			Width(fileW).
			Render(truncate(base, fileW-1))

		msgStr := lipgloss.NewStyle().
			Foreground(colorText).
			Render(truncate(issue.Message, msgW))

		row := fmt.Sprintf("  %s  %s  %s", sevStr, fileStr, msgStr)

		if i == l.cursor {
			row = lipgloss.NewStyle().
				Background(colorSelectedBg).
				Width(l.width).
				Render(row)
		} else {
			row = lipgloss.NewStyle().Width(l.width).Render(row)
		}
		lines = append(lines, row)
	}

	if len(l.items) > 0 {
		counter := fmt.Sprintf("  %d / %d", l.cursor+1, len(l.items))
		lines = append(lines, dimStyle.Render(counter))
	}

	return strings.Join(lines, "\n")
}

type issueViewport struct {
	lines  []string
	offset int
	height int
	width  int
}

func (v *issueViewport) setContent(content string) {
	v.lines = strings.Split(content, "\n")
	v.offset = 0
}

func (v *issueViewport) scrollDown(n int) {
	maxOffset := max(0, len(v.lines)-v.height)
	v.offset = min(v.offset+n, maxOffset)
}

func (v *issueViewport) scrollUp(n int) {
	v.offset = max(0, v.offset-n)
}

func (v issueViewport) view() string {
	if len(v.lines) == 0 {
		return strings.Repeat("\n", v.height)
	}
	end := min(v.offset+v.height, len(v.lines))
	visible := make([]string, 0, v.height)
	visible = append(visible, v.lines[v.offset:end]...)
	for len(visible) < v.height {
		visible = append(visible, "")
	}
	return strings.Join(visible, "\n")
}
