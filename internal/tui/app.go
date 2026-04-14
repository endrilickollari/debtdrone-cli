package tui

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/internal/analysis"
	"github.com/endrilickollari/debtdrone-cli/internal/analysis/analyzers"
	"github.com/endrilickollari/debtdrone-cli/internal/git"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/endrilickollari/debtdrone-cli/internal/store/memory"
	"github.com/endrilickollari/debtdrone-cli/internal/update"
	"github.com/google/uuid"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type state int

const (
	stateMenu state = iota
	stateScanning
	stateResults
	stateUpdating
	stateHelp
)

var allCommands = []struct {
	cmd  string
	desc string
}{
	{"/scan", "Analyze repository for technical debt"},
	{"/update", "Check for and install updates"},
	{"/history", "View past scan results"},
	{"/config", "View or edit configuration"},
	{"/help", "Show available commands"},
	{"/quit", "Exit the application"},
}

type model struct {
	state              state
	input              string
	cursorPos          int
	issues             []models.TechnicalDebtIssue
	scanPath           string
	err                error
	scanning           bool
	selectedIssue      int
	scrollOffset       int
	updateInfo         *update.UpdateInfo
	mu                 sync.Mutex
	spinnerFrame       int
	width              int
	height             int
	suggestions        []string
	selectedSuggestion int
}

func initialModel() *model {
	return &model{
		state:              stateMenu,
		input:              "",
		cursorPos:          0,
		scanning:           false,
		scrollOffset:       0,
		spinnerFrame:       0,
		width:              120,
		height:             40,
		selectedSuggestion: -1,
	}
}

type tickMsg struct{}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tickMsg:
		if m.state == stateScanning {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerChars)
			return m, tea.Tick(time.Second/10, func(t time.Time) tea.Msg { return tickMsg{} })
		}
	case scanCompleteMsg:
		m.mu.Lock()
		m.scanning = false
		if msg.err != nil {
			m.err = msg.err
			m.state = stateMenu
		} else {
			m.issues = msg.issues
			m.state = stateResults
			m.selectedIssue = 0
			m.scrollOffset = 0
		}
		m.mu.Unlock()
		return m, nil
	case checkUpdateMsg:
		m.mu.Lock()
		m.scanning = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.updateInfo = msg.info
		}
		m.state = stateUpdating
		m.mu.Unlock()
		return m, nil
	}

	return m, nil
}

func (m *model) computeSuggestions() {
	if m.input == "" {
		m.suggestions = nil
		m.selectedSuggestion = -1
		return
	}
	prefix := strings.ToLower(strings.Fields(m.input)[0])
	var matches []string
	for _, c := range allCommands {
		if strings.HasPrefix(c.cmd, prefix) && c.cmd != prefix {
			matches = append(matches, c.cmd)
		}
	}
	m.suggestions = matches
	if m.selectedSuggestion >= len(matches) {
		m.selectedSuggestion = -1
	}
}

func (m *model) acceptSuggestion() {
	var chosen string
	if m.selectedSuggestion >= 0 && m.selectedSuggestion < len(m.suggestions) {
		chosen = m.suggestions[m.selectedSuggestion]
	} else if len(m.suggestions) > 0 {
		chosen = m.suggestions[0]
	}
	if chosen == "" {
		return
	}
	m.input = chosen + " "
	m.cursorPos = len(m.input)
	m.suggestions = nil
	m.selectedSuggestion = -1
}

