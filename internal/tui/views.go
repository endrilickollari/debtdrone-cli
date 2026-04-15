package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/internal/update"
)

func (m *model) View() tea.View {
	var content string

	switch m.state {
	case stateMenu:
		content = m.menuView()
	case stateScanning:
		content = m.scanningView()
	case stateResults:
		content = m.resultsView()
	case stateHistory:
		content = m.historyView()
	case stateConfig:
		content = m.configView()
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
		inputText = lipgloss.NewStyle().Foreground(placeholderColor).Render(`Ask anything...  "/scan ."  to analyze the current repo`)
	} else {
		inputText = lipgloss.NewStyle().Foreground(inputTextColor).Render(m.renderInputLine())
	}

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
	const boxWidth = 80
	spinner := spinnerChars[m.spinnerFrame]
	accentBlue := lipgloss.Color("#4fc3f7")
	dimColor := lipgloss.Color("#4a5068")
	pathColor := lipgloss.Color("#8899bb")
	progressColor := lipgloss.Color("#5af78e")

	// Create a simple progress bar
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
		lipgloss.NewStyle().Foreground(dimColor).Render("Task  ") +
			lipgloss.NewStyle().Foreground(colorText).Render(m.scanTask),
		lipgloss.NewStyle().Foreground(dimColor).Render("Path  ") +
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

	inner := box + "\n\n" + hint
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, inner)
}

func (m *model) resultsView() string {
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

	if m.getConfigValue("Output Format") == "json" {
		const divTitle = " Raw JSON Results "
		innerW := max(m.width-len(divTitle)-4, 0)
		leftW := innerW / 2
		rightW := innerW - leftW
		divider := lipgloss.NewStyle().Foreground(colorAccentBlue).Render(
			strings.Repeat("─", leftW) +
				lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true).Render(divTitle) +
				strings.Repeat("─", rightW),
		)

		detailInner := m.detail.view()
		detailPane := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccentBlue).
			Width(m.width - 2).
			Render(detailInner)

		hintStyle := lipgloss.NewStyle().Foreground(colorDim)
		hints := hintStyle.Render(
			"j/k or J/K scroll json   " +
				"r rescan   " +
				"q quit",
		)

		return lipgloss.JoinVertical(lipgloss.Left,
			divider,
			detailPane,
			hints,
		)
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

	detailInner := m.detail.view()
	detailPane := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentBlue).
		Width(m.width - 2).
		Render(detailInner)

	hintStyle := lipgloss.NewStyle().Foreground(colorDim)
	hints := hintStyle.Render(
		"j/k ↑↓ navigate   " +
			"J/K scroll detail   " +
			"g/G top/bottom   " +
			"pgup/pgdn page   " +
			"r rescan   " +
			"q quit",
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		listPane,
		divider,
		detailPane,
		hints,
	)
}

