package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/service"
	"github.com/endrilickollari/debtdrone-cli/v2/scanner"
	"github.com/google/uuid"
)

// scanRunID identifies one scan attempt. Every message a scan emits carries the
// id of the run that produced it, so results from a superseded or cancelled
// scan are discarded instead of overwriting the run the reader is watching.
type scanRunID uint64

type scanProgressMsg struct {
	runID     scanRunID
	stage     string
	completed int
	total     int
}

type scanCompleteMsg struct {
	runID    scanRunID
	path     string
	issues   []models.TechnicalDebtIssue
	warnings []scanner.Warning
	err      error
}

// scanTickMsg animates the scanning view. It is distinct from the shared
// tickMsg so a superseded scan's animation cannot outlive its run or interfere
// with the self-update view's ticker.
type scanTickMsg struct{ runID scanRunID }

// scanMessageBuffer keeps the scanner from stalling on progress reporting
// between update-loop reads. Sends also abort on cancellation, so a cancelled
// scan can never leak a goroutine blocked on this channel.
const scanMessageBuffer = 32

type scanDisplayOptions struct {
	outputFormat    string
	showLineNumbers bool
	maxResults      int
}

var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second/10, func(time.Time) tea.Msg { return tickMsg{} })
}

func scanTickCmd(runID scanRunID) tea.Cmd {
	return tea.Tick(time.Second/10, func(time.Time) tea.Msg { return scanTickMsg{runID: runID} })
}

// listenForScanMessages waits for the next message from one scan. Both inputs
// are captured by value so a listener started for an earlier run can neither
// consume from its replacement nor remain blocked after its run is cancelled.
func listenForScanMessages(done <-chan struct{}, messages <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-messages:
			return msg
		case <-done:
			return nil
		}
	}
}

// startScan runs the scan off the update loop. The command returns immediately
// and the scan reports back through messages, so rendering and key handling
// stay responsive for the whole run. ctx cancellation both stops the scanner
// and releases this goroutine from any pending send.
func startScan(
	ctx context.Context,
	runID scanRunID,
	path string,
	options service.ScanOptions,
	historyEnabled bool,
	messages chan<- tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		go func() {
			send := func(msg tea.Msg) {
				select {
				case messages <- msg:
				case <-ctx.Done():
				}
			}

			svc := service.NewScanServiceWithHistoryEnabled(historyEnabled)
			result, err := svc.RunDetailed(ctx, path, options, func(p service.ScanProgress) {
				stage := ""
				if p.Started {
					stage = p.AnalyzerName
				}
				send(scanProgressMsg{
					runID:     runID,
					stage:     stage,
					completed: p.Completed,
					total:     p.Total,
				})
			})

			send(scanCompleteMsg{
				runID:    runID,
				path:     path,
				issues:   result.Issues,
				warnings: result.Warnings,
				err:      err,
			})
		}()
		return nil
	}
}

type scanPhase int

const (
	scanIdle scanPhase = iota
	scanRunning
	scanResults
)

// ScanModel manages the scan progress and results views.
type ScanModel struct {
	phase        scanPhase
	scanPath     string
	spinnerFrame int
	scanChan     chan tea.Msg
	scanDone     <-chan struct{}
	outputFormat string
	display      scanDisplayOptions

	// Active scan lifecycle. runID identifies the run whose messages the model
	// accepts; cancel stops it. Progress is reported as analyzers completed out
	// of the total rather than as a synthetic percentage.
	runID              scanRunID
	cancel             context.CancelFunc
	stage              string
	completedAnalyzers int
	totalAnalyzers     int
	startedAt          time.Time
	elapsed            time.Duration

	err     error
	warning error
	issues  []models.TechnicalDebtIssue
	list    issueList
	detail  issueViewport

	// Results workspace state. issues holds the render-bounded findings, summary
	// describes the complete scan, and list holds the filtered visible subset.
	filter      resultsFilter
	summary     resultsSummary
	categories  []string
	searching   bool
	searchDraft string
	status      string

	width, height int
}

// resultsChrome is the number of rows the results workspace spends on the
// summary band, status line, and search prompt before the panes are sized.
func (m *ScanModel) resultsChrome() int {
	rows := 3 // summary band: headline, severity counts, repository path
	if m.filter.active() || m.status != "" {
		rows++
	}
	if m.searching {
		rows++
	}
	return rows
}

