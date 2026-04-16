package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

// ─────────────────────────────────────────────────────────────────────────────
// MenuModel
// ─────────────────────────────────────────────────────────────────────────────

// MenuModel owns the main prompt screen and the help overlay. It handles
// command input, tab-completion, and the /help view.
//
// Navigation contract:
//   - /scan <path>  → StartScanMsg (AppModel reads config, starts ScanModel)
//   - /update       → StartUpdateMsg
//   - /history      → NavigateMsg{stateHistory}
//   - /config       → NavigateMsg{stateConfig}
//   - /help         → NavigateMsg{stateHelp}  (renders via renderHelp)
//   - esc in help   → NavigateMsg{stateMenu}
//   - /quit         → os.Exit
type MenuModel struct {
	input              string
	cursorPos          int
	suggestions        []string
	selectedSuggestion int
	pathComplete       bool   // true when suggestions are filesystem paths for /scan
	showingHelp        bool
	err                string // last command error (displayed below input box)
	width, height      int
}

func newMenuModel() *MenuModel {
	return &MenuModel{
		selectedSuggestion: -1,
		width:              120,
		height:             40,
	}
}

// Reset is called by AppModel.navigateTo when returning to the menu screen.
// It clears the input and error state so the prompt is always clean on entry.
func (m *MenuModel) Reset() {
	m.input = ""
	m.cursorPos = 0
	m.suggestions = nil
	m.selectedSuggestion = -1
	m.showingHelp = false
	m.err = ""
}

// ShowHelp is called by AppModel.navigateTo(stateHelp) to flip the help flag.
func (m *MenuModel) ShowHelp() {
	m.showingHelp = true
}

// Init satisfies tea.Model.
func (m *MenuModel) Init() tea.Cmd { return nil }

// Update handles keyboard input for both the menu prompt and the help overlay.
func (m *MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyPressMsg:
		str := msg.String()

		// Help overlay: only esc/q navigates back.
		if m.showingHelp {
			if str == "esc" || str == "q" {
				m.showingHelp = false
				return m, func() tea.Msg { return NavigateMsg{State: stateMenu} }
			}
			return m, nil
		}

		return m.handleMenuKey(str)
	}
	return m, nil
}

func (m *MenuModel) handleMenuKey(str string) (tea.Model, tea.Cmd) {
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
	default:
		if len(str) == 1 {
			m.input = m.input[:m.cursorPos] + str + m.input[m.cursorPos:]
			m.cursorPos++
			m.computeSuggestions()
		}
	}
	return m, nil
}

// handleCommand parses the current input buffer and returns the appropriate
// router message as a tea.Cmd. AppModel intercepts the resulting messages.
func (m *MenuModel) handleCommand() (tea.Model, tea.Cmd) {
	cmd := strings.TrimSpace(m.input)
	m.input = ""
	m.cursorPos = 0
	m.suggestions = nil
	m.selectedSuggestion = -1
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
			m.err = fmt.Sprintf("failed to resolve path: %v", err)
			return m, nil
		}
		// Emit StartScanMsg. AppModel intercepts it, reads config values,
		// and calls ScanModel.Start with the full options.
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
		fmt.Println("👋 Goodbye!")
		os.Exit(0)
	}
	return m, nil
}

