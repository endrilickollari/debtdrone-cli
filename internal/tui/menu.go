package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localhistory"
)

const dashboardRecentLimit = 3

type recentHistoryLoader func(context.Context) ([]localhistory.Record, error)

type recentHistoryLoadedMsg struct {
	records []localhistory.Record
	err     error
}

type dashboardActionKind int

const (
	dashboardScanCurrent dashboardActionKind = iota
	dashboardChoosePath
	dashboardHistory
	dashboardSettings
	dashboardHelp
	dashboardQuit
)

type dashboardAction struct {
	kind        dashboardActionKind
	label       string
	description string
	shortcut    string
}

var dashboardActions = []dashboardAction{
	{dashboardScanCurrent, "Scan current directory", "Analyze this repository now", "s"},
	{dashboardChoosePath, "Choose repository path", "Scan another local directory", "p"},
	{dashboardHistory, "Session scan history", "Browse full findings kept this session", "h"},
	{dashboardSettings, "Settings", "Adjust scan and display behavior", "c"},
	{dashboardHelp, "Keyboard help", "See every command and shortcut", "?"},
	{dashboardQuit, "Quit DebtDrone", "Return to your shell", "q"},
}

// MenuModel manages the dashboard, command palette, and help overlay.
type MenuModel struct {
	input              string
	cursorPos          int
	suggestions        []string
	selectedSuggestion int
	pathComplete       bool
	inputActive        bool
	showingHelp        bool
	helpOffset         int
	err                string

	focus          int
	recent         []localhistory.Record
	recentDetail   *localhistory.Record
	recentOffset   int
	historyLoading bool
	historyErr     string
	currentPath    string
	loadHistory    recentHistoryLoader

	width, height int
}

func newMenuModel() *MenuModel {
	currentPath, err := os.Getwd()
	if err != nil {
		currentPath = "."
	}
	return newMenuModelWithHistory(currentPath, loadRecentHistory)
}

func newMenuModelWithHistory(currentPath string, loadHistory recentHistoryLoader) *MenuModel {
	if strings.TrimSpace(currentPath) == "" {
		currentPath = "."
	}
	return &MenuModel{
		selectedSuggestion: -1,
		currentPath:        filepath.Clean(currentPath),
		loadHistory:        loadHistory,
		width:              120,
		height:             40,
	}
}

func loadRecentHistory(ctx context.Context) ([]localhistory.Record, error) {
	path, err := localhistory.DefaultPath()
	if err != nil {
		return nil, err
	}
	store, err := localhistory.New(path)
	if err != nil {
		return nil, err
	}
	return store.List(ctx)
}

// Reset clears transient overlays while preserving the dashboard focus. A user
// returning home lands on the action or recent scan they previously selected.
func (m *MenuModel) Reset() {
	m.input = ""
	m.cursorPos = 0
	m.suggestions = nil
	m.selectedSuggestion = -1
	m.pathComplete = false
	m.inputActive = false
	m.showingHelp = false
	m.helpOffset = 0
	m.recentDetail = nil
	m.recentOffset = 0
	m.err = ""
	m.clampFocus()
}

func (m *MenuModel) ShowHelp() {
	m.showingHelp = true
	m.helpOffset = 0
}

func (m *MenuModel) Init() tea.Cmd {
	return m.RefreshHistory()
}

func (m *MenuModel) RefreshHistory() tea.Cmd {
	if m.loadHistory == nil {
		m.historyLoading = false
		return nil
	}
	m.historyLoading = true
	load := m.loadHistory
	return func() tea.Msg {
		records, err := load(context.Background())
		return recentHistoryLoadedMsg{records: records, err: err}
	}
}