func (m *model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	str := msg.String()

	switch m.state {
	case stateMenu:
		switch str {
		case "enter":
			if m.selectedSuggestion >= 0 && len(m.suggestions) > 0 {
				m.acceptSuggestion()
				return m, nil
			}
			return m.handleCommand()
		case "tab":
			if len(m.suggestions) > 0 {
				m.selectedSuggestion = (m.selectedSuggestion + 1) % len(m.suggestions)
			}
		case "shift+tab":
			if len(m.suggestions) > 0 {
				m.selectedSuggestion--
				if m.selectedSuggestion < 0 {
					m.selectedSuggestion = len(m.suggestions) - 1
				}
			}
		case "down":
			if len(m.suggestions) > 0 {
				m.selectedSuggestion = (m.selectedSuggestion + 1) % len(m.suggestions)
			}
		case "up":
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
		case "ctrl+c":
			fmt.Println("👋 Goodbye!")
			os.Exit(0)
		default:
			if len(str) == 1 {
				m.input = m.input[:m.cursorPos] + str + m.input[m.cursorPos:]
				m.cursorPos++
				m.computeSuggestions()
			}
		}
		return m, nil

	case stateScanning:
		if str == "ctrl+c" {
			fmt.Println("👋 Goodbye!")
			os.Exit(0)
		}
		return m, nil

	case stateResults:
		switch str {
		case "q", "esc":
			m.state = stateMenu
			m.input = ""
			m.cursorPos = 0
			m.selectedIssue = 0
			m.scrollOffset = 0
		case "j", "down":
			m.scrollDown(1)
		case "k", "up":
			m.scrollUp(1)
		case "pgdn":
			m.scrollDown(20)
		case "pgup":
			m.scrollUp(20)
		case "g":
			m.scrollTop()
		case "G":
			m.scrollBottom()
		case "r":
			m.state = stateMenu
			m.input = ""
			m.cursorPos = 0
		}
		return m, nil

	case stateUpdating:
		if str == "y" {
			m.state = stateScanning
			return m, performUpdateCmd
		} else if str == "n" {
			m.state = stateMenu
			m.updateInfo = nil
		}
		return m, nil

	case stateHelp:
		if str == "esc" || str == "q" {
			m.state = stateMenu
		}
		return m, nil
	}

	return m, nil
}

func (m *model) scrollDown(lines int) {
	if len(m.issues) == 0 {
		return
	}
	maxVisible := 100
	newOffset := m.scrollOffset + lines
	if newOffset+maxVisible > len(m.issues) {
		newOffset = len(m.issues) - maxVisible
	}
	if newOffset < 0 {
		newOffset = 0
	}
	m.scrollOffset = newOffset
}

func (m *model) scrollUp(lines int) {
	if len(m.issues) == 0 {
		return
	}
	newOffset := m.scrollOffset - lines
	if newOffset < 0 {
		newOffset = 0
	}
	m.scrollOffset = newOffset
}

func (m *model) scrollTop() {
	m.scrollOffset = 0
}

func (m *model) scrollBottom() {
	if len(m.issues) == 0 {
		return
	}
	maxVisible := 100
	m.scrollOffset = len(m.issues) - maxVisible
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *model) handleCommand() (tea.Model, tea.Cmd) {
	cmd := strings.TrimSpace(m.input)
	m.input = ""
	m.cursorPos = 0
	m.suggestions = nil
	m.selectedSuggestion = -1

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return m, nil
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "/scan":
		path := "."
		if len(parts) > 1 {
			path = parts[1]
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			m.err = fmt.Errorf("failed to resolve path: %w", err)
			return m, nil
		}
		m.scanPath = absPath
		m.scanning = true
		m.state = stateScanning
		return m, startScan(absPath)

	case "/update":
		m.state = stateUpdating
		m.scanning = true
		return m, startUpdateCheck

	case "/history":
		return m, nil

	case "/config":
		return m, nil

	case "/help", "/h", "?":
		m.state = stateHelp
		return m, nil

	case "/quit", "/q", "exit":
		fmt.Println("👋 Goodbye!")
		os.Exit(0)
	}

	return m, nil
}

type scanCompleteMsg struct {
	path   string
	issues []models.TechnicalDebtIssue
	err    error
}

type checkUpdateMsg struct {
	info *update.UpdateInfo
	err  error
}

