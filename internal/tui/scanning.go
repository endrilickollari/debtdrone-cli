package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/endrilickollari/debtdrone-cli/internal/service"
	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────────────────────
// Internal message types (package-private; never cross model boundaries)
// ─────────────────────────────────────────────────────────────────────────────

// scanProgressMsg is sent by the scan goroutine to report incremental progress.
type scanProgressMsg struct {
	Task     string
	Progress float64
}

// scanCompleteMsg is the final message from the scan goroutine.
type scanCompleteMsg struct {
	path   string
	issues []models.TechnicalDebtIssue
	err    error
}

// ─────────────────────────────────────────────────────────────────────────────
// Shared utilities
// ─────────────────────────────────────────────────────────────────────────────

// spinnerChars is the braille dot-spinner animation used by both ScanModel
// and UpdateModel.
var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// tickCmd fires a tickMsg every 100ms to drive spinner frames.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second/10, func(time.Time) tea.Msg { return tickMsg{} })
}

// startScan starts the analysis goroutine and returns immediately with nil
// so the Bubble Tea event loop stays responsive. Progress and completion
// events are sent to progressChan and later surfaced by listenForScanProgress.
func startScan(path string, maxComplexity int, securityScan bool, progressChan chan tea.Msg) tea.Cmd {
	log.SetOutput(io.Discard)
	return func() tea.Msg {
		go func() {
			svc := service.NewScanService()
			ctx := context.WithValue(context.Background(), "isCLI", true)
			opts := service.ScanOptions{
				MaxComplexity: maxComplexity,
				SecurityScan:  securityScan,
			}

			issues, err := svc.Run(ctx, path, opts, func(p service.ScanProgress) {
				progressChan <- scanProgressMsg{
					Task:     "Running " + p.AnalyzerName + "...",
					Progress: float64(p.Index) / float64(p.Total),
				}
				time.Sleep(300 * time.Millisecond)
			})

			log.SetOutput(os.Stderr)

			if err != nil {
				progressChan <- scanCompleteMsg{path: path, err: err}
				return
			}

			progressChan <- scanProgressMsg{Task: "Finalizing results...", Progress: 1.0}
			time.Sleep(500 * time.Millisecond)
			progressChan <- scanCompleteMsg{path: path, issues: issues}
		}()
		return nil
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// scanPhase tracks which sub-screen ScanModel is showing
// ─────────────────────────────────────────────────────────────────────────────

type scanPhase int

const (
	scanIdle    scanPhase = iota // not yet started
	scanRunning                  // goroutine active, progress bar visible
	scanResults                  // results (or error) ready to display
)

// ─────────────────────────────────────────────────────────────────────────────
// ScanModel
// ─────────────────────────────────────────────────────────────────────────────

// ScanModel handles two closely related screens that share state:
//   - stateScanning: animated progress bar while the goroutine runs.
//   - stateResults:  master-detail issue list after the scan (or on history replay).
//
// Navigation contract:
//   - When the scan goroutine finishes, ScanModel returns a command that
//     yields ScanFinishedMsg{Entry, Err}. AppModel intercepts it, adds the
//     run to the shared history list, and transitions to stateResults.
//   - When the user presses q/esc on the results screen, ScanModel returns a
//     command that yields NavigateMsg{State: stateMenu}. AppModel intercepts
//     and transitions.
//   - ScanModel never mutates AppModel state directly.
type ScanModel struct {
	phase        scanPhase
	scanPath     string
	scanTask     string
	scanProgress float64
	spinnerFrame int
	scanChan     chan tea.Msg
	outputFormat string // "text" | "json"; set by AppModel at scan start

	// Results state (populated after scan completes or LoadResults is called)
	err    error
	issues []models.TechnicalDebtIssue
	list   issueList
	detail issueViewport

	width, height int
}

func newScanModel() *ScanModel {
	return &ScanModel{
		phase:  scanIdle,
		width:  120,
		height: 40,
	}
}

// Start is called by AppModel when it intercepts StartScanMsg. It arms the
// model and returns the batch of commands needed to run the scan and keep the
// spinner alive.
func (m *ScanModel) Start(path string, maxComplexity int, securityScan bool, outputFormat string) tea.Cmd {
	m.phase = scanRunning
	m.scanPath = path
	m.scanTask = "Initializing scan..."
	m.scanProgress = 0
	m.spinnerFrame = 0
	m.outputFormat = outputFormat
	m.err = nil
	m.issues = nil
	m.scanChan = make(chan tea.Msg, 10) // fresh channel per scan

	return tea.Batch(
		startScan(path, maxComplexity, securityScan, m.scanChan),
		m.listenForScanProgress(),
		tickCmd(),
	)
}

// LoadResults is called by AppModel when it intercepts LoadHistoryRunMsg.
// It bypasses a live scan and hydrates the results pane from a past run.
func (m *ScanModel) LoadResults(entry historyEntry, outputFormat string) {
	m.phase = scanResults
	m.scanPath = entry.path
	m.outputFormat = outputFormat
	m.err = nil
	m.issues = entry.issues

	listH, detailH := splitHeight(m.height)
	m.list = newIssueList(entry.issues, m.width, listH)
	m.detail = issueViewport{height: detailH, width: m.width - 4}
	m.detail.setContent(formatIssueDetail(m.list.selected(), m.detail.width))
}

// listenForScanProgress returns a Cmd that blocks until the scan goroutine
// sends its next event, then surfaces that event as a Bubble Tea message so
// that Update can process it in the main goroutine.
func (m *ScanModel) listenForScanProgress() tea.Cmd {
	return func() tea.Msg { return <-m.scanChan }
}

// Init satisfies tea.Model.
func (m *ScanModel) Init() tea.Cmd { return nil }

// Update handles input and async events for both the scanning and results screens.
//
// Key message flows:
//
//	tea.WindowSizeMsg   → reflow list/viewport dimensions
//	tickMsg             → advance spinner frame while phase == scanRunning
//	scanProgressMsg     → update task label + progress bar; re-arm listener
//	scanCompleteMsg     → build results state; emit ScanFinishedMsg upward
//	tea.KeyPressMsg     → navigation keys on results screen; q/esc → NavigateMsg
func (m *ScanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.phase == scanResults {
			listH, detailH := splitHeight(m.height)
			m.list.height = listH
			m.list.width = m.width
			m.detail.height = detailH
			m.detail.width = m.width - 4
			if m.outputFormat != "json" {
				m.detail.setContent(formatIssueDetail(m.list.selected(), m.detail.width))
			}
		}
		return m, nil

	case tickMsg:
		if m.phase == scanRunning {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerChars)
			return m, tickCmd()
		}
		return m, nil

	case scanProgressMsg:
		m.scanTask = msg.Task
		m.scanProgress = msg.Progress
		// Re-arm the channel listener so the next event is delivered.
		return m, m.listenForScanProgress()

	case scanCompleteMsg:
		m.phase = scanResults

		if msg.err != nil {
			m.err = msg.err
			// Bubble a ScanFinishedMsg with the error so AppModel still
			// transitions to stateResults where the error is displayed.
			return m, func() tea.Msg {
				return ScanFinishedMsg{Err: msg.err}
			}
		}

		m.issues = msg.issues
		listH, detailH := splitHeight(m.height)
		m.list = newIssueList(msg.issues, m.width, listH)

		if m.outputFormat == "json" {
			jsonData, _ := json.MarshalIndent(msg.issues, "", "  ")
			m.detail = issueViewport{height: m.height - 4, width: m.width - 4}
			m.detail.setContent(string(jsonData))
		} else {
			m.detail = issueViewport{height: detailH, width: m.width - 4}
			m.detail.setContent(formatIssueDetail(m.list.selected(), m.detail.width))
		}

		// Build the history record for this scan.
		now := time.Now()
		run := models.AnalysisRun{
			ID:                  uuid.New(),
			StartedAt:           now,
			Status:              "completed",
			TotalIssuesFound:    len(msg.issues),
			CriticalIssuesCount: countBySeverity(msg.issues, "critical"),
			HighIssuesCount:     countBySeverity(msg.issues, "high"),
			MediumIssuesCount:   countBySeverity(msg.issues, "medium"),
			LowIssuesCount:      countBySeverity(msg.issues, "low"),
		}
		run.CompletedAt = &now
		run.RepositoryName = &msg.path

		entry := historyEntry{run: run, path: msg.path, issues: msg.issues}

		// Emit ScanFinishedMsg. AppModel intercepts it, adds the entry to
		// the shared history list, and transitions activeState to stateResults.
		return m, func() tea.Msg { return ScanFinishedMsg{Entry: entry} }

	case tea.KeyPressMsg:
		return m.handleKey(msg.String())
	}

	return m, nil
}