// resizePanes recomputes the list and detail geometry for the current terminal.
func (m *ScanModel) resizePanes() {
	listH, detailH := splitHeightWithChrome(m.height, m.resultsChrome())
	m.list.height = listH
	m.list.width = m.width
	m.list.clampOffset()
	m.detail.height = detailH
	m.detail.width = m.width - 4
}

// applyFilters recomputes the visible findings and keeps the reader on the same
// finding when it survives the new filter. When it does not, the cursor holds
// its row position so the view does not jump back to the top.
func (m *ScanModel) applyFilters() {
	previous := ""
	if selected := m.list.selected(); selected != nil {
		previous = issueIdentity(*selected)
	}
	filtered := applyResultsFilter(m.issues, m.filter)
	m.list.setItems(filtered, indexOfIdentity(filtered, previous))
	m.refreshDetail()
}

func (m *ScanModel) refreshDetail() {
	if m.outputFormat == "json" {
		return
	}
	m.detail.setContent(formatIssueDetail(m.list.selected(), m.detail.width))
}

// resetResultsView loads the render-bounded findings into the workspace while
// retaining a summary calculated from the complete scan result.
func (m *ScanModel) resetResultsView(issues []models.TechnicalDebtIssue, summary resultsSummary) {
	m.issues = issues
	m.filter = resultsFilter{}
	m.searching = false
	m.searchDraft = ""
	m.status = ""
	m.summary = summary
	m.categories = issueCategories(issues)

	m.list = newIssueList(nil, m.width, 0)
	m.detail = issueViewport{}
	m.resizePanes()
	m.applyFilters()
}

func newScanModel() *ScanModel {
	return &ScanModel{
		phase:  scanIdle,
		width:  120,
		height: 40,
	}
}

// Start begins a new repository scan. Any scan still running is cancelled and
// superseded first, so a second start can neither race the active scan nor let
// its results overwrite the newer run.
func (m *ScanModel) Start(path string, options service.ScanOptions, display scanDisplayOptions, historyEnabled bool) tea.Cmd {
	m.Cancel()

	m.runID++
	runID := m.runID
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.scanDone = ctx.Done()

	m.phase = scanRunning
	m.scanPath = path
	m.stage = ""
	m.completedAnalyzers = 0
	m.totalAnalyzers = 0
	m.startedAt = time.Now()
	m.elapsed = 0
	m.spinnerFrame = 0
	m.outputFormat = display.outputFormat
	m.display = display
	m.err = nil
	m.warning = nil
	m.issues = nil

	messages := make(chan tea.Msg, scanMessageBuffer)
	m.scanChan = messages

	return tea.Batch(
		startScan(ctx, runID, path, options, historyEnabled, messages),
		listenForScanMessages(m.scanDone, messages),
		scanTickCmd(runID),
	)
}

// Cancel stops the active scan, if any, and releases its goroutine. Messages
// already in flight for that run are ignored because the run is superseded.
// Calling it when no scan is running is a no-op.
func (m *ScanModel) Cancel() {
	if m.cancel == nil {
		return
	}
	m.cancel()
	m.cancel = nil
	m.scanDone = nil
	// Advancing the run id retires every message the cancelled scan may still
	// deliver, including one that was already queued before cancellation.
	m.runID++
}

// Scanning reports whether a scan is currently running.
func (m *ScanModel) Scanning() bool {
	return m.phase == scanRunning
}

// LoadResults displays historical scan data.
func (m *ScanModel) LoadResults(entry historyEntry, outputFormat string) {
	m.phase = scanResults
	m.scanPath = entry.path
	m.outputFormat = outputFormat
	m.err = nil
	m.warning = nil
	summary := entry.summary
	if summary.severityCounts == nil {
		summary = summarizeIssues(entry.issues)
	}
	m.resetResultsView(entry.issues, summary)
	if m.outputFormat == "json" {
		m.setJSONResults(entry.issues)
	}
}

func (m *ScanModel) Init() tea.Cmd { return nil }