func startScan(path string) tea.Cmd {
	log.SetOutput(io.Discard)

	return func() tea.Msg {
		complexityStore := memory.NewInMemoryComplexityStore()
		lineCounter := analyzers.NewLineCounter()
		complexityAnalyzer := analyzers.NewComplexityAnalyzer(complexityStore)

		analyzersList := []analysis.Analyzer{lineCounter, complexityAnalyzer}

		gitService := git.NewService()
		repo, err := gitService.OpenLocal(path)
		if err != nil {
			log.SetOutput(os.Stderr)
			return scanCompleteMsg{path: path, err: fmt.Errorf("failed to open repository: %w", err)}
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, "analysisRunID", uuid.New())
		ctx = context.WithValue(ctx, "repositoryID", uuid.New())
		ctx = context.WithValue(ctx, "userID", uuid.New())
		ctx = context.WithValue(ctx, "isCLI", true)

		var allIssues []models.TechnicalDebtIssue

		for _, analyzer := range analyzersList {
			result, err := analyzer.Analyze(ctx, repo)
			if err != nil {
				continue
			}
			allIssues = append(allIssues, result.Issues...)
		}

		log.SetOutput(os.Stderr)

		return scanCompleteMsg{path: path, issues: allIssues}
	}
}

func startUpdateCheck() tea.Msg {
	return func() tea.Msg {
		ctx := context.Background()
		info, err := update.CheckForUpdate(ctx, version)
		if err != nil {
			return checkUpdateMsg{err: err}
		}
		return checkUpdateMsg{info: info}
	}
}