func (m *MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case recentHistoryLoadedMsg:
		m.historyLoading = false
		if msg.err != nil {
			m.historyErr = msg.err.Error()
			m.recent = nil
			m.clampFocus()
			return m, nil
		}
		m.historyErr = ""
		limit := min(len(msg.records), dashboardRecentLimit)
		m.recent = append([]localhistory.Record(nil), msg.records[:limit]...)
		m.clampFocus()
		return m, nil
	case tea.KeyPressMsg:
		str := msg.String()
		if m.showingHelp {
			switch str {
			case "esc", "q":
				m.showingHelp = false
				return m, func() tea.Msg { return NavigateMsg{State: stateMenu} }
			case "j", "down":
				m.helpOffset++
			case "k", "up":
				m.helpOffset--
			case "pgdown":
				m.helpOffset += m.overlayViewportHeight()
			case "pgup":
				m.helpOffset -= m.overlayViewportHeight()
			case "g", "home":
				m.helpOffset = 0
			case "G", "end":
				m.helpOffset = len(m.helpLines(max(m.width-6, 1)))
			}
			m.helpOffset = max(m.helpOffset, 0)
			return m, nil
		}
		if m.recentDetail != nil {
			return m.handleRecentDetailKey(str)
		}
		if m.inputActive {
			return m.handleInputKey(str)
		}
		return m.handleDashboardKey(str)
	}
	return m, nil
}

func (m *MenuModel) handleDashboardKey(str string) (tea.Model, tea.Cmd) {
	for index, action := range dashboardActions {
		if str == action.shortcut {
			m.focus = index
			return m.activateFocusedItem()
		}
	}

	switch str {
	case "j", "down", "tab":
		if m.focus < m.dashboardItemCount()-1 {
			m.focus++
		}
	case "k", "up", "shift+tab":
		if m.focus > 0 {
			m.focus--
		}
	case "g", "home":
		m.focus = 0
	case "G", "end":
		m.focus = m.dashboardItemCount() - 1
	case "enter":
		return m.activateFocusedItem()
	case "r":
		if len(m.recent) > 0 {
			m.focus = len(dashboardActions)
			m.openRecent(0)
		}
	case "/":
		m.openCommandInput()
	}
	return m, nil
}

func (m *MenuModel) handleRecentDetailKey(str string) (tea.Model, tea.Cmd) {
	switch str {
	case "esc", "q", "enter":
		m.recentDetail = nil
		m.recentOffset = 0
	case "s":
		return m.startCurrentScan()
	case "h":
		m.recentDetail = nil
		m.recentOffset = 0
		return m, func() tea.Msg { return NavigateMsg{State: stateHistory} }
	case "j", "down":
		m.recentOffset++
	case "k", "up":
		m.recentOffset = max(m.recentOffset-1, 0)
	case "pgdown":
		m.recentOffset += m.overlayViewportHeight()
	case "pgup":
		m.recentOffset = max(m.recentOffset-m.overlayViewportHeight(), 0)
	case "g", "home":
		m.recentOffset = 0
	case "G", "end":
		m.recentOffset = 1 << 30
	}
	return m, nil
}

func (m *MenuModel) handleInputKey(str string) (tea.Model, tea.Cmd) {
	switch str {
	case "esc":
		m.closeInput()
	case "enter":
		if m.selectedSuggestion >= 0 && len(m.suggestions) > 0 {
			m.acceptSuggestion()
			return m, nil
		}
		return m.handleCommand()
	case "tab", "down":
		if len(m.suggestions) > 0 {
			m.selectedSuggestion = (m.selectedSuggestion + 1) % len(m.suggestions)
		}
	case "shift+tab", "up":
		if len(m.suggestions) > 0 {
			m.selectedSuggestion--
			if m.selectedSuggestion < 0 {
				m.selectedSuggestion = len(m.suggestions) - 1
			}
		}
	case "right":
		if m.cursorPos < len(m.input) {
			m.cursorPos++
		} else if len(m.suggestions) > 0 {
			m.acceptSuggestion()
		}
	case "left":
		if m.cursorPos > 0 {
			m.cursorPos--
		}
	case "backspace":
		if m.cursorPos > 0 {
			m.input = m.input[:m.cursorPos-1] + m.input[m.cursorPos:]
			m.cursorPos--
			m.computeSuggestions()
		}
	default:
		if len(str) == 1 {
			m.input = m.input[:m.cursorPos] + str + m.input[m.cursorPos:]
			m.cursorPos++
			m.computeSuggestions()
		}
	}
	return m, nil
}

