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
	err                string

	focus          int
	recent         []localhistory.Record
	recentDetail   *localhistory.Record
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
	m.recentDetail = nil
	m.err = ""
	m.clampFocus()
}

func (m *MenuModel) ShowHelp() {
	m.showingHelp = true
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
			if str == "esc" || str == "q" {
				m.showingHelp = false
				return m, func() tea.Msg { return NavigateMsg{State: stateMenu} }
			}
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
	case "s":
		return m.startCurrentScan()
	case "h":
		m.recentDetail = nil
		return m, func() tea.Msg { return NavigateMsg{State: stateHistory} }
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
	contentWidth := min(max(m.width-8, 68), 118)
	titleStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	repository := filepath.Base(m.currentPath)
	if repository == "." || repository == string(filepath.Separator) {
		repository = m.currentPath
	}
	header := titleStyle.Render("DEBTDRONE") + "  " + dimStyle.Render("LOCAL SCANNER  /  "+repository)
	subtitle := lipgloss.NewStyle().Foreground(colorText).
		Render("Choose an action. Shortcuts accelerate the workflow; they are never required.")

	actionWidth := 42
	if contentWidth < 90 {
		actionWidth = 32
	}
	recentWidth := contentWidth - actionWidth - 3
	actions := m.renderActions(actionWidth)
	recent := m.renderRecentScans(max(recentWidth, 33))
	var body string
	if recentWidth >= 33 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, actions, "   ", recent)
	} else {
		body = lipgloss.JoinVertical(lipgloss.Left, actions, "", m.renderRecentScans(contentWidth))
	}

	hintKey := lipgloss.NewStyle().Foreground(colorText).Bold(true)
	hintText := lipgloss.NewStyle().Foreground(colorDim)
	hints := hintKey.Render("↑/↓") + hintText.Render(" move   ") +
		hintKey.Render("enter") + hintText.Render(" open   ") +
		hintKey.Render("/") + hintText.Render(" commands   ") +
		hintKey.Render("?") + hintText.Render(" help   ") +
		hintKey.Render("q") + hintText.Render(" quit")

	var content strings.Builder
	content.WriteString(header)
	content.WriteString("\n")
	content.WriteString(subtitle)
	content.WriteString("\n\n")
	content.WriteString(body)
	if m.err != "" {
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Foreground(colorError).Render("! " + m.err))
	}
	content.WriteString("\n\n")
	content.WriteString(hints)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content.String())
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
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccentBlue).
		Padding(1).Width(width).Render(strings.TrimSuffix(body.String(), "\n"))
}

func (m *MenuModel) renderRecentScans(width int) string {
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
			body.WriteString(m.renderRecentRow(record, innerWidth, selected))
			if index < len(m.recent)-1 {
				body.WriteString("\n")
			}
		}
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccentBlue).
		Padding(1).Width(width).Render(body.String())
}

func (m *MenuModel) renderRecentRow(record localhistory.Record, width int, selected bool) string {
	statusColor := colorOK
	if record.Outcome == localhistory.OutcomePartial {
		statusColor = colorHigh
	}
	status := lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render(strings.ToUpper(string(record.Outcome)))
	repository := truncate(record.Repository, max(width-lipgloss.Width(status)-3, 10))
	gap := max(width-lipgloss.Width(repository)-lipgloss.Width(status)-1, 1)
	first := repository + strings.Repeat(" ", gap) + status
	second := record.CompletedAt.UTC().Format("Jan 02 15:04 UTC") + fmt.Sprintf(" · %d findings", record.Summary.Findings)
	third := fmt.Sprintf("C:%d  H:%d  M:%d  L:%d", record.Summary.Critical, record.Summary.High, record.Summary.Medium, record.Summary.Low)
	row := first + "\n" + lipgloss.NewStyle().Foreground(colorDim).Render(second) + "\n" +
		lipgloss.NewStyle().Foreground(colorFilePath).Render(third)
	if selected {
		return lipgloss.NewStyle().Background(colorSelectedBg).Foreground(colorText).PaddingLeft(1).Width(width).Render(row)
	}
	return lipgloss.NewStyle().PaddingLeft(1).Width(width).Render(row)
}