// computeSuggestions recomputes m.suggestions for the current input.
// It operates in two modes:
//
//	Command mode: input starts with any prefix of a known command → suggest
//	             command names (e.g. "/s" → "/scan", "/update", …).
//	Path mode:   input starts with "/scan " (note the space) → suggest
//	             filesystem paths relative to the working directory.
func (m *MenuModel) computeSuggestions() {
	m.selectedSuggestion = -1

	// ── Path completion for /scan ─────────────────────────────────────────
	// Triggered as soon as the user types the space after "/scan".
	if strings.HasPrefix(strings.ToLower(m.input), "/scan ") {
		m.pathComplete = true
		pathPrefix := m.input[len("/scan "):]
		m.suggestions = pathSuggestions(pathPrefix)
		return
	}

	// ── Command completion ─────────────────────────────────────────────────
	m.pathComplete = false
	if m.input == "" {
		m.suggestions = nil
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
}

// acceptSuggestion inserts the highlighted (or first) suggestion into the
// input buffer. Behaviour differs between command and path modes:
//
//	Command mode: replaces the whole input with "/command " (trailing space).
//	Path mode:    replaces the path argument with the chosen path and
//	              immediately recomputes suggestions so the user can keep
//	              drilling into subdirectories with Tab.
func (m *MenuModel) acceptSuggestion() {
	if len(m.suggestions) == 0 {
		return
	}
	idx := m.selectedSuggestion
	if idx < 0 || idx >= len(m.suggestions) {
		idx = 0
	}
	chosen := m.suggestions[idx]
	if chosen == "" {
		return
	}

	if m.pathComplete {
		// Keep the "/scan " prefix and replace the path argument.
		m.input = "/scan " + chosen
		m.cursorPos = len(m.input)
		// Immediately recompute: if chosen ends in "/" the user can keep
		// tabbing into subdirectories without typing anything more.
		m.computeSuggestions()
		return
	}

	// Command mode: insert "command " and clear suggestions.
	m.input = chosen + " "
	m.cursorPos = len(m.input)
	m.suggestions = nil
	m.selectedSuggestion = -1
}

// pathSuggestions returns up to maxPathSuggestions directory paths that match
// prefix, suitable for the /scan argument. It normalises relative paths to the
// "./" form for readability and appends "/" to every result to signal that the
// entry is a directory and to allow further completion.
func pathSuggestions(prefix string) []string {
	const maxSuggestions = 10

	// Normalise relative paths (that are not "../…") to start with "./" so
	// the suggestions are visually consistent.
	if prefix != "" && !filepath.IsAbs(prefix) &&
		!strings.HasPrefix(prefix, "./") &&
		!strings.HasPrefix(prefix, "../") &&
		prefix != "." && prefix != ".." {
		prefix = "./" + prefix
	}

	// Determine the directory to list and the name prefix to filter by.
	var dirToList, nameFilter string
	switch {
	case prefix == "":
		dirToList, nameFilter = ".", ""
	case strings.HasSuffix(prefix, "/"):
		// User finished a path segment — list the children of that dir.
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
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip hidden directories unless the user is explicitly typing a
		// dot-prefixed name (e.g. ".git" or ".config").
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(nameFilter, ".") {
			continue
		}
		if nameFilter != "" && !strings.HasPrefix(name, nameFilter) {
			continue
		}

		// Compose the full path in the same style as the prefix.
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

// View satisfies tea.Model.
func (m *MenuModel) View() tea.View {
	return tea.NewView(m.render())
}

// render produces the main menu prompt screen.
func (m *MenuModel) render() string {
	const boxWidth = 100

	accentBlue := lipgloss.Color("#4fc3f7")
	placeholderColor := lipgloss.Color("#3a4060")
	inputTextColor := lipgloss.Color("#89ddff")
	hintKeyColor := lipgloss.Color("#c8d0e8")
	hintSepColor := lipgloss.Color("#3a3f58")
	hintDescColor := lipgloss.Color("#5a6080")
	tipBulletColor := lipgloss.Color("#4fc3f7")
	tipTextColor := lipgloss.Color("#5a6080")
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
		inputText = lipgloss.NewStyle().Foreground(placeholderColor).
			Render(`Ask anything...  "/scan ."  to analyze the current repo`)
	} else {
		inputText = lipgloss.NewStyle().Foreground(inputTextColor).Render(m.renderInputLine())
	}

	var errLine string
	if m.err != "" {
		errLine = "\n" + lipgloss.NewStyle().Foreground(colorError).Render("⚠ "+m.err)
	}

	inputBoxStyle := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.Border{Left: "│"}).
		BorderForeground(accentBlue).
		PaddingLeft(2).PaddingRight(2).PaddingTop(1).PaddingBottom(1).
		Width(boxWidth).
		Background(lipgloss.Color("#1e2035"))

	inputBox := inputBoxStyle.Render(inputText + "\n" + errLine)

	var autocompleteBlock string
	if len(m.suggestions) > 0 {
		// Resolve the right-hand annotation for each suggestion row.
		// Command mode: description from allCommands.
		// Path mode:    a dim "directory" badge (the trailing "/" in the
		//               suggestion name already signals it's a dir, but the
		//               badge keeps the visual rhythm consistent).
		annotation := func(s string) string {
			if m.pathComplete {
				return lipgloss.NewStyle().Foreground(suggestionDescFg).Render("directory")
			}
			for _, c := range allCommands {
				if c.cmd == s {
					return lipgloss.NewStyle().Foreground(suggestionDescFg).Render(c.desc)
				}
			}
			return ""
		}

		var sb strings.Builder
		for i, s := range m.suggestions {
			ann := annotation(s)
			label := s
			if ann != "" {
				label = s + "  " + ann
			}
			if i == m.selectedSuggestion {
				row := lipgloss.NewStyle().
					Foreground(suggestionSelFg).Background(suggestionSelBg).Bold(true).
					PaddingLeft(2).PaddingRight(2).Width(boxWidth).
					Render(label)
				sb.WriteString(row)
			} else {
				row := lipgloss.NewStyle().
					Foreground(suggestionFg).Background(suggestionBg).
					PaddingLeft(2).PaddingRight(2).Width(boxWidth).
					Render(label)
				sb.WriteString(row)
			}
			sb.WriteString("\n")
		}
		autocompleteBlock = lipgloss.NewStyle().
			BorderLeft(true).BorderStyle(lipgloss.Border{Left: "│"}).BorderForeground(accentBlue).
			Render(sb.String())
	}

	hintKey := func(k string) string { return lipgloss.NewStyle().Foreground(hintKeyColor).Render(k) }
	hintDesc := func(d string) string { return lipgloss.NewStyle().Foreground(hintDescColor).Render(d) }
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

// renderHelp produces the /help overlay screen.
func (m *MenuModel) renderHelp() string {
	const boxWidth = 100
	accentBlue := lipgloss.Color("#4fc3f7")
	dimColor := lipgloss.Color("#4a5068")

	headerStyle := lipgloss.NewStyle().Foreground(accentBlue).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(accentBlue).Bold(true).Width(18)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8899bb"))
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
		BorderLeft(true).BorderStyle(lipgloss.Border{Left: "│"}).BorderForeground(accentBlue).
		PaddingLeft(2).PaddingRight(2).PaddingTop(1).PaddingBottom(1).
		Width(boxWidth).Background(lipgloss.Color("#1e2035")).
		Render(rows.String())

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