func (m *MenuModel) activateFocusedItem() (tea.Model, tea.Cmd) {
	if m.focus >= len(dashboardActions) {
		m.openRecent(m.focus - len(dashboardActions))
		return m, nil
	}
	switch dashboardActions[m.focus].kind {
	case dashboardScanCurrent:
		return m.startCurrentScan()
	case dashboardChoosePath:
		m.openPathInput()
	case dashboardHistory:
		return m, func() tea.Msg { return NavigateMsg{State: stateHistory} }
	case dashboardSettings:
		return m, func() tea.Msg { return NavigateMsg{State: stateConfig} }
	case dashboardHelp:
		return m, func() tea.Msg { return NavigateMsg{State: stateHelp} }
	case dashboardQuit:
		return m, tea.Quit
	}
	return m, nil
}

func (m *MenuModel) startCurrentScan() (tea.Model, tea.Cmd) {
	path, err := filepath.Abs(m.currentPath)
	if err != nil {
		m.err = fmt.Sprintf("Could not resolve the current directory: %v", err)
		return m, nil
	}
	return m, func() tea.Msg { return StartScanMsg{Path: path} }
}

func (m *MenuModel) openRecent(index int) {
	if index < 0 || index >= len(m.recent) {
		return
	}
	record := m.recent[index]
	m.recentDetail = &record
	m.recentOffset = 0
}

func (m *MenuModel) openPathInput() {
	m.inputActive = true
	m.input = "/scan "
	m.cursorPos = len(m.input)
	m.computeSuggestions()
}

func (m *MenuModel) openCommandInput() {
	m.inputActive = true
	m.input = "/"
	m.cursorPos = len(m.input)
	m.computeSuggestions()
}

func (m *MenuModel) closeInput() {
	m.input = ""
	m.cursorPos = 0
	m.suggestions = nil
	m.selectedSuggestion = -1
	m.pathComplete = false
	m.inputActive = false
}

// handleCommand processes the entered command.
func (m *MenuModel) handleCommand() (tea.Model, tea.Cmd) {
	cmd := strings.TrimSpace(m.input)
	m.closeInput()
	m.err = ""
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return m, nil
	}
	switch strings.ToLower(parts[0]) {
	case "/scan":
		path := "."
		if len(parts) > 1 {
			path = parts[1]
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			m.err = fmt.Sprintf("Could not resolve that path: %v", err)
			return m, nil
		}
		return m, func() tea.Msg { return StartScanMsg{Path: absPath} }
	case "/update":
		return m, func() tea.Msg { return StartUpdateMsg{} }
	case "/history":
		return m, func() tea.Msg { return NavigateMsg{State: stateHistory} }
	case "/config":
		return m, func() tea.Msg { return NavigateMsg{State: stateConfig} }
	case "/help", "/h", "?":
		return m, func() tea.Msg { return NavigateMsg{State: stateHelp} }
	case "/quit", "/q", "exit":
		return m, tea.Quit
	default:
		m.err = fmt.Sprintf("Unknown command %q. Press ? to see available actions.", parts[0])
		return m, nil
	}
}

func (m *MenuModel) dashboardItemCount() int {
	return len(dashboardActions) + len(m.recent)
}

func (m *MenuModel) clampFocus() {
	last := m.dashboardItemCount() - 1
	if m.focus > last {
		if len(m.recent) == 0 {
			m.focus = 0
		} else {
			m.focus = last
		}
	}
	if m.focus < 0 {
		m.focus = 0
	}
}

func (m *MenuModel) computeSuggestions() {
	m.selectedSuggestion = -1
	if strings.HasPrefix(strings.ToLower(m.input), "/scan ") {
		m.pathComplete = true
		pathPrefix := m.input[len("/scan "):]
		m.suggestions = pathSuggestions(pathPrefix)
		return
	}
	m.pathComplete = false
	if m.input == "" {
		m.suggestions = nil
		return
	}
	prefix := strings.ToLower(strings.Fields(m.input)[0])
	var matches []string
	for _, command := range allCommands {
		if strings.HasPrefix(command.cmd, prefix) && command.cmd != prefix {
			matches = append(matches, command.cmd)
		}
	}
	m.suggestions = matches
}