func (m *ScanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.phase == scanResults {
			if m.outputFormat == "json" {
				m.detail.height = m.height - 4
				m.detail.width = m.width - 4
			} else {
				m.resizePanes()
				m.refreshDetail()
			}
		}
		return m, nil

	case scanTickMsg:
		// A tick from a superseded run stops here rather than driving a second
		// animation loop alongside the current scan.
		if msg.runID != m.runID || m.phase != scanRunning {
			return m, nil
		}
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerChars)
		m.elapsed = time.Since(m.startedAt)
		return m, scanTickCmd(msg.runID)

	case scanProgressMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		if msg.stage != "" {
			m.stage = msg.stage
		}
		m.completedAnalyzers = msg.completed
		m.totalAnalyzers = msg.total
		return m, listenForScanMessages(m.scanDone, m.scanChan)

	case scanCompleteMsg:
		if msg.runID != m.runID {
			return m, nil
		}
		if m.cancel != nil {
			m.cancel()
		}
		m.cancel = nil
		m.scanDone = nil
		m.elapsed = time.Since(m.startedAt)
		m.phase = scanResults

		if msg.err != nil && len(msg.issues) == 0 {
			m.err = msg.err
			return m, func() tea.Msg {
				return ScanFinishedMsg{Err: msg.err}
			}
		}

		m.warning = msg.err
		for _, warning := range msg.warnings {
			m.warning = errors.Join(m.warning, fmt.Errorf("%s: %s", warning.AnalyzerID, warning.Message))
		}
		displayIssues := prepareDisplayIssues(msg.issues, m.display)
		fullSummary := summarizeIssues(msg.issues)
		m.resetResultsView(displayIssues, fullSummary)

		if m.outputFormat == "json" {
			m.setJSONResults(displayIssues)
		}

		now := time.Now()
		run := models.AnalysisRun{
			ID:                      uuid.New(),
			StartedAt:               now,
			Status:                  "completed",
			TotalIssuesFound:        len(msg.issues),
			CriticalIssuesCount:     countBySeverity(msg.issues, "critical"),
			HighIssuesCount:         countBySeverity(msg.issues, "high"),
			MediumIssuesCount:       countBySeverity(msg.issues, "medium"),
			LowIssuesCount:          countBySeverity(msg.issues, "low"),
			TotalTechnicalDebtHours: fullSummary.debtHours,
		}
		run.CompletedAt = &now
		run.RepositoryName = &msg.path

		entry := historyEntry{run: run, path: msg.path, issues: displayIssues, summary: fullSummary}
		return m, func() tea.Msg { return ScanFinishedMsg{Entry: entry} }

	case tea.KeyPressMsg:
		return m.handleKey(msg.String())
	}

	return m, nil
}

func (m *ScanModel) setJSONResults(issues []models.TechnicalDebtIssue) {
	jsonData, _ := json.MarshalIndent(issues, "", "  ")
	m.detail = issueViewport{height: m.height - 4, width: m.width - 4}
	m.detail.setContent(string(jsonData))
}

func prepareDisplayIssues(issues []models.TechnicalDebtIssue, options scanDisplayOptions) []models.TechnicalDebtIssue {
	limit := len(issues)
	if options.maxResults > 0 && options.maxResults < limit {
		limit = options.maxResults
	}
	result := append([]models.TechnicalDebtIssue(nil), issues[:limit]...)
	if !options.showLineNumbers {
		for index := range result {
			result[index].LineNumber = nil
			result[index].ColumnNumber = nil
		}
	}
	return result
}

