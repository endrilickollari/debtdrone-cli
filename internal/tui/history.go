package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
)

// historyEntry bundles everything needed to re-display or replay a past scan.
type historyEntry struct {
	run    models.AnalysisRun
	path   string
	issues []models.TechnicalDebtIssue
}

// ─────────────────────────────────────────────────────────────────────────────
// HistoryModel
// ─────────────────────────────────────────────────────────────────────────────

// HistoryModel encapsulates the /history screen: a list of past scans on top
// and a detail viewport on the bottom.
//
// Navigation contract:
//   - esc/q → returns NavigateMsg{State: stateMenu}
//   - enter on a run → returns LoadHistoryRunMsg{Entry: …}
//     AppModel intercepts LoadHistoryRunMsg, hydrates ScanModel, and
//     transitions to stateResults without running a new scan.
type HistoryModel struct {
	entries []historyEntry
	cursor  int
	offset  int
	detail  issueViewport
	width   int
	height  int
}

func newHistoryModel() *HistoryModel {
	return &HistoryModel{width: 120, height: 40}
}

// SetEntries is called by AppModel whenever the history list changes (after a
// new scan completes) or when the user navigates to this screen. It resets the
// cursor so the most-recent run is always highlighted on entry.
func (m *HistoryModel) SetEntries(entries []historyEntry) {
	m.entries = entries
	m.cursor = 0
	m.offset = 0
	_, detailH := splitHeight(m.height)
	m.detail = issueViewport{height: detailH, width: m.width - 4}
	if len(entries) > 0 {
		m.detail.setContent(formatHistoryDetail(entries[0], m.detail.width))
	}
}

// Init satisfies tea.Model.
func (m *HistoryModel) Init() tea.Cmd { return nil }

// Update handles input for the history screen.
//
// Key message flows:
//
//	tea.WindowSizeMsg → resize the detail viewport; refresh its content
//	tea.KeyPressMsg   → j/k navigation, g/G jump, enter → LoadHistoryRunMsg,
//	                    q/esc → NavigateMsg
func (m *HistoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		_, detailH := splitHeight(m.height)
		m.detail.height = detailH
		m.detail.width = m.width - 4
		if len(m.entries) > 0 {
			m.detail.setContent(formatHistoryDetail(m.entries[m.cursor], m.detail.width))
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg.String())
	}
	return m, nil
}

func (m *HistoryModel) handleKey(str string) (tea.Model, tea.Cmd) {
	listH, _ := splitHeight(m.height)

	updateDetail := func() {
		if len(m.entries) > 0 {
			m.detail.setContent(formatHistoryDetail(m.entries[m.cursor], m.detail.width))
		}
	}

	switch str {
	case "q", "esc":
		return m, func() tea.Msg { return NavigateMsg{State: stateMenu} }

	case "j", "down":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			if m.cursor >= m.offset+listH {
				m.offset++
			}
			updateDetail()
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.offset {
				m.offset--
			}
			updateDetail()
		}
	case "g":
		m.cursor, m.offset = 0, 0
		updateDetail()
	case "G":
		if len(m.entries) > 0 {
			m.cursor = len(m.entries) - 1
			m.offset = max(0, m.cursor-listH+1)
			updateDetail()
		}
	case "J":
		m.detail.scrollDown(3)
	case "K":
		m.detail.scrollUp(3)

	case "enter":
		if len(m.entries) == 0 {
			break
		}
		entry := m.entries[m.cursor]
		// Emit LoadHistoryRunMsg. AppModel intercepts it, hydrates ScanModel
		// with the historical data, and transitions to stateResults.
		return m, func() tea.Msg { return LoadHistoryRunMsg{Entry: entry} }
	}
	return m, nil
}

// View satisfies tea.Model.
func (m *HistoryModel) View() tea.View {
	return tea.NewView(m.render())
}

// render produces the history screen string.
func (m *HistoryModel) render() string {
	listPane := m.renderList()

	const divTitle = " Past Scan Summary "
	innerW := max(m.width-len(divTitle)-4, 0)
	leftW := innerW / 2
	rightW := innerW - leftW
	divider := lipgloss.NewStyle().Foreground(colorAccentBlue).Render(
		strings.Repeat("─", leftW) +
			lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true).Render(divTitle) +
			strings.Repeat("─", rightW),
	)

	detailPane := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentBlue).
		Width(m.width - 2).
		Render(m.detail.view())

	hints := lipgloss.NewStyle().Foreground(colorDim).Render(
		"j/k ↑↓ navigate   J/K scroll detail   g/G top/bottom   enter browse results   q quit",
	)

	return lipgloss.JoinVertical(lipgloss.Left, listPane, divider, detailPane, hints)
}

func (m *HistoryModel) renderList() string {
	headerStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)

	const dateW = 19
	const totalW = 9
	const breakW = 28
	const gap = 2
	pathW := max(m.width-dateW-totalW-breakW-(gap*4)-2, 12)

	header := fmt.Sprintf("  %s  %s  %s  %s",
		headerStyle.Width(dateW).Render("Date / Time"),
		headerStyle.Width(pathW).Render("Scanned Path"),
		headerStyle.Width(totalW).Render("Issues"),
		headerStyle.Render("Breakdown"),
	)
	sep := dimStyle.Render(strings.Repeat("─", m.width))
	lines := []string{header, sep}

	listH, _ := splitHeight(m.height)
	end := min(m.offset+listH, len(m.entries))

	for i := m.offset; i < end; i++ {
		e := m.entries[i]
		run := e.run

		dateStr := lipgloss.NewStyle().Foreground(colorFilePath).Width(dateW).
			Render(run.StartedAt.Format("2006-01-02 15:04:05"))
		pathStr := lipgloss.NewStyle().Foreground(colorText).Width(pathW).
			Render(truncate(e.path, pathW-1))
		totalStr := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true).Width(totalW).
			Render(fmt.Sprintf("%d", run.TotalIssuesFound))

		breakdownStr := lipgloss.NewStyle().Foreground(colorCritical).Render(fmt.Sprintf("C:%-3d", run.CriticalIssuesCount)) +
			"  " + lipgloss.NewStyle().Foreground(colorHigh).Render(fmt.Sprintf("H:%-3d", run.HighIssuesCount)) +
			"  " + lipgloss.NewStyle().Foreground(colorMedium).Render(fmt.Sprintf("M:%-3d", run.MediumIssuesCount)) +
			"  " + lipgloss.NewStyle().Foreground(colorLow).Render(fmt.Sprintf("L:%-3d", run.LowIssuesCount))

		row := fmt.Sprintf("  %s  %s  %s  %s", dateStr, pathStr, totalStr, breakdownStr)
		if i == m.cursor {
			row = lipgloss.NewStyle().Background(colorSelectedBg).Foreground(colorAccentBlue).Width(m.width).Render(row)
		} else {
			row = lipgloss.NewStyle().Width(m.width).Render(row)
		}
		lines = append(lines, row)
	}

	if len(m.entries) > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  %d / %d", m.cursor+1, len(m.entries))))
	}

	return strings.Join(lines, "\n")
}