func (m *MenuModel) acceptSuggestion() {
	if len(m.suggestions) == 0 {
		return
	}
	index := m.selectedSuggestion
	if index < 0 || index >= len(m.suggestions) {
		index = 0
	}
	chosen := m.suggestions[index]
	if chosen == "" {
		return
	}
	if m.pathComplete {
		m.input = "/scan " + chosen
		m.cursorPos = len(m.input)
		m.computeSuggestions()
		return
	}
	m.input = chosen + " "
	m.cursorPos = len(m.input)
	m.suggestions = nil
	m.selectedSuggestion = -1
}

// pathSuggestions provides directory completions for the /scan command.
func pathSuggestions(prefix string) []string {
	const maxSuggestions = 10
	if prefix != "" && !filepath.IsAbs(prefix) &&
		!strings.HasPrefix(prefix, "./") &&
		!strings.HasPrefix(prefix, "../") &&
		prefix != "." && prefix != ".." {
		prefix = "./" + prefix
	}
	var dirToList, nameFilter string
	switch {
	case prefix == "":
		dirToList, nameFilter = ".", ""
	case strings.HasSuffix(prefix, "/"):
		dirToList = filepath.Clean(prefix)
		nameFilter = ""
	default:
		dirToList = filepath.Dir(prefix)
		nameFilter = filepath.Base(prefix)
	}
	entries, err := os.ReadDir(dirToList)
	if err != nil {
		return nil
	}
	var suggestions []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(nameFilter, ".") {
			continue
		}
		if nameFilter != "" && !strings.HasPrefix(name, nameFilter) {
			continue
		}
		joined := filepath.Join(dirToList, name)
		var full string
		switch {
		case filepath.IsAbs(joined):
			full = joined
		case strings.HasPrefix(joined, ".."):
			full = joined
		default:
			full = "./" + joined
		}
		suggestions = append(suggestions, full+"/")
		if len(suggestions) == maxSuggestions {
			break
		}
	}
	return suggestions
}

func (m *MenuModel) renderInputLine() string {
	if m.cursorPos >= len(m.input) {
		return m.input + "█"
	}
	return m.input[:m.cursorPos] + "█" + m.input[m.cursorPos:]
}

func (m *MenuModel) renderInputLineWithin(width int) string {
	line := m.renderInputLine()
	if lipgloss.Width(line) <= width {
		return line
	}
	cursor := clamp(m.cursorPos, 0, len(m.input))
	if lipgloss.Width(m.input[:cursor]) >= width-1 {
		return truncateLeft(m.input[:cursor]+"█", width)
	}
	return truncate(line, width)
}

func (m *MenuModel) View() tea.View {
	return tea.NewView(m.render())
}

func (m *MenuModel) render() string {
	switch {
	case m.recentDetail != nil:
		return m.renderRecentDetail()
	case m.inputActive:
		return m.renderCommandPalette()
	default:
		return m.renderDashboard()
	}
}