// handleKey processes keyboard input. The behaviour differs depending on the
// active phase and whether output format is JSON (scroll-only) vs text (list).
func (m *ScanModel) handleKey(str string) (tea.Model, tea.Cmd) {
	if m.phase == scanRunning {
		// A running scan accepts cancellation only. Everything else is ignored
		// so a stray key cannot mutate results that do not exist yet.
		if str == "esc" || str == "q" {
			m.Cancel()
			m.phase = scanIdle
			return m, func() tea.Msg { return NavigateMsg{State: stateMenu} }
		}
		return m, nil
	}

	isJSON := m.outputFormat == "json"

	if m.searching {
		return m.handleSearchKey(str)
	}

	updateDetail := func() {
		if !isJSON {
			m.detail.setContent(formatIssueDetail(m.list.selected(), m.detail.width))
		}
	}

	// A transient status such as an export confirmation occupies the row that
	// otherwise reports the active filters, so moving on dismisses it and
	// returns that row to the filter summary.
	dismissStatus := func() {
		if m.status == "" {
			return
		}
		m.status = ""
		m.resizePanes()
	}

	// Filtering, sorting, and export operate on the findings table, which the
	// raw JSON view does not render.
	if !isJSON {
		switch str {
		case "/":
			m.searching = true
			m.searchDraft = m.filter.query
			m.status = ""
			m.resizePanes()
			return m, nil
		case "1", "2", "3", "4":
			severities := service.SeverityOrder()
			index := int(str[0] - '1')
			if index < len(severities) {
				m.filter.toggleSeverity(severities[index])
				m.status = ""
				m.resizePanes()
				m.applyFilters()
			}
			return m, nil
		case "c":
			m.filter.category = nextCategory(m.filter.category, m.categories)
			m.status = ""
			m.resizePanes()
			m.applyFilters()
			return m, nil
		case "s":
			m.filter.sort = m.filter.sort.next()
			m.status = fmt.Sprintf("Sorted by %s", m.filter.sort)
			m.resizePanes()
			m.applyFilters()
			return m, nil
		case "x":
			m.filter.clear()
			m.status = ""
			m.resizePanes()
			m.applyFilters()
			return m, nil
		case "e":
			m.status = m.exportVisibleFindings()
			m.resizePanes()
			return m, nil
		}
	}

	dismissStatus()

	switch str {
	case "q", "esc":
		return m, func() tea.Msg { return NavigateMsg{State: stateMenu} }
	case "r":
		return m, func() tea.Msg { return StartScanMsg{Path: m.scanPath} }
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

// handleSearchKey edits the search query. The findings table updates on every
// keystroke so the reader sees matches narrow as they type; Enter keeps the
// query and Esc restores the one that was active before searching began.
func (m *ScanModel) handleSearchKey(str string) (tea.Model, tea.Cmd) {
	switch str {
	case "enter":
		m.searching = false
		m.resizePanes()
		m.applyFilters()
	case "esc":
		m.searching = false
		m.filter.query = m.searchDraft
		m.resizePanes()
		m.applyFilters()
	case "backspace":
		runes := []rune(m.filter.query)
		if len(runes) > 0 {
			m.filter.query = string(runes[:len(runes)-1])
			m.applyFilters()
		}
	default:
		if isEditableChar(str) {
			m.filter.query += str
			m.applyFilters()
		}
	}
	return m, nil
}

// exportVisibleFindings writes the findings currently in view to a JSON file in
// the working directory and returns the status message to show the reader.
func (m *ScanModel) exportVisibleFindings() string {
	if len(m.list.items) == 0 {
		return "Nothing to export — no findings match the current filters."
	}

	payload, err := json.MarshalIndent(m.list.items, "", "  ")
	if err != nil {
		return "Export failed: " + err.Error()
	}

	name, err := writeFindingsExport(m.scanPath, time.Now().Format("20060102-150405"), payload)
	if err != nil {
		return "Export failed: " + err.Error()
	}

	absolute, absErr := filepath.Abs(name)
	if absErr != nil {
		absolute = name
	}
	return fmt.Sprintf("Exported %d findings to %s", len(m.list.items), absolute)
}

// writeFindingsExport creates a private file without replacing an existing path.
// A suffix keeps repeated exports from the same second distinct and O_EXCL also
// prevents a pre-existing symlink from redirecting the write.
func writeFindingsExport(repositoryPath, stamp string, payload []byte) (string, error) {
	const maximumAttempts = 1000
	base := strings.TrimSuffix(exportFileName(repositoryPath, stamp), ".json")
	for attempt := 0; attempt < maximumAttempts; attempt++ {
		name := base + ".json"
		if attempt > 0 {
			name = fmt.Sprintf("%s-%d.json", base, attempt+1)
		}

		file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}

		if _, err := file.Write(payload); err != nil {
			_ = file.Close()
			_ = os.Remove(name)
			return "", err
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(name)
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("could not create a unique export after %d attempts", maximumAttempts)
}

func (m *ScanModel) View() tea.View {
	return tea.NewView(m.render())
}

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
	boxWidth := min(max(m.width-2, 24), 80)
	innerWidth := max(boxWidth-8, 1) // four cells of padding per side
	spinner := spinnerChars[m.spinnerFrame]
	accentBlue := lipgloss.Color("#4fc3f7")
	dimColor := lipgloss.Color("#4a5068")
	pathColor := lipgloss.Color("#8899bb")
	progressColor := lipgloss.Color("#5af78e")

	stage := m.stage
	if stage == "" {
		stage = "Preparing analyzers"
	}

	rows := []string{
		lipgloss.NewStyle().Foreground(accentBlue).Bold(true).Render(spinner + " Analyzing Repository…"),
		"",
		lipgloss.NewStyle().Foreground(dimColor).Render("Stage    ") +
			lipgloss.NewStyle().Foreground(colorText).Render(stage),
		lipgloss.NewStyle().Foreground(dimColor).Render("Path     ") +
			lipgloss.NewStyle().Foreground(pathColor).Render(truncate(m.scanPath, max(innerWidth-9, 1))),
		lipgloss.NewStyle().Foreground(dimColor).Render("Elapsed  ") +
			lipgloss.NewStyle().Foreground(colorText).Render(formatElapsed(m.elapsed)),
	}

	// The bar counts analyzers that have actually finished. Until the scanner
	// reports a total there is nothing honest to draw, so the view shows the
	// stage and elapsed time alone rather than inventing a percentage.
	if m.totalAnalyzers > 0 {
		progressLabel := fmt.Sprintf("  %d/%d analyzers", m.completedAnalyzers, m.totalAnalyzers)
		barWidth := min(40, max(innerWidth-lipgloss.Width(progressLabel), 1))
		filled := clamp(m.completedAnalyzers*barWidth/m.totalAnalyzers, 0, barWidth)
		bar := lipgloss.NewStyle().Foreground(progressColor).Render(strings.Repeat("█", filled)) +
			lipgloss.NewStyle().Foreground(dimColor).Render(strings.Repeat("░", barWidth-filled))
		rows = append(rows, "", bar+lipgloss.NewStyle().Foreground(colorText).Bold(true).
			Render(progressLabel))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentBlue).
		Padding(1, 4).
		Width(boxWidth).
		Background(lipgloss.Color("#1e2035")).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))

	hintText := "esc  cancel and return to the dashboard        ctrl+c  quit"
	if m.width < lipgloss.Width(hintText) {
		hintText = "esc  cancel/back    ctrl+c  quit"
	}
	hint := lipgloss.NewStyle().Foreground(dimColor).Render(hintText)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box+"\n\n"+hint)
}