// handleKey processes keyboard input. The behaviour differs depending on the
// active phase and whether output format is JSON (scroll-only) vs text (list).
func (m *ScanModel) handleKey(str string) (tea.Model, tea.Cmd) {
	if m.phase == scanRunning {
		// During an active scan we only allow ctrl+c (handled by AppModel).
		return m, nil
	}

	// stateResults keys
	isJSON := m.outputFormat == "json"

	updateDetail := func() {
		if !isJSON {
			m.detail.setContent(formatIssueDetail(m.list.selected(), m.detail.width))
		}
	}

	switch str {
	case "q", "esc", "r":
		// Signal AppModel to navigate back to the menu.
		return m, func() tea.Msg { return NavigateMsg{State: stateMenu} }
	case "j", "down":
		if isJSON {
			m.detail.scrollDown(1)
		} else {
			m.list.moveDown()
			updateDetail()
		}
	case "k", "up":
		if isJSON {
			m.detail.scrollUp(1)
		} else {
			m.list.moveUp()
			updateDetail()
		}
	case "pgdn":
		m.list.pageDown()
		updateDetail()
	case "pgup":
		m.list.pageUp()
		updateDetail()
	case "g":
		m.list.goTop()
		updateDetail()
	case "G":
		m.list.goBottom()
		updateDetail()
	case "J":
		m.detail.scrollDown(3)
	case "K":
		m.detail.scrollUp(3)
	}
	return m, nil
}