func (m *MenuModel) renderDashboard() string {
	view := layoutFor(m.width, m.height)
	contentWidth := min(max(m.width-8, 1), 118)
	if view.compact() {
		contentWidth = max(m.width, 1)
	}
	titleStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	repository := filepath.Base(m.currentPath)
	if repository == "." || repository == string(filepath.Separator) {
		repository = m.currentPath
	}
	brand := titleStyle.Render("DEBTDRONE")
	contextWidth := max(contentWidth-lipgloss.Width("DEBTDRONE  "), 1)
	context := truncate("LOCAL SCANNER  /  "+repository, contextWidth)
	header := brand + "  " + dimStyle.Render(context)
	subtitleText := "Choose an action. Shortcuts accelerate the workflow; they are never required."
	if lipgloss.Width(subtitleText) > contentWidth {
		subtitleText = "Choose an action. Every workflow is available without shortcuts."
	}
	// The shorter wording can still exceed a very narrow terminal.
	subtitle := lipgloss.NewStyle().Foreground(colorText).Render(truncate(subtitleText, contentWidth))

	// Below the compact breakpoint the two panels stack. Side by side they would
	// each be squeezed until the recent-scan rows wrapped mid-line, which reads
	// as broken rather than dense.
	var body string
	if view.compact() {
		actions := m.renderActions(contentWidth - 2)
		// Stacking costs vertical space, so the recent panel switches to
		// single-line rows when the two panels would not otherwise fit. Every
		// scan stays listed, which keeps focus order intact.
		const surroundingRows = 5 // header, subtitle, blank, blank, hints
		remaining := m.height - lipgloss.Height(actions) - surroundingRows
		recent := m.renderRecentScans(contentWidth-2, false)
		if lipgloss.Height(recent) > remaining {
			recent = m.renderRecentScans(contentWidth-2, true)
		}
		body = lipgloss.JoinVertical(lipgloss.Left, actions, "", recent)
	} else {
		const actionWidth = 42
		const gapWidth = 3
		// Width excludes each panel's border, so reserve two cells per panel in
		// addition to the visible gap between them.
		recentWidth := contentWidth - actionWidth - gapWidth - 4
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderActions(actionWidth),
			strings.Repeat(" ", gapWidth),
			m.renderRecentScans(recentWidth, false))
	}

	hintKey := lipgloss.NewStyle().Foreground(colorText).Bold(true)
	hintText := lipgloss.NewStyle().Foreground(colorDim)
	hints := hintKey.Render("↑/↓") + hintText.Render(" move   ") +
		hintKey.Render("enter") + hintText.Render(" open   ") +
		hintKey.Render("/") + hintText.Render(" commands   ") +
		hintKey.Render("?") + hintText.Render(" help   ") +
		hintKey.Render("q") + hintText.Render(" quit")

	compose := func(withSubtitle bool, panels string) string {
		var content strings.Builder
		content.WriteString(header)
		content.WriteString("\n")
		if withSubtitle {
			content.WriteString(subtitle)
			content.WriteString("\n")
		}
		content.WriteString("\n")
		content.WriteString(panels)
		if m.err != "" {
			content.WriteString("\n")
			content.WriteString(lipgloss.NewStyle().Foreground(colorError).
				Render(truncate("! "+m.err, contentWidth)))
		}
		content.WriteString("\n\n")
		content.WriteString(hints)
		return content.String()
	}

	// A short terminal gives up explanatory chrome first. If both panels cannot
	// fit, it shows the panel containing the focused row; moving across the panel
	// boundary swaps the visible panel, so no selectable item becomes invisible.
	actionsOnly := m.renderActions(contentWidth - 2)
	if !view.compact() {
		actionsOnly = m.renderActions(42)
	}
	focusedPanel := actionsOnly
	if m.focus >= len(dashboardActions) && len(m.recent) > 0 {
		focusedPanel = m.renderRecentScans(contentWidth-2, true)
	}
	for _, candidate := range []string{
		compose(true, body),
		compose(false, body),
		compose(false, focusedPanel),
	} {
		if lipgloss.Height(candidate) <= m.height {
			return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, candidate)
		}
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, compose(false, focusedPanel))
}

func (m *MenuModel) renderActions(width int) string {
	innerWidth := max(width-4, 24)
	header := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true).Render("PRIMARY ACTIONS")
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	keyStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	var body strings.Builder
	body.WriteString(header)
	body.WriteString("\n\n")
	for index, action := range dashboardActions {
		selected := m.focus == index
		prefix := "  "
		if selected {
			prefix = "› "
		}
		key := keyStyle.Render("[" + action.shortcut + "]")
		gap := max(innerWidth-lipgloss.Width(prefix+action.label)-lipgloss.Width(key), 1)
		row := prefix + action.label + strings.Repeat(" ", gap) + key
		if selected {
			row = lipgloss.NewStyle().Foreground(colorText).Background(colorSelectedBg).Bold(true).
				Width(innerWidth).Render(row)
		} else {
			row = lipgloss.NewStyle().Foreground(colorText).Width(innerWidth).Render(row)
		}
		body.WriteString(row)
		body.WriteString("\n")
		if selected {
			body.WriteString(dimStyle.PaddingLeft(2).Render(action.description))
			body.WriteString("\n")
		}
	}
	return panelStyle(m.width, width).Render(strings.TrimSuffix(body.String(), "\n"))
}