// formatElapsed renders a running duration at whole-second resolution, which is
// the precision the scan phase can honestly claim.
func formatElapsed(elapsed time.Duration) string {
	if elapsed < time.Second {
		return "0s"
	}
	return elapsed.Truncate(time.Second).String()
}

func (m *ScanModel) renderResults() string {
	if m.err != nil {
		// The failure state carries the inputs needed to retry or troubleshoot:
		// which repository was scanned, how long it ran before failing, and the
		// unabridged scanner error.
		body := lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Foreground(colorError).Bold(true).Render("Scan failed"),
			"",
			lipgloss.NewStyle().Foreground(colorDim).Render("Repository  ")+
				lipgloss.NewStyle().Foreground(colorFilePath).Render(truncate(m.scanPath, max(m.width-18, 10))),
			lipgloss.NewStyle().Foreground(colorDim).Render("Failed after  ")+
				lipgloss.NewStyle().Foreground(colorText).Render(formatElapsed(m.elapsed)),
			"",
			lipgloss.NewStyle().Foreground(colorText).Render(m.err.Error()),
		)
		boxWidth := min(max(m.width-1, 24), 100)
		box := lipgloss.NewStyle().
			BorderLeft(true).BorderStyle(lipgloss.Border{Left: "│"}).BorderForeground(colorError).
			PaddingLeft(2).PaddingRight(2).PaddingTop(1).PaddingBottom(1).Width(boxWidth).
			Background(colorBg).Render(body)
		hintText := "r  retry this scan        esc  back to the dashboard        ctrl+c  quit"
		if m.width < lipgloss.Width(hintText) {
			hintText = "r retry    esc dashboard    ctrl+c quit"
		}
		hint := lipgloss.NewStyle().Foreground(colorDim).Render(hintText)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box+"\n\n"+hint)
	}

	if len(m.issues) == 0 {
		body := lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Foreground(colorOK).Bold(true).Render("No issues found — clean scan!"),
			"",
			lipgloss.NewStyle().Foreground(colorDim).Render("Repository  ")+
				lipgloss.NewStyle().Foreground(colorFilePath).Render(truncate(m.scanPath, max(m.width-18, 10))),
			lipgloss.NewStyle().Foreground(colorDim).Render(
				"Every analyzer completed without reporting a finding above your configured thresholds."),
		)
		boxWidth := min(max(m.width-1, 24), 100)
		box := lipgloss.NewStyle().
			BorderLeft(true).BorderStyle(lipgloss.Border{Left: "│"}).BorderForeground(colorOK).
			PaddingLeft(2).PaddingRight(2).PaddingTop(1).PaddingBottom(1).Width(boxWidth).
			Background(colorBg).Render(body)
		hint := lipgloss.NewStyle().Foreground(colorDim).Render("r  rescan    q  quit")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box+"\n\n"+hint)
	}

	var results string
	if m.outputFormat == "json" {
		results = m.renderJSONResults()
	} else {
		results = m.renderTextResults()
	}
	if m.warning == nil {
		return results
	}
	warning := lipgloss.NewStyle().
		Foreground(colorHigh).
		Bold(true).
		Render("Partial scan: ") + lipgloss.NewStyle().Foreground(colorDim).Render(m.warning.Error())
	return lipgloss.JoinVertical(lipgloss.Left, warning, results)
}