func (m *model) View() tea.View {
	var content string

	switch m.state {
	case stateMenu:
		content = m.menuView()
	case stateScanning:
		content = m.scanningView()
	case stateResults:
		content = m.resultsView()
	case stateUpdating:
		content = m.updateView()
	case stateHelp:
		content = m.helpView()
	default:
		content = ""
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m *model) renderInputLine() string {
	if m.input == "" {
		return ""
	}
	if m.cursorPos >= len(m.input) {
		return m.input + "█"
	}
	return m.input[:m.cursorPos] + "█" + m.input[m.cursorPos:]
}

func (m *model) menuView() string {
	const boxWidth = 100

	accentBlue := lipgloss.Color("#4fc3f7")
	// dimColor := lipgloss.Color("#4a5068")
	placeholderColor := lipgloss.Color("#3a4060")
	inputTextColor := lipgloss.Color("#89ddff")
	hintKeyColor := lipgloss.Color("#c8d0e8")
	hintSepColor := lipgloss.Color("#3a3f58")
	hintDescColor := lipgloss.Color("#5a6080")
	tipBulletColor := lipgloss.Color("#4fc3f7")
	tipTextColor := lipgloss.Color("#5a6080")
	// buildLabelColor := lipgloss.Color("#4a5068")
	// buildValueColor := lipgloss.Color("#4fc3f7")
	suggestionBg := lipgloss.Color("#1a1d30")
	suggestionFg := lipgloss.Color("#6a7090")
	suggestionSelBg := lipgloss.Color("#1e2a40")
	suggestionSelFg := lipgloss.Color("#4fc3f7")
	suggestionDescFg := lipgloss.Color("#3a4060")
	logoColor := lipgloss.Color("#8899bb")

	logoLines := []string{
		"██████╗ ███████╗██████╗ ████████╗██████╗ ██████╗  ██████╗ ███╗   ██╗███████╗",
		"██╔══██╗██╔════╝██╔══██╗╚══██╔══╝██╔══██╗██╔══██╗██╔═══██╗████╗  ██║██╔════╝",
		"██║  ██║█████╗  ██████╔╝   ██║   ██║  ██║██████╔╝██║   ██║██╔██╗ ██║█████╗  ",
		"██║  ██║██╔══╝  ██╔══██╗   ██║   ██║  ██║██╔══██╗██║   ██║██║╚██╗██║██╔══╝  ",
		"██████╔╝███████╗██████╔╝   ██║   ██████╔╝██║  ██║╚██████╔╝██║ ╚████║███████╗",
		"╚═════╝ ╚══════╝╚═════╝    ╚═╝   ╚═════╝ ╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═══╝╚══════╝",
	}

	logoStyle := lipgloss.NewStyle().Foreground(logoColor)
	var logo strings.Builder
	for _, line := range logoLines {
		logo.WriteString(logoStyle.Render(line))
		logo.WriteString("\n")
	}

	var inputText string
	if m.input == "" {
		inputText = lipgloss.NewStyle().Foreground(placeholderColor).Render(`Ask anything...  "/scan ."  to analyze the current repo`)
	} else {
		inputText = lipgloss.NewStyle().Foreground(inputTextColor).Render(m.renderInputLine())
	}

	// buildLabel := lipgloss.NewStyle().Foreground(buildLabelColor).Bold(true).Render("Build")
	// buildSep := lipgloss.NewStyle().Foreground(dimColor).Render(" · ")
	// buildVal := lipgloss.NewStyle().Foreground(buildValueColor).Render(version)
	// buildLine := buildLabel + buildSep + buildVal
	buildLine := ""

	inputBoxStyle := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.Border{Left: "│"}).
		BorderForeground(accentBlue).
		PaddingLeft(2).
		PaddingRight(2).
		PaddingTop(1).
		PaddingBottom(1).
		Width(boxWidth).
		Background(lipgloss.Color("#1e2035"))

	inputBox := inputBoxStyle.Render(inputText + "\n\n" + buildLine)

	var autocompleteBlock string
	if len(m.suggestions) > 0 {
		var sb strings.Builder
		for i, s := range m.suggestions {
			var desc string
			for _, c := range allCommands {
				if c.cmd == s {
					desc = c.desc
					break
				}
			}
			if i == m.selectedSuggestion {
				row := lipgloss.NewStyle().
					Foreground(suggestionSelFg).
					Background(suggestionSelBg).
					Bold(true).
					PaddingLeft(2).
					PaddingRight(2).
					Width(boxWidth).
					Render(s + "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#3a6080")).Bold(false).Render(desc))
				sb.WriteString(row)
			} else {
				row := lipgloss.NewStyle().
					Foreground(suggestionFg).
					Background(suggestionBg).
					PaddingLeft(2).
					PaddingRight(2).
					Width(boxWidth).
					Render(s + "  " + lipgloss.NewStyle().Foreground(suggestionDescFg).Render(desc))
				sb.WriteString(row)
			}
			sb.WriteString("\n")
		}
		autocompleteBlock = lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.Border{Left: "│"}).
			BorderForeground(accentBlue).
			Render(sb.String())
	}

	hintKey := func(k string) string {
		return lipgloss.NewStyle().Foreground(hintKeyColor).Render(k)
	}
	hintDesc := func(d string) string {
		return lipgloss.NewStyle().Foreground(hintDescColor).Render(d)
	}
	hintSep := lipgloss.NewStyle().Foreground(hintSepColor).Render(" · ")
	hints := hintKey("tab") + " " + hintDesc("cycle suggestions") + hintSep +
		hintKey("→") + " " + hintDesc("accept") + hintSep +
		hintKey("enter") + " " + hintDesc("run") + hintSep +
		hintKey("ctrl+c") + " " + hintDesc("quit")

	tipBullet := lipgloss.NewStyle().Foreground(tipBulletColor).Render("●")
	tipLabel := lipgloss.NewStyle().Foreground(tipBulletColor).Bold(true).Render(" Tip")
	tipText := lipgloss.NewStyle().Foreground(tipTextColor).Render(" Type ")
	tipCmd := lipgloss.NewStyle().Foreground(tipBulletColor).Render("/scan .")
	tipRest := lipgloss.NewStyle().Foreground(tipTextColor).Render(" to analyze the current directory")
	tip := lipgloss.NewStyle().PaddingBottom(2).Render(tipBullet + tipLabel + tipText + tipCmd + tipRest)

	var inner strings.Builder
	inner.WriteString(logo.String())
	inner.WriteString("\n")
	inner.WriteString(inputBox)
	inner.WriteString("\n")
	if autocompleteBlock != "" {
		inner.WriteString(autocompleteBlock)
		inner.WriteString("\n")
	}
	inner.WriteString(hints)
	inner.WriteString("\n\n")
	inner.WriteString(tip)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner.String())
}