// panelStyle frames a dashboard panel. Vertical padding is dropped on a narrow
// terminal, where the two stacked panels need every row they can get.
func panelStyle(terminalWidth, width int) lipgloss.Style {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentBlue).Width(width)
	if layoutFor(terminalWidth, 0).compact() {
		return style.Padding(0, 1)
	}
	return style.Padding(1)
}

// renderRecentScans draws the recent-scan panel. When dense, each scan takes a
// single line instead of three, which is what lets the stacked layout fit a
// short terminal without dropping scans or breaking focus order.
func (m *MenuModel) renderRecentScans(width int, dense bool) string {
	innerWidth := max(width-4, 28)
	headerStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	var body strings.Builder
	body.WriteString(headerStyle.Render("RECENT SCANS"))
	if len(m.recent) > 0 {
		shortcut := dimStyle.Render("[r] reopen newest")
		gap := max(innerWidth-lipgloss.Width("RECENT SCANS")-lipgloss.Width(shortcut), 1)
		body.WriteString(strings.Repeat(" ", gap))
		body.WriteString(shortcut)
	}
	body.WriteString("\n\n")
	switch {
	case m.historyLoading && len(m.recent) == 0:
		body.WriteString(dimStyle.Render("Loading local scan summaries…"))
	case m.historyErr != "":
		body.WriteString(lipgloss.NewStyle().Foreground(colorError).Render("Recent scans unavailable"))
		body.WriteString("\n")
		body.WriteString(dimStyle.Render(truncate(m.historyErr, innerWidth)))
	case len(m.recent) == 0:
		body.WriteString(lipgloss.NewStyle().Foreground(colorText).Render("No scan summaries yet."))
		body.WriteString("\n")
		body.WriteString(dimStyle.Render("Choose Scan current directory to create the first one."))
	default:
		for index, record := range m.recent {
			selected := m.focus == len(dashboardActions)+index
			body.WriteString(m.renderRecentRow(record, innerWidth, selected, dense))
			if index < len(m.recent)-1 {
				body.WriteString("\n")
			}
		}
	}
	return panelStyle(m.width, width).Render(body.String())
}

func (m *MenuModel) renderRecentRow(record localhistory.Record, width int, selected, dense bool) string {
	statusColor := colorOK
	if record.Outcome == localhistory.OutcomePartial {
		statusColor = colorHigh
	}
	// A marker keeps the focused row identifiable when the highlight colour is
	// unavailable, and each line is bounded to the panel so none of them wrap.
	marker := "  "
	if selected {
		marker = "› "
	}
	available := max(width-lipgloss.Width(marker)-1, 12)

	status := lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render(strings.ToUpper(string(record.Outcome)))
	repository := truncate(record.Repository, max(available-lipgloss.Width(status)-2, 8))
	gap := max(available-lipgloss.Width(repository)-lipgloss.Width(status), 1)
	first := repository + strings.Repeat(" ", gap) + status

	second := truncate(record.CompletedAt.UTC().Format("Jan 02 15:04 UTC")+
		fmt.Sprintf(" · %d findings", record.Summary.Findings), available)
	third := truncate(fmt.Sprintf("C:%d  H:%d  M:%d  L:%d",
		record.Summary.Critical, record.Summary.High, record.Summary.Medium, record.Summary.Low), available)

	markerStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	var row string
	if dense {
		// Repository, outcome and finding count on one line; the severity
		// breakdown is the detail a short terminal gives up first.
		summary := truncate(fmt.Sprintf("%s · %d findings", record.Repository, record.Summary.Findings),
			max(available-lipgloss.Width(status)-2, 8))
		gap := max(available-lipgloss.Width(summary)-lipgloss.Width(status), 1)
		row = markerStyle.Render(marker) + summary + strings.Repeat(" ", gap) + status
	} else {
		row = markerStyle.Render(marker) + first + "\n" +
			strings.Repeat(" ", lipgloss.Width(marker)) + lipgloss.NewStyle().Foreground(colorDim).Render(second) + "\n" +
			strings.Repeat(" ", lipgloss.Width(marker)) + lipgloss.NewStyle().Foreground(colorFilePath).Render(third)
	}

	if selected {
		return lipgloss.NewStyle().Background(colorSelectedBg).Foreground(colorText).Width(width).Render(row)
	}
	return lipgloss.NewStyle().Width(width).Render(row)
}

