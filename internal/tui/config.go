package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

type configMode int

const (
	configNavigating configMode = iota
	configEditing
)

type configItem struct {
	Category    string
	Key         string
	Value       string
	Type        string
	Description string
	Options     []string
	IsOption    bool
}

func defaultConfigItems() []configItem {
	return []configItem{
		{
			Category:    "General",
			Key:         "Output Format",
			Value:       "text",
			Type:        "string",
			Description: "Render mode for scan results",
			Options:     []string{"text", "json"},
			IsOption:    true,
		},
		{
			Category:    "General",
			Key:         "Auto-Update Checks",
			Value:       "true",
			Type:        "bool",
			Description: "Check for a newer release on each startup",
		},
		{
			Category:    "Quality Gate",
			Key:         "Fail on Severity",
			Value:       "high",
			Type:        "string",
			Description: "Min severity for non-zero exit code",
			Options:     []string{"low", "medium", "high", "critical", "none"},
			IsOption:    true,
		},
		{
			Category:    "Quality Gate",
			Key:         "Max Complexity",
			Value:       "15",
			Type:        "int",
			Description: "Cyclomatic-complexity threshold per function",
		},
		{
			Category:    "Quality Gate",
			Key:         "Security Scan",
			Value:       "true",
			Type:        "bool",
			Description: "Run Trivy vulnerability and secret detection",
		},
		{
			Category:    "Display",
			Key:         "Show Line Numbers",
			Value:       "true",
			Type:        "bool",
			Description: "Include line:col in the results list",
		},
		{
			Category:    "Display",
			Key:         "Max Results",
			Value:       "500",
			Type:        "int",
			Description: "Cap on issues rendered per scan (0 = unlimited)",
		},
	}
}

// ConfigModel manages the settings screen.
type ConfigModel struct {
	items       []configItem
	cursor      int
	offset      int
	mode        configMode
	editBuffer  string
	width       int
	height      int
}

func newConfigModel() *ConfigModel {
	return &ConfigModel{
		items:  defaultConfigItems(),
		mode:   configNavigating,
		width:  120,
		height: 40,
	}
}

func (m *ConfigModel) Reset() {
	m.cursor = 0
	m.offset = 0
	m.mode = configNavigating
	m.editBuffer = ""
}

func (m *ConfigModel) GetValue(key string) string {
	for _, item := range m.items {
		if item.Key == key {
			return item.Value
		}
	}
	return ""
}

func (m *ConfigModel) Init() tea.Cmd { return nil }

func (m *ConfigModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyPressMsg:
		switch m.mode {
		case configNavigating:
			return m.handleNavKey(msg.String())
		case configEditing:
			return m.handleEditKey(msg.String())
		}
	}
	return m, nil
}

// handleNavKey processes keyboard input in navigation (non-editing) mode.
func (m *ConfigModel) handleNavKey(str string) (tea.Model, tea.Cmd) {
	visibleRows := max(m.height-10, 4)

	switch str {
	case "j", "down":
		if m.cursor < len(m.items)-1 {
			m.cursor++
			if m.cursor >= m.offset+visibleRows {
				m.offset++
			}
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.offset {
				m.offset--
			}
		}
	case "g":
		m.cursor, m.offset = 0, 0
	case "G":
		m.cursor = len(m.items) - 1
		m.offset = max(0, m.cursor-visibleRows+1)

	case "q", "esc":
		return m, func() tea.Msg { return NavigateMsg{State: stateMenu} }

	case "enter", "space", " ":
		item := &m.items[m.cursor]
		switch {
		case item.Type == "bool":
			if item.Value == "true" {
				item.Value = "false"
			} else {
				item.Value = "true"
			}
		case item.IsOption:
			m.cycleOption(item, +1)
		default:
			m.editBuffer = item.Value
			m.mode = configEditing
		}
	case "right":
		item := &m.items[m.cursor]
		if item.IsOption {
			m.cycleOption(item, +1)
		}
	case "left":
		item := &m.items[m.cursor]
		if item.IsOption {
			m.cycleOption(item, -1)
		}
	}
	return m, nil
}

// handleEditKey processes keyboard input while the user is editing a
// free-text config value.
func (m *ConfigModel) handleEditKey(str string) (tea.Model, tea.Cmd) {
	switch str {
	case "esc":
		m.editBuffer = ""
		m.mode = configNavigating
	case "enter":
		m.items[m.cursor].Value = m.editBuffer
		m.editBuffer = ""
		m.mode = configNavigating
	case "backspace":
		runes := []rune(m.editBuffer)
		if len(runes) > 0 {
			m.editBuffer = string(runes[:len(runes)-1])
		}
	default:
		if isEditableChar(str) {
			m.editBuffer += str
		}
	}
	return m, nil
}

