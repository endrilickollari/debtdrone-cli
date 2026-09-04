package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
)

type configMode int

const (
	configNavigating configMode = iota
	configEditing
)

type configItem struct {
	Category    string
	ConfigKey   localconfig.Key
	Key         string
	Value       string
	Type        string
	Description string
	Options     []string
	IsOption    bool
}

func defaultConfigItems() []configItem {
	return configItems(localconfig.Defaults())
}

func configItems(values localconfig.Values) []configItem {
	return []configItem{
		{
			Category:    "General",
			ConfigKey:   localconfig.KeyOutputFormat,
			Key:         "Output Format",
			Value:       localconfig.Value(values, localconfig.KeyOutputFormat),
			Type:        "string",
			Description: "Render mode for scan results",
			Options:     []string{"text", "json"},
			IsOption:    true,
		},
		{
			Category:    "General",
			ConfigKey:   localconfig.KeyUpdateChecks,
			Key:         "Auto-Update Checks",
			Value:       localconfig.Value(values, localconfig.KeyUpdateChecks),
			Type:        "bool",
			Description: "Check for a newer release on each startup",
		},
		{
			Category:    "Quality Gate",
			ConfigKey:   localconfig.KeyFailOn,
			Key:         "Fail on Severity",
			Value:       localconfig.Value(values, localconfig.KeyFailOn),
			Type:        "string",
			Description: "Min severity for non-zero exit code",
			Options:     []string{"low", "medium", "high", "critical", "none"},
			IsOption:    true,
		},
		{
			Category:    "Quality Gate",
			ConfigKey:   localconfig.KeyMaxComplexity,
			Key:         "Max Complexity",
			Value:       localconfig.Value(values, localconfig.KeyMaxComplexity),
			Type:        "int",
			Description: "Cyclomatic-complexity threshold per function",
		},
		{
			Category:    "Quality Gate",
			ConfigKey:   localconfig.KeySecurityScan,
			Key:         "Security Scan",
			Value:       localconfig.Value(values, localconfig.KeySecurityScan),
			Type:        "bool",
			Description: "Run Trivy vulnerability and secret detection",
		},
		{
			Category:    "Quality Gate",
			ConfigKey:   localconfig.KeyCoverage,
			Key:         "Coverage",
			Value:       localconfig.Value(values, localconfig.KeyCoverage),
			Type:        "bool",
			Description: "Parse existing coverage artifacts",
		},
		{
			Category:    "Display",
			ConfigKey:   localconfig.KeyShowLineNumbers,
			Key:         "Show Line Numbers",
			Value:       localconfig.Value(values, localconfig.KeyShowLineNumbers),
			Type:        "bool",
			Description: "Include line:col in the results list",
		},
		{
			Category:    "Display",
			ConfigKey:   localconfig.KeyMaxResults,
			Key:         "Max Results",
			Value:       localconfig.Value(values, localconfig.KeyMaxResults),
			Type:        "int",
			Description: "Cap on issues rendered per scan (0 = unlimited)",
		},
		{
			Category:    "Privacy",
			ConfigKey:   localconfig.KeyHistoryEnabled,
			Key:         "History Persistence",
			Value:       localconfig.Value(values, localconfig.KeyHistoryEnabled),
			Type:        "bool",
			Description: "Persist privacy-safe scan summaries locally",
		},
	}
}

// ConfigModel manages the settings screen.
type ConfigModel struct {
	items           []configItem
	cursor          int
	offset          int
	mode            configMode
	editBuffer      string
	validationError string
	width           int
	height          int
}

func newConfigModel() *ConfigModel {
	return newConfigModelWithValues(localconfig.Defaults())
}