func (m *model) updateView() string {
	const modalWidth = 80
	const innerWidth = modalWidth - 8

	spinner := spinnerChars[m.spinnerFrame]

	heading := func(s string, c lipgloss.Color) string {
		return lipgloss.NewStyle().Foreground(c).Bold(true).Render(s)
	}
	dim := func(s string) string {
		return lipgloss.NewStyle().Foreground(colorDim).Render(s)
	}
	divider := func() string {
		return lipgloss.NewStyle().Foreground(colorDim).
			Render(strings.Repeat("─", innerWidth))
	}
	keyHint := func(key, label string) string {
		k := lipgloss.NewStyle().
			Foreground(colorBg).
			Background(colorAccentBlue).
			Bold(true).
			Padding(0, 1).
			Render(key)
		l := lipgloss.NewStyle().Foreground(colorDim).Render(" " + label)
		return k + l
	}

	// ── Per-phase body ───────────────────────────────────────────────────
	var body string

	// baseStyle ensures every line in the JoinVertical has the background color
	// and takes up the full innerWidth, preventing "black gaps" where the
	// terminal background would otherwise bleed through.
	baseStyle := lipgloss.NewStyle().Background(colorBg).Width(innerWidth)
	emptyLine := baseStyle.Render("")

	switch m.updateStatus {
	case updateChecking:
		body = lipgloss.JoinVertical(lipgloss.Left,
			baseStyle.Render(heading(spinner+"  Checking for updates…", colorAccentBlue)),
			emptyLine,
			baseStyle.Render(dim("Querying GitHub releases for "+update.RepoOwner+"/"+update.RepoName)),
		)
	case updateInstalling:
		body = lipgloss.JoinVertical(lipgloss.Left,
			baseStyle.Render(heading(spinner+"  Downloading and installing update…", colorAccentBlue)),
			emptyLine,
			baseStyle.Render(dim("Please wait — do not close the terminal.")),
			baseStyle.Render(dim("The binary will be replaced once the download completes.")),
		)
	case updateSuccess:
		var successMsg string
		if m.updateInfo != nil && m.updateInfo.Available {
			successMsg = "DebtDrone has been updated to " +
				lipgloss.NewStyle().Foreground(colorOK).Bold(true).Render("v"+m.updateInfo.Version) +
				"\n" +
				dim("Please restart the tool to use the new version.")
		} else {
			successMsg = heading("You are already on the latest version.", colorOK)
		}
		body = lipgloss.JoinVertical(lipgloss.Left,
			baseStyle.Render(heading("✓  "+successMsg, colorOK)),
			emptyLine,
			baseStyle.Render(divider()),
			emptyLine,
			baseStyle.Render(dim("Press any key to return to the menu.")),
		)
	case updateError:
		errText := "unknown error"
		if m.updateErr != nil {
			errText = m.updateErr.Error()
		}
		body = lipgloss.JoinVertical(lipgloss.Left,
			baseStyle.Render(heading("✗  Update failed", colorError)),
			emptyLine,
			baseStyle.Render(lipgloss.NewStyle().Foreground(colorError).Width(innerWidth).Render(errText)),
			emptyLine,
			baseStyle.Render(divider()),
			emptyLine,
			baseStyle.Render(dim("Press any key to return to the menu.")),
		)
	case updatePrompt:
		currentVer := version
		if currentVer == "" || currentVer == "dev" {
			currentVer = "dev"
		}
		newVer := ""
		if m.updateInfo != nil {
			newVer = m.updateInfo.Version
		}
		versionLine := dim("Current: ") +
			lipgloss.NewStyle().Foreground(colorText).Render("v"+currentVer) +
			lipgloss.NewStyle().Foreground(colorDim).Render("  →  ") +
			dim("New: ") +
			lipgloss.NewStyle().Foreground(colorOK).Bold(true).Render("v"+newVer)

		notes := "(no release notes)"
		if m.updateInfo != nil && m.updateInfo.ReleaseNotes != "" {
			rawLines := strings.Split(strings.TrimSpace(m.updateInfo.ReleaseNotes), "\n")
			const maxNoteLines = 12
			if len(rawLines) > maxNoteLines {
				rawLines = append(rawLines[:maxNoteLines], dim("… (truncated)"))
			}
			notes = strings.Join(rawLines, "\n")
		}
		notesRendered := lipgloss.NewStyle().
			Foreground(colorText).
			Width(innerWidth).
			Render(notes)

		footer := keyHint("y", "Install update") +
			"   " +
			keyHint("n", "Skip for now")

		body = lipgloss.JoinVertical(lipgloss.Left,
			baseStyle.Render(heading("Update Available", colorAccentBlue)),
			emptyLine,
			baseStyle.Render(versionLine),
			emptyLine,
			baseStyle.Render(divider()),
			emptyLine,
			baseStyle.Render(lipgloss.NewStyle().Foreground(colorDim).Bold(true).Render("Release Notes")),
			emptyLine,
			baseStyle.Render(notesRendered),
			emptyLine,
			baseStyle.Render(divider()),
			emptyLine,
			baseStyle.Render(footer),
		)
	}

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentBlue).
		Padding(1, 3).
		Width(modalWidth).
		Background(colorBg).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
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