// cycleOption advances (delta=+1) or reverses (delta=-1) through the option
// list for items with IsOption=true, wrapping at both ends.
func (m *ConfigModel) cycleOption(item *configItem, delta int) {
	for i, opt := range item.Options {
		if opt == item.Value {
			n := (i + delta + len(item.Options)) % len(item.Options)
			item.Value = item.Options[n]
			return
		}
	}
	if len(item.Options) > 0 {
		item.Value = item.Options[0]
	}
}

// View satisfies tea.Model. AppModel calls render() directly; this wrapper
// exists only to fulfil the interface so ConfigModel can be used anywhere a
// tea.Model is expected.
func (m *ConfigModel) View() tea.View {
	return tea.NewView(m.render())
}

// render produces the full config-screen string using cached dimensions.
func (m *ConfigModel) render() string {
	const boxWidth = 104
	const innerWidth = boxWidth - 6

	const keyW = 22
	const valW = 20
	const gap = 2
	descW := innerWidth - keyW - valW - (gap * 2)

	titleStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)

	categoryStyle := lipgloss.NewStyle().Foreground(colorDim).Bold(true)

	keyNormalStyle := lipgloss.NewStyle().Foreground(colorDim).Width(keyW)

	keySelectedStyle := lipgloss.NewStyle().
		Foreground(colorAccentBlue).Bold(true).Width(keyW)

	descStyle := lipgloss.NewStyle().Foreground(colorDim).Width(descW)

	valueBadge := func(item configItem, idx int) string {
		displayVal := item.Value
		if m.mode == configEditing && idx == m.cursor {
			displayVal = m.editBuffer + "█"
		}
		inner := truncate(displayVal, valW-4)

		var bracketColor lipgloss.Color
		switch {
		case item.Type == "bool" && item.Value == "true":
			bracketColor = colorOK
		case item.Type == "bool":
			bracketColor = colorDim
		default:
			bracketColor = colorAccentBlue
		}

		var content string
		if item.IsOption && idx == m.cursor {
			content = "← [ " + inner + " ] →"
		} else {
			content = "  [ " + inner + " ]  "
		}
		return lipgloss.NewStyle().
			Foreground(bracketColor).
			Width(valW + 4).
			Render(content)
	}

	rowBg := lipgloss.NewStyle().Background(colorSelectedBg).Width(innerWidth)
	normalRow := lipgloss.NewStyle().Width(innerWidth)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Settings"))
	b.WriteString("\n\n")

	var lastCategory string
	visibleRows := max(m.height-10, 4)
	end := min(m.offset+visibleRows, len(m.items))

	for i := m.offset; i < end; i++ {
		item := m.items[i]

		if item.Category != lastCategory {
			if lastCategory != "" {
				b.WriteString("\n")
			}
			lastCategory = item.Category

			divPad := innerWidth - len(item.Category) - 6
			divider := "──── " +
				categoryStyle.Render(item.Category) +
				lipgloss.NewStyle().Foreground(colorDim).
					Render(" "+strings.Repeat("─", max(divPad, 2)))
			b.WriteString(divider)
			b.WriteString("\n")
		}

		var keyRendered string
		if i == m.cursor {
			keyRendered = keySelectedStyle.Render(item.Key)
		} else {
			keyRendered = keyNormalStyle.Render(item.Key)
		}

		row := keyRendered +
			strings.Repeat(" ", gap) +
			descStyle.Render(item.Description) +
			strings.Repeat(" ", gap) +
			valueBadge(item, i)

		if i == m.cursor {
			b.WriteString(rowBg.Render(row))
		} else {
			b.WriteString(normalRow.Render(row))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	hintStyle := lipgloss.NewStyle().Foreground(colorDim)
	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("#3a3f58")).Render("  ·  ")
	k := func(s string) string { return lipgloss.NewStyle().Foreground(colorText).Render(s) }

	var hints string
	if m.mode == configNavigating {
		hints = hintStyle.Render(
			k("↑/↓") + hintStyle.Render(" navigate") + sep +
				k("←/→") + hintStyle.Render(" cycle") + sep +
				k("enter/space") + hintStyle.Render(" edit/toggle") + sep +
				k("esc") + hintStyle.Render(" back"),
		)
	} else {
		ke := func(s string) string { return lipgloss.NewStyle().Foreground(colorOK).Render(s) }
		hints = hintStyle.Render(
			ke("type") + hintStyle.Render(" to edit") + sep +
				ke("enter") + hintStyle.Render(" save") + sep +
				ke("esc") + hintStyle.Render(" cancel"),
		)
	}
	b.WriteString(hints)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentBlue).
		Padding(1, 3).
		Width(boxWidth).
		Background(colorBg).
		Render(b.String())

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