// View satisfies tea.Model (used when ScanModel is exercised standalone in
// tests). AppModel calls render() directly.
func (m *ScanModel) View() tea.View {
	return tea.NewView(m.render())
}

// render produces the string for whichever sub-screen is active.
func (m *ScanModel) render() string {
	switch m.phase {
	case scanRunning:
		return m.renderScanning()
	case scanResults:
		return m.renderResults()
	default:
		return ""
	}
}

func (m *ScanModel) renderScanning() string {
	const boxWidth = 80
	spinner := spinnerChars[m.spinnerFrame]
	accentBlue := lipgloss.Color("#4fc3f7")
	dimColor := lipgloss.Color("#4a5068")
	pathColor := lipgloss.Color("#8899bb")
	progressColor := lipgloss.Color("#5af78e")

	const barWidth = 40
	completed := int(m.scanProgress * float64(barWidth))
	if completed > barWidth {
		completed = barWidth
	}
	bar := lipgloss.NewStyle().Foreground(progressColor).Render(strings.Repeat("█", completed)) +
		lipgloss.NewStyle().Foreground(dimColor).Render(strings.Repeat("░", barWidth-completed))
	percentage := fmt.Sprintf(" %3.0f%%", m.scanProgress*100)

	body := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Foreground(accentBlue).Bold(true).Render(spinner+" Analyzing Repository…"),
		"",
		lipgloss.NewStyle().Foreground(dimColor).Render("Task  ")+
			lipgloss.NewStyle().Foreground(colorText).Render(m.scanTask),
		lipgloss.NewStyle().Foreground(dimColor).Render("Path  ")+
			lipgloss.NewStyle().Foreground(pathColor).Render(truncate(m.scanPath, 60)),
		"",
		bar+lipgloss.NewStyle().Foreground(colorText).Bold(true).Render(percentage),
	)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentBlue).
		Padding(1, 4).
		Width(boxWidth).
		Background(lipgloss.Color("#1e2035")).
		Render(body)

	hint := lipgloss.NewStyle().Foreground(dimColor).Render("ctrl+c to cancel")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box+"\n\n"+hint)
}

func (m *ScanModel) renderResults() string {
	if m.err != nil {
		body := lipgloss.NewStyle().Foreground(colorError).Bold(true).Render("Scan failed") +
			"\n" + lipgloss.NewStyle().Foreground(colorDim).Render(m.err.Error())
		box := lipgloss.NewStyle().
			BorderLeft(true).BorderStyle(lipgloss.Border{Left: "│"}).BorderForeground(colorError).
			PaddingLeft(2).PaddingRight(2).PaddingTop(1).PaddingBottom(1).Width(150).
			Background(colorBg).Render(body)
		hint := lipgloss.NewStyle().Foreground(colorDim).Render("r  rescan    q  quit")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box+"\n\n"+hint)
	}

	if len(m.issues) == 0 {
		body := lipgloss.NewStyle().Foreground(colorOK).Bold(true).Render("No issues found — clean scan!")
		box := lipgloss.NewStyle().
			BorderLeft(true).BorderStyle(lipgloss.Border{Left: "│"}).BorderForeground(colorOK).
			PaddingLeft(2).PaddingRight(2).PaddingTop(1).PaddingBottom(1).Width(150).
			Background(colorBg).Render(body)
		hint := lipgloss.NewStyle().Foreground(colorDim).Render("r  rescan    q  quit")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box+"\n\n"+hint)
	}

	if m.outputFormat == "json" {
		return m.renderJSONResults()
	}
	return m.renderTextResults()
}

func (m *ScanModel) renderTextResults() string {
	listPane := m.list.view()

	const divTitle = " Issue Details "
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
		"j/k ↑↓ navigate   J/K scroll detail   g/G top/bottom   pgup/pgdn page   r rescan   q quit",
	)

	return lipgloss.JoinVertical(lipgloss.Left, listPane, divider, detailPane, hints)
}

func (m *ScanModel) renderJSONResults() string {
	const divTitle = " Raw JSON Results "
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
		"j/k or J/K scroll json   r rescan   q quit",
	)

	return lipgloss.JoinVertical(lipgloss.Left, divider, detailPane, hints)
}