func (m *model) configView() string {
	const boxWidth = 104
	const innerWidth = boxWidth - 6

	const keyW = 22
	const valW = 20
	const gap = 2
	descW := innerWidth - keyW - valW - (gap * 2)

	titleStyle := lipgloss.NewStyle().
		Foreground(colorAccentBlue).
		Bold(true)

	categoryStyle := lipgloss.NewStyle().
		Foreground(colorDim).
		Bold(true)

	keyNormalStyle := lipgloss.NewStyle().
		Foreground(colorDim).
		Width(keyW)

	keySelectedStyle := lipgloss.NewStyle().
		Foreground(colorAccentBlue).
		Bold(true).
		Width(keyW)

	descStyle := lipgloss.NewStyle().
		Foreground(colorDim).
		Width(descW)

	valueBadge := func(item configItem, idx int) string {
		displayVal := item.Value
		if m.configCurrentMode == configEditing && idx == m.configCursor {
			displayVal = m.configEditBuffer + "█"
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

		content := "[ " + inner + " ]"
		if item.IsOption && idx == m.configCursor {
			content = "← [ " + inner + " ] →"
		} else if item.Type == "bool" && idx == m.configCursor {
			content = "  [ " + inner + " ]  "
		} else {
			content = "  [ " + inner + " ]  "
		}

		return lipgloss.NewStyle().
			Foreground(bracketColor).
			Width(valW + 4). // account for arrows
			Render(content)
	}

	rowBg := lipgloss.NewStyle().
		Background(colorSelectedBg).
		Width(innerWidth)

	normalRow := lipgloss.NewStyle().
		Width(innerWidth)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Settings"))
	b.WriteString("\n\n")

	var lastCategory string
	visibleRows := max(m.height-10, 4)
	end := min(m.configOffset+visibleRows, len(m.configItems))

	for i := m.configOffset; i < end; i++ {
		item := m.configItems[i]

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
		if i == m.configCursor {
			keyRendered = keySelectedStyle.Render(item.Key)
		} else {
			keyRendered = keyNormalStyle.Render(item.Key)
		}

		row := keyRendered +
			strings.Repeat(" ", gap) +
			descStyle.Render(item.Description) +
			strings.Repeat(" ", gap) +
			valueBadge(item, i)

		if i == m.configCursor {
			b.WriteString(rowBg.Render(row))
		} else {
			b.WriteString(normalRow.Render(row))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	var hints string
	hintStyle := lipgloss.NewStyle().Foreground(colorDim)
	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("#3a3f58")).Render("  ·  ")

	if m.configCurrentMode == configNavigating {
		k := func(s string) string {
			return lipgloss.NewStyle().Foreground(colorText).Render(s)
		}
		hints = hintStyle.Render(
			k("↑/↓") + hintStyle.Render(" navigate") + sep +
				k("←/→") + hintStyle.Render(" cycle") + sep +
				k("enter/space") + hintStyle.Render(" edit/toggle") + sep +
				k("esc") + hintStyle.Render(" back"),
		)
	} else {
		k := func(s string) string {
			return lipgloss.NewStyle().Foreground(colorOK).Render(s)
		}
		hints = hintStyle.Render(
			k("type") + hintStyle.Render(" to edit") + sep +
				k("enter") + hintStyle.Render(" save") + sep +
				k("esc") + hintStyle.Render(" cancel"),
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

func (m *model) historyListView() string {
	headerStyle := lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)

	const dateW = 19
	const totalW = 9
	const breakW = 28
	const gap = 2
	pathW := max(m.width-dateW-totalW-breakW-(gap*4)-2, 12)

	hDate := headerStyle.Width(dateW).Render("Date / Time")
	hPath := headerStyle.Width(pathW).Render("Scanned Path")
	hTotal := headerStyle.Width(totalW).Render("Issues")
	hBreak := headerStyle.Render("Breakdown")
	header := fmt.Sprintf("  %s  %s  %s  %s", hDate, hPath, hTotal, hBreak)
	sep := dimStyle.Render(strings.Repeat("─", m.width))

	lines := []string{header, sep}

	historyListH, _ := splitHeight(m.height)
	end := min(m.historyOffset+historyListH, len(m.historyEntries))

	for i := m.historyOffset; i < end; i++ {
		e := m.historyEntries[i]
		run := e.run

		dateStr := lipgloss.NewStyle().
			Foreground(colorFilePath).
			Width(dateW).
			Render(run.StartedAt.Format("2006-01-02 15:04:05"))

		pathStr := lipgloss.NewStyle().
			Foreground(colorText).
			Width(pathW).
			Render(truncate(e.path, pathW-1))

		totalStr := lipgloss.NewStyle().
			Foreground(colorAccentBlue).
			Bold(true).
			Width(totalW).
			Render(fmt.Sprintf("%d", run.TotalIssuesFound))

		cStr := lipgloss.NewStyle().Foreground(colorCritical).Render(fmt.Sprintf("C:%-3d", run.CriticalIssuesCount))
		hStr := lipgloss.NewStyle().Foreground(colorHigh).Render(fmt.Sprintf("H:%-3d", run.HighIssuesCount))
		mStr := lipgloss.NewStyle().Foreground(colorMedium).Render(fmt.Sprintf("M:%-3d", run.MediumIssuesCount))
		lStr := lipgloss.NewStyle().Foreground(colorLow).Render(fmt.Sprintf("L:%-3d", run.LowIssuesCount))
		breakdownStr := cStr + "  " + hStr + "  " + mStr + "  " + lStr

		row := fmt.Sprintf("  %s  %s  %s  %s", dateStr, pathStr, totalStr, breakdownStr)

		if i == m.historyCursor {
			row = lipgloss.NewStyle().
				Background(colorSelectedBg).
				Foreground(colorAccentBlue).
				Width(m.width).
				Render(row)
		} else {
			row = lipgloss.NewStyle().Width(m.width).Render(row)
		}
		lines = append(lines, row)
	}

	if len(m.historyEntries) > 0 {
		counter := fmt.Sprintf("  %d / %d", m.historyCursor+1, len(m.historyEntries))
		lines = append(lines, dimStyle.Render(counter))
	}

	return strings.Join(lines, "\n")
}

func (m *model) historyView() string {
	listPane := m.historyListView()

	const divTitle = " Past Scan Summary "
	innerW := max(m.width-len(divTitle)-4, 0)
	leftW := innerW / 2
	rightW := innerW - leftW
	divider := lipgloss.NewStyle().Foreground(colorAccentBlue).Render(
		strings.Repeat("─", leftW) +
			lipgloss.NewStyle().Foreground(colorAccentBlue).Bold(true).Render(divTitle) +
			strings.Repeat("─", rightW),
	)

	detailInner := m.historyDetail.view()
	detailPane := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentBlue).
		Width(m.width - 2).
		Render(detailInner)

	hintStyle := lipgloss.NewStyle().Foreground(colorDim)
	hints := hintStyle.Render(
		"j/k ↑↓ navigate   " +
			"J/K scroll detail   " +
			"g/G top/bottom   " +
			"enter browse results   " +
			"q quit",
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		listPane,
		divider,
		detailPane,
		hints,
	)
}