func (m *model) scanningView() string {
	const boxWidth = 150
	spinner := spinnerChars[m.spinnerFrame]
	accentBlue := lipgloss.Color("#4fc3f7")
	dimColor := lipgloss.Color("#4a5068")
	pathColor := lipgloss.Color("#8899bb")

	body := lipgloss.NewStyle().Foreground(accentBlue).Bold(true).Render(spinner+" Scanning…") +
		"\n" +
		lipgloss.NewStyle().Foreground(dimColor).Render("Path  ") +
		lipgloss.NewStyle().Foreground(pathColor).Render(m.scanPath)

	box := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.Border{Left: "│"}).
		BorderForeground(accentBlue).
		PaddingLeft(2).PaddingRight(2).PaddingTop(1).PaddingBottom(1).
		Width(boxWidth).
		Background(lipgloss.Color("#1e2035")).
		Render(body)

	hint := lipgloss.NewStyle().Foreground(dimColor).Render("ctrl+c to cancel")

	inner := box + "\n\n" + hint
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
}

func (m *model) resultsView() string {
	dimColor := lipgloss.Color("#4a5068")
	okColor := lipgloss.Color("#5af78e")
	errorColor := lipgloss.Color("#ff5f5f")

	if m.err != nil {
		body := lipgloss.NewStyle().Foreground(errorColor).Bold(true).Render("Scan failed") +
			"\n" + lipgloss.NewStyle().Foreground(dimColor).Render(m.err.Error())
		box := lipgloss.NewStyle().
			BorderLeft(true).BorderStyle(lipgloss.Border{Left: "│"}).BorderForeground(errorColor).
			PaddingLeft(2).PaddingRight(2).PaddingTop(1).PaddingBottom(1).Width(150).
			Background(lipgloss.Color("#1e2035")).Render(body)
		hint := lipgloss.NewStyle().Foreground(dimColor).Render("r  rescan    q  quit")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box+"\n\n"+hint)
	}

	if len(m.issues) == 0 {
		body := lipgloss.NewStyle().Foreground(okColor).Bold(true).Render("No issues found — clean scan!")
		box := lipgloss.NewStyle().
			BorderLeft(true).BorderStyle(lipgloss.Border{Left: "│"}).BorderForeground(okColor).
			PaddingLeft(2).PaddingRight(2).PaddingTop(1).PaddingBottom(1).Width(150).
			Background(lipgloss.Color("#1e2035")).Render(body)
		hint := lipgloss.NewStyle().Foreground(dimColor).Render("r  rescan    q  quit")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box+"\n\n"+hint)
	}

	hintStyle := lipgloss.NewStyle().Foreground(dimColor)

	var b strings.Builder

	criticalCol := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")).Bold(true).Width(12)
	highCol := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa55")).Bold(true).Width(12)
	mediumCol := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd080")).Width(12)
	lowCol := lipgloss.NewStyle().Foreground(lipgloss.Color("#5a6080")).Width(12)
	fileCol := lipgloss.NewStyle().Foreground(lipgloss.Color("#8899bb")).Width(45)
	descCol := lipgloss.NewStyle().Foreground(lipgloss.Color("#c8d0e8"))
	headerCol := lipgloss.NewStyle().Foreground(lipgloss.Color("#4fc3f7")).Bold(true)

	headerCritical := headerCol.Width(12).Render("Criticality")
	headerFile := headerCol.Width(45).Render("File Path")
	headerDesc := headerCol.Render("Description")

	b.WriteString(headerCritical + " │ " + headerFile + " │ " + headerDesc + "\n")
	b.WriteString(strings.Repeat("─", 12) + " ┼ " + strings.Repeat("─", 45) + " ┼ " + strings.Repeat("─", 40) + "\n")

	maxLines := 100
	endIdx := min(m.scrollOffset+maxLines, len(m.issues))

	for i := m.scrollOffset; i < endIdx; i++ {
		issue := m.issues[i]
		var severityCol lipgloss.Style
		switch issue.Severity {
		case "critical":
			severityCol = criticalCol
		case "high":
			severityCol = highCol
		case "medium":
			severityCol = mediumCol
		default:
			severityCol = lowCol
		}

		severityStr := severityCol.Render(issue.Severity)
		filePathStr := fileCol.Render(truncate(issue.FilePath, 43))
		descStr := descCol.Render(truncate(issue.Message, 40))
		b.WriteString(severityStr + " │ " + filePathStr + " │ " + descStr + "\n")
	}

	b.WriteString("\n")
	b.WriteString(hintStyle.Render("j/k · ↑/↓  scroll    g/G  top/bottom    pgup/pgdn  page    r  rescan    q  quit"))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, b.String())
}