func (m *MenuModel) renderCommandPalette() string {
	boxWidth := min(max(m.width-8, 60), 100)
	innerWidth := max(boxWidth-6, 24)
	headerStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	input := lipgloss.NewStyle().Foreground(lipgloss.Color("#89ddff")).Render(m.renderInputLine())
	inputBox := lipgloss.NewStyle().BorderLeft(true).
		BorderStyle(lipgloss.Border{Left: "│"}).BorderForeground(colorAccentBlue).
		Padding(1, 2).Width(innerWidth).Background(colorBg).Render(input)

	var body strings.Builder
	body.WriteString(headerStyle.Render("COMMAND PALETTE"))
	body.WriteString("\n")
	body.WriteString(dimStyle.Render("Type a command or choose a repository path."))
	body.WriteString("\n\n")
	body.WriteString(inputBox)
	if len(m.suggestions) > 0 {
		body.WriteString("\n")
		for index, suggestion := range m.suggestions {
			annotation := m.suggestionAnnotation(suggestion)
			row := "  " + suggestion
			if annotation != "" {
				row += "  " + annotation
			}
			style := lipgloss.NewStyle().Foreground(colorFilePath).Width(innerWidth)
			if index == m.selectedSuggestion {
				style = style.Foreground(colorAccentBlue).Background(colorSelectedBg).Bold(true)
			}
			body.WriteString(style.Render(row))
			body.WriteString("\n")
		}
	}
	body.WriteString("\n")
	body.WriteString(dimStyle.Render("tab / ↑↓ choose   → accept   enter run   esc dashboard"))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccentBlue).
		Padding(1, 2).Width(boxWidth).Background(colorBg).Render(body.String())
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
	boxWidth := min(max(m.width-8, 64), 86)
	labelStyle := lipgloss.NewStyle().Foreground(colorDim).Bold(true).Width(18)
	valueStyle := lipgloss.NewStyle().Foreground(colorText)
	titleStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	statusColor := colorOK
	if record.Outcome == localhistory.OutcomePartial {
		statusColor = colorHigh
	}
	line := func(label, value string) string {
		return labelStyle.Render(label) + valueStyle.Render(value)
	}
	severity := fmt.Sprintf("%d / %d / %d / %d", record.Summary.Critical, record.Summary.High, record.Summary.Medium, record.Summary.Low)

	var body strings.Builder
	body.WriteString(titleStyle.Render("RECENT SCAN SUMMARY"))
	body.WriteString("\n")
	body.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("Reopened from privacy-safe local history"))
	body.WriteString("\n\n")
	body.WriteString(line("Repository", record.Repository) + "\n")
	body.WriteString(labelStyle.Render("Status") + lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render(strings.ToUpper(string(record.Outcome))) + "\n")
	body.WriteString(line("Completed", record.CompletedAt.UTC().Format(time.RFC3339)) + "\n")
	body.WriteString(line("Findings", fmt.Sprintf("%d", record.Summary.Findings)) + "\n")
	body.WriteString(line("C / H / M / L", severity) + "\n")
	body.WriteString(line("Debt estimate", fmt.Sprintf("%.2f hours", record.Summary.TechnicalDebtHours)) + "\n")
	body.WriteString(line("Warnings", fmt.Sprintf("%d", record.Summary.Warnings)) + "\n")
	body.WriteString(line("Analyzer failures", fmt.Sprintf("%d", record.Summary.AnalyzerFailures)) + "\n")
	body.WriteString("\n")
	body.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render("Only summary metadata is stored; source and finding details are never persisted."))
	body.WriteString("\n\n")
	body.WriteString(lipgloss.NewStyle().Foreground(colorText).Bold(true).Render("esc / enter"))
	body.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render(" dashboard   "))
	body.WriteString(lipgloss.NewStyle().Foreground(colorText).Bold(true).Render("s"))
	body.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render(" scan current   "))
	body.WriteString(lipgloss.NewStyle().Foreground(colorText).Bold(true).Render("h"))
	body.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render(" session history"))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccentBlue).
		Padding(1, 2).Width(boxWidth).Background(colorBg).Render(body.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *MenuModel) renderHelp() string {
	boxWidth := min(max(m.width-8, 68), 100)
	headerStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true).Width(18)
	descStyle := lipgloss.NewStyle().Foreground(colorText)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	var rows strings.Builder
	rows.WriteString(headerStyle.Render("DASHBOARD SHORTCUTS"))
	rows.WriteString("\n\n")
	for _, action := range dashboardActions {
		rows.WriteString(keyStyle.Render(action.shortcut))
		rows.WriteString(descStyle.Render(action.label))
		rows.WriteString("\n")
	}
	rows.WriteString(keyStyle.Render("r"))
	rows.WriteString(descStyle.Render("Reopen the newest local scan summary"))
	rows.WriteString("\n")
	rows.WriteString(keyStyle.Render("↑/↓ or j/k"))
	rows.WriteString(descStyle.Render("Move focus; Enter opens the selected row"))
	rows.WriteString("\n")
	rows.WriteString(keyStyle.Render("/"))
	rows.WriteString(descStyle.Render("Open the command palette"))
	rows.WriteString("\n\n")
	rows.WriteString(headerStyle.Render("COMMAND PALETTE"))
	rows.WriteString("\n\n")
	for _, command := range allCommands {
		rows.WriteString(keyStyle.Render(command.cmd))
		rows.WriteString(descStyle.Render(command.desc))
		rows.WriteString("\n")
	}
	rows.WriteString("\n")
	rows.WriteString(dimStyle.Render("q / esc  back to dashboard"))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccentBlue).
		Padding(1, 2).Width(boxWidth).Background(colorBg).Render(rows.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