func (m *MenuModel) renderCommandPalette() string {
	boxWidth := min(100, max(m.width-2, 20))
	innerWidth := max(boxWidth-6, 1)
	headerStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	input := lipgloss.NewStyle().Foreground(lipgloss.Color("#89ddff")).
		Render(m.renderInputLineWithin(max(innerWidth-4, 1)))
	inputBox := lipgloss.NewStyle().BorderLeft(true).
		BorderStyle(lipgloss.Border{Left: "│"}).BorderForeground(colorAccentBlue).
		Padding(1, 2).Width(innerWidth).Background(colorBg).Render(input)

	sections := []string{
		headerStyle.Render(truncate("COMMAND PALETTE", innerWidth)),
		dimStyle.Render(truncate("Type a command or choose a repository path.", innerWidth)),
		"",
		inputBox,
	}
	if len(m.suggestions) > 0 {
		rows := make([]string, 0, len(m.suggestions))
		for index, suggestion := range m.suggestions {
			annotation := m.suggestionAnnotation(suggestion)
			marker := "  "
			if index == m.selectedSuggestion {
				marker = "› "
			}
			row := marker + suggestion
			if annotation != "" {
				row += "  " + annotation
			}
			row = truncate(row, innerWidth)
			style := lipgloss.NewStyle().Foreground(colorFilePath).Width(innerWidth)
			if index == m.selectedSuggestion {
				style = style.Foreground(colorAccentBlue).Background(colorSelectedBg).Bold(true)
			}
			rows = append(rows, style.Render(row))
		}
		focus := max(m.selectedSuggestion, 0)
		visible := windowLines(rows, max(m.height-12, 1), focus)
		sections = append(sections, strings.Join(visible, "\n"))
	}
	sections = append(sections, "", dimStyle.Render(truncate(
		"tab/↑↓ choose   → accept   enter run   esc back", innerWidth)))
	body := lipgloss.JoinVertical(lipgloss.Left, sections...)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccentBlue).
		Padding(1, 2).Width(boxWidth).Background(colorBg).Render(body)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *MenuModel) suggestionAnnotation(suggestion string) string {
	if m.pathComplete {
		return "directory"
	}
	for _, command := range allCommands {
		if command.cmd == suggestion {
			return command.desc
		}
	}
	return ""
}

