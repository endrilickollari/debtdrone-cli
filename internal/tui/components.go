package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
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

// setItems replaces the visible findings and moves the cursor to the requested
// row. Callers pass the row holding the previously selected finding so that
// changing a filter keeps the reader on the same issue where possible; a
// negative index clamps to the nearest valid row instead.
func (l *issueList) setItems(items []models.TechnicalDebtIssue, cursor int) {
	l.items = items
	if cursor < 0 {
		cursor = l.cursor
	}
	l.cursor = clamp(cursor, 0, max(len(items)-1, 0))
	l.clampOffset()
}

// clampOffset keeps the scroll window inside the item range and guarantees the
// cursor stays visible after the list length or viewport height changes.
func (l *issueList) clampOffset() {
	if l.height <= 0 {
		l.offset = 0
		return
	}
	l.offset = clamp(l.offset, 0, max(len(l.items)-l.height, 0))
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+l.height {
		l.offset = l.cursor - l.height + 1
	}
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
	const catW = 16
	const gap = 2
	// Every row opens with a focus marker so the selected finding is identifiable
	// from text alone, without relying on the highlight colour.
	const markerW = 2

	// The file column gives back width on narrow terminals so the message stays
	// readable rather than being cut to a few characters.
	fileW := 38
	if layoutFor(l.width, 0).compact() {
		fileW = 24
	}

	// The category column is only worth its width once the message column can
	// still show a useful amount of text alongside it.
	showCategory := l.width >= markerW+sevW+fileW+catW+40
	fixedW := markerW + sevW + fileW + (gap * 3)
	if showCategory {
		fixedW += catW + gap
	}
	msgW := max(l.width-fixedW, 8)

	header := fmt.Sprintf("%s%s  %s",
		strings.Repeat(" ", markerW),
		headerStyle.Width(sevW).Render("Severity"),
		headerStyle.Width(fileW).Render("File"))
	if showCategory {
		header += "  " + headerStyle.Width(catW).Render("Category")
	}
	header += "  " + headerStyle.Render("Message")
	sep := dimStyle.Render(strings.Repeat("─", l.width))

	lines := []string{header, sep}

	end := min(l.offset+l.height, len(l.items))
	for i := l.offset; i < end; i++ {
		issue := l.items[i]
		sev := issue.Severity
		if sev == "" {
			sev = "low"
		}

		selected := i == l.cursor

		// Each cell carries the selection background itself. A style that only
		// wrapped the finished row would be cancelled by the reset sequence
		// every inner style emits, leaving the highlight painted on the
		// padding alone.
		cell := func(colour lipgloss.Color) lipgloss.Style {
			style := lipgloss.NewStyle().Foreground(colour)
			if selected {
				style = style.Background(colorSelectedBg)
			}
			return style
		}

		marker := "  "
		if selected {
			marker = "› "
		}

		sevStr := cell(severityColor(sev)).Bold(true).Width(sevW).Render(sev)

		base := filepath.Base(issue.FilePath)
		if issue.LineNumber != nil {
			base = fmt.Sprintf("%s:%d", base, *issue.LineNumber)
		}
		fileStr := cell(colorFilePath).Width(fileW).Render(truncate(base, fileW-1))
		msgStr := cell(colorText).Render(truncate(issue.Message, msgW))

		row := cell(colorAccentBlue).Bold(true).Render(marker) + sevStr + cell(colorText).Render("  ") + fileStr
		if showCategory {
			category := issue.Category
			if category == "" {
				category = "—"
			}
			row += cell(colorText).Render("  ") + cell(colorDim).Width(catW).Render(truncate(category, catW-1))
		}
		row += cell(colorText).Render("  ") + msgStr

		rowStyle := lipgloss.NewStyle().Width(l.width)
		if selected {
			rowStyle = rowStyle.Background(colorSelectedBg)
		}
		row = rowStyle.Render(row)
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