// renderSummaryBand shows the headline numbers for the whole scan so the reader
// never has to navigate to learn how bad the result is.
func (m *ScanModel) renderSummaryBand() string {
	labelStyle := lipgloss.NewStyle().Foreground(colorDim)
	totalStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)

	counts := make([]string, 0, len(service.SeverityOrder()))
	for _, severity := range service.SeverityOrder() {
		count := m.summary.severityCounts[severity]
		style := lipgloss.NewStyle().Foreground(severityColor(severity)).Bold(true)
		if count == 0 {
			style = lipgloss.NewStyle().Foreground(colorDim)
		}
		counts = append(counts, style.Render(fmt.Sprintf("%s %d", strings.ToUpper(severity[:1])+severity[1:], count)))
	}

	headline := totalStyle.Render(fmt.Sprintf("%d findings", m.summary.total)) +
		labelStyle.Render(fmt.Sprintf("  across %d files  ·  %.1fh estimated debt", m.summary.filesAffected, m.summary.debtHours))

	path := labelStyle.Render("Repository  ") +
		lipgloss.NewStyle().Foreground(colorFilePath).Render(truncate(m.scanPath, max(m.width-16, 20)))

	return lipgloss.JoinVertical(lipgloss.Left,
		headline,
		strings.Join(counts, lipgloss.NewStyle().Foreground(colorDim).Render("   ")),
		path,
	)
}

// renderStatusLine reports the active filters, or a transient action result
// such as an export confirmation.
func (m *ScanModel) renderStatusLine() string {
	if m.status != "" {
		return lipgloss.NewStyle().Foreground(colorOK).Render(m.status)
	}
	description := filterDescription(m.filter, len(m.list.items), len(m.issues))
	if description == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(colorAccentBlue).Render("Filtered  ") +
		lipgloss.NewStyle().Foreground(colorText).Render(description) +
		lipgloss.NewStyle().Foreground(colorDim).Render("   x to clear")
}

// renderNoMatches replaces the table when filters exclude every finding. It
// states what is filtered and how to recover rather than showing a blank pane.
func (m *ScanModel) renderNoMatches() string {
	title := lipgloss.NewStyle().Foreground(colorHigh).Bold(true).
		Render("No findings match the current filters")
	detail := lipgloss.NewStyle().Foreground(colorText).
		Render(filterDescription(m.filter, 0, len(m.issues)))
	recovery := lipgloss.NewStyle().Foreground(colorDim).
		Render("x  clear all filters        /  edit the search        c  cycle category")
	return lipgloss.JoinVertical(lipgloss.Left, "", title, detail, "", recovery)
}

func (m *ScanModel) renderTextResults() string {
	sections := []string{m.renderSummaryBand()}
	if status := m.renderStatusLine(); status != "" {
		sections = append(sections, status)
	}
	if m.searching {
		cursor := lipgloss.NewStyle().Foreground(colorAccentBlue).Render("▌")
		sections = append(sections,
			lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true).Render("Search  ")+
				lipgloss.NewStyle().Foreground(colorText).Render(m.filter.query)+cursor+
				lipgloss.NewStyle().Foreground(colorDim).Render("   enter apply   esc cancel"))
	}

	if len(m.list.items) == 0 {
		sections = append(sections, m.renderNoMatches(),
			lipgloss.NewStyle().Foreground(colorDim).Render("\nr rescan   q quit"))
		return lipgloss.JoinVertical(lipgloss.Left, sections...)
	}

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

	hintText := "j/k navigate   J/K scroll detail   g/G top/bottom   /  search   1-4 severity   c category   " +
		"s sort: " + m.filter.sort.String() + "   x clear   e export   r rescan   q quit"
	if m.width < lipgloss.Width(hintText) {
		hintText = "j/k move   J/K detail   / search   1-4 severity   c category\n" +
			"s sort: " + m.filter.sort.String() + "   x clear   e export   r rescan   q quit"
	}
	hints := lipgloss.NewStyle().Foreground(colorDim).Render(hintText)

	sections = append(sections, listPane, divider, detailPane, hints)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
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