func (m *MenuModel) renderRecentDetail() string {
	record := *m.recentDetail
	boxWidth := min(86, max(m.width-2, 20))
	innerWidth := max(boxWidth-4, 1)
	labelWidth := min(18, max(innerWidth/3, 10))
	valueWidth := max(innerWidth-labelWidth, 1)
	labelStyle := lipgloss.NewStyle().Foreground(colorDim).Bold(true).Width(labelWidth)
	titleStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	statusColor := colorOK
	if record.Outcome == localhistory.OutcomePartial {
		statusColor = colorHigh
	}
	line := func(label, value string) string {
		return labelStyle.Render(label) + hangingValue(value, labelWidth, valueWidth)
	}
	severity := fmt.Sprintf("%d / %d / %d / %d", record.Summary.Critical, record.Summary.High, record.Summary.Medium, record.Summary.Low)

	content := []string{
		titleStyle.Render(truncate("RECENT SCAN SUMMARY", innerWidth)),
		lipgloss.NewStyle().Foreground(colorDim).Render(truncate("Reopened from privacy-safe local history", innerWidth)),
		"",
		line("Repository", record.Repository),
		labelStyle.Render("Status") + lipgloss.NewStyle().Foreground(statusColor).Bold(true).
			Render(strings.ToUpper(string(record.Outcome))),
		line("Completed", record.CompletedAt.UTC().Format(time.RFC3339)),
		line("Findings", fmt.Sprintf("%d", record.Summary.Findings)),
		line("C / H / M / L", severity),
		line("Debt estimate", fmt.Sprintf("%.2f hours", record.Summary.TechnicalDebtHours)),
		line("Warnings", fmt.Sprintf("%d", record.Summary.Warnings)),
		line("Analyzer failures", fmt.Sprintf("%d", record.Summary.AnalyzerFailures)),
		"",
	}
	privacy := lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth).
		Render("Only summary metadata is stored; source and finding details are never persisted.")
	content = append(content, strings.Split(privacy, "\n")...)
	content = flattenRenderedLines(content)
	viewportHeight := m.overlayViewportHeight()
	maxOffset := max(len(content)-viewportHeight, 0)
	m.recentOffset = clamp(m.recentOffset, 0, maxOffset)
	visible := windowLinesAt(content, viewportHeight, m.recentOffset)
	footer := lipgloss.NewStyle().Foreground(colorDim).Render(truncate(
		"j/k scroll   s scan   h history   esc/enter dashboard", innerWidth))
	body := lipgloss.JoinVertical(lipgloss.Left, strings.Join(visible, "\n"), footer)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccentBlue).
		Padding(1, 2).Width(boxWidth).Background(colorBg).Render(body)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *MenuModel) renderHelp() string {
	boxWidth := min(100, max(m.width-2, 20))
	innerWidth := max(boxWidth-4, 1)
	helpLines := m.helpLines(innerWidth)
	viewportHeight := m.overlayViewportHeight()
	maxOffset := max(len(helpLines)-viewportHeight, 0)
	m.helpOffset = clamp(m.helpOffset, 0, maxOffset)
	visible := windowLinesAt(helpLines, viewportHeight, m.helpOffset)
	footerText := fmt.Sprintf("%d-%d/%d   j/k scroll   g/G ends   q/esc back",
		min(m.helpOffset+1, len(helpLines)), min(m.helpOffset+len(visible), len(helpLines)), len(helpLines))
	footer := lipgloss.NewStyle().Foreground(colorDim).Render(truncate(footerText, innerWidth))
	body := lipgloss.JoinVertical(lipgloss.Left, strings.Join(visible, "\n"), footer)

	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccentBlue).
		Padding(1, 2).Width(boxWidth).Background(colorBg).Render(body)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *MenuModel) helpLines(innerWidth int) []string {
	headerStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	keyWidth := min(18, max(innerWidth/3, 8))
	descriptionWidth := max(innerWidth-keyWidth, 1)
	keyStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true).Width(keyWidth)
	descStyle := lipgloss.NewStyle().Foreground(colorText)
	row := func(key, description string) string {
		return keyStyle.Render(truncate(key, keyWidth)) + descStyle.Render(truncate(description, descriptionWidth))
	}
	rows := []string{headerStyle.Render(truncate("DASHBOARD SHORTCUTS", innerWidth)), ""}
	for _, action := range dashboardActions {
		rows = append(rows, row(action.shortcut, action.label))
	}
	rows = append(rows,
		row("r", "Reopen the newest local scan summary"),
		row("↑/↓ or j/k", "Move focus; Enter opens the selected row"),
		row("/", "Open the command palette"),
		"",
		headerStyle.Render(truncate("COMMAND PALETTE", innerWidth)),
		"",
	)
	for _, command := range allCommands {
		rows = append(rows, row(command.cmd, command.desc))
	}
	return rows
}

func (m *MenuModel) overlayViewportHeight() int {
	const chromeAndFooter = 5 // border, vertical padding, and pinned footer
	return max(m.height-chromeAndFooter, 1)
}

func flattenRenderedLines(groups []string) []string {
	var lines []string
	for _, group := range groups {
		lines = append(lines, strings.Split(group, "\n")...)
	}
	return lines
}