func newConfigModelWithValues(values localconfig.Values) *ConfigModel {
	return &ConfigModel{
		items:  configItems(values),
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
	m.validationError = ""
}

func (m *ConfigModel) ConfigValue(key localconfig.Key) string {
	for _, item := range m.items {
		if item.ConfigKey == key {
			return item.Value
		}
	}
	return ""
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
			value := "true"
			if item.Value == "true" {
				value = "false"
			}
			m.applyValue(item, value)
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

func (m *ConfigModel) handleEditKey(str string) (tea.Model, tea.Cmd) {
	switch str {
	case "esc":
		m.editBuffer = ""
		m.validationError = ""
		m.mode = configNavigating
	case "enter":
		if m.applyValue(&m.items[m.cursor], m.editBuffer) {
			m.editBuffer = ""
			m.mode = configNavigating
		}
	case "backspace":
		runes := []rune(m.editBuffer)
		if len(runes) > 0 {
			m.editBuffer = string(runes[:len(runes)-1])
		}
	default:
		if isEditableChar(str) {
			m.editBuffer += str
			m.validationError = ""
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
			m.applyValue(item, item.Options[n])
			return
		}
	}
	if len(item.Options) > 0 {
		m.applyValue(item, item.Options[0])
	}
}

func (m *ConfigModel) applyValue(item *configItem, value string) bool {
	override, err := localconfig.ParseOverride(item.ConfigKey, value)
	if err != nil {
		m.validationError = err.Error()
		return false
	}
	canonical, ok := localconfig.OverrideValue(override, item.ConfigKey)
	if !ok {
		m.validationError = "configuration value is unavailable"
		return false
	}
	item.Value = canonical
	m.validationError = ""
	return true
}

func (m *ConfigModel) View() tea.View {
	return tea.NewView(m.render())
}

func (m *ConfigModel) render() string {
	// The editor fills the terminal up to its comfortable maximum, so it never
	// renders wider than the window it is drawn in.
	boxWidth := min(104, max(m.width-2, 20))
	innerWidth := boxWidth - 6

	const keyW = 22
	const valW = 20
	const gap = 2
	const markerW = 2
	// The description column is the first thing to go when the terminal cannot
	// hold the key and value columns alongside it. Kept below its minimum it
	// would wrap one word per line, which is far less readable than dropping it.
	const minimumDescW = 18
	descW := innerWidth - markerW - keyW - valW - (gap * 2)
	showDescription := descW >= minimumDescW

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
	cursorLine := 0
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

		marker := "  "
		if i == m.cursor {
			marker = "› "
		}
		row := marker + keyRendered + strings.Repeat(" ", gap)
		if showDescription {
			// Truncated rather than wrapped: a description that spills onto a
			// second line turns every setting into a two-row entry.
			row += descStyle.Render(truncate(item.Description, descW)) + strings.Repeat(" ", gap)
		}
		row += valueBadge(item, i)

		if i == m.cursor {
			// Recorded so the scrolling window below can keep the focused row
			// on screen.
			cursorLine = strings.Count(b.String(), "\n")
			b.WriteString(rowBg.Render(row))
		} else {
			b.WriteString(normalRow.Render(row))
		}
		b.WriteString("\n")
	}

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
	// The settings list scrolls when it is taller than the terminal, keeping the
	// row being edited on screen. Without this the lower sections render past
	// the bottom edge, where they can be selected but never read.
	settings := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	const boxChrome = 4 // border and vertical padding
	// Measured at the box's inner width: the hint line wraps on a narrow
	// terminal, and an unwrapped measurement would under-reserve its rows.
	hintHeight := lipgloss.Height(lipgloss.NewStyle().Width(innerWidth).Render(hints))
	errorHeight := 0
	if m.validationError != "" {
		errorHeight = lipgloss.Height(lipgloss.NewStyle().Width(innerWidth).Render(m.validationError)) + 1
	}
	visible := windowLines(settings, m.height-boxChrome-hintHeight-errorHeight-1, cursorLine)

	var content strings.Builder
	content.WriteString(strings.Join(visible, "\n"))
	content.WriteString("\n\n")
	content.WriteString(hints)
	if m.validationError != "" {
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Foreground(colorCritical).Width(innerWidth).Render(m.validationError))
	}
	b = content

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentBlue).
		Padding(1, 3).
		Width(boxWidth).
		Background(colorBg).
		Render(b.String())

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