func (m *model) updateView() string {
	const boxWidth = 100
	accentBlue := lipgloss.Color("#4fc3f7")
	dimColor := lipgloss.Color("#4a5068")
	errorColor := lipgloss.Color("#ff5f5f")
	okColor := lipgloss.Color("#5af78e")

	boxStyle := func(border lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.Border{Left: "│"}).
			BorderForeground(border).
			PaddingLeft(2).PaddingRight(2).PaddingTop(1).PaddingBottom(1).
			Width(boxWidth).
			Background(lipgloss.Color("#1e2035"))
	}
	hint := func(s string) string {
		return lipgloss.NewStyle().Foreground(dimColor).Render(s)
	}

	if m.err != nil {
		body := lipgloss.NewStyle().Foreground(errorColor).Bold(true).Render("Update check failed") +
			"\n" + lipgloss.NewStyle().Foreground(dimColor).Render(m.err.Error())
		inner := boxStyle(errorColor).Render(body) + "\n\n" + hint("q  back")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
	}

	if m.updateInfo == nil {
		body := lipgloss.NewStyle().Foreground(accentBlue).Bold(true).Render("Checking for updates…")
		inner := boxStyle(accentBlue).Render(body)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
	}

	if !m.updateInfo.Available {
		body := lipgloss.NewStyle().Foreground(okColor).Bold(true).Render("You are on the latest version")
		inner := boxStyle(okColor).Render(body) + "\n\n" + hint("q  back")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
	}

	body := lipgloss.NewStyle().Foreground(accentBlue).Bold(true).Render("Update Available") +
		"\n" + lipgloss.NewStyle().Foreground(dimColor).Render("New version  ") +
		lipgloss.NewStyle().Foreground(accentBlue).Render(m.updateInfo.Version)
	inner := boxStyle(accentBlue).Render(body) + "\n\n" + hint("y  install    n  skip")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
}

func (m *model) helpView() string {
	const boxWidth = 100
	accentBlue := lipgloss.Color("#4fc3f7")
	dimColor := lipgloss.Color("#4a5068")
	keyColor := lipgloss.Color("#4fc3f7")
	descColor := lipgloss.Color("#8899bb")

	headerStyle := lipgloss.NewStyle().Foreground(accentBlue).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(keyColor).Bold(true).Width(18)
	descStyle := lipgloss.NewStyle().Foreground(descColor)
	hintStyle := lipgloss.NewStyle().Foreground(dimColor)

	var rows strings.Builder
	rows.WriteString(headerStyle.Render("Commands"))
	rows.WriteString("\n\n")
	for _, c := range allCommands {
		rows.WriteString(keyStyle.Render(c.cmd))
		rows.WriteString("  ")
		rows.WriteString(descStyle.Render(c.desc))
		rows.WriteString("\n")
	}
	rows.WriteString("\n")
	rows.WriteString(hintStyle.Render("q / esc  back"))

	box := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.Border{Left: "│"}).
		BorderForeground(accentBlue).
		PaddingLeft(2).PaddingRight(2).PaddingTop(1).PaddingBottom(1).
		Width(boxWidth).
		Background(lipgloss.Color("#1e2035")).
		Render(rows.String())

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func RunTUI() error {
	p := tea.NewProgram(
		initialModel(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	return nil
}

func performUpdateCmd() tea.Msg {
	return func() tea.Msg {
		ctx := context.Background()
		err := update.PerformUpdate(ctx)
		if err != nil {
			fmt.Printf("Update failed: %v\n", err)
		}
		return nil
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
