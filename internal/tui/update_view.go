package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/update"
)

type updatePhase int

const (
	updateChecking updatePhase = iota
	updatePrompt
	updateInstalling
	updateSuccess
	updateError
)

type checkUpdateMsg struct {
	info *update.UpdateInfo
	err  error
}

type updateCompleteMsg struct{ err error }

// UpdateModel manages the auto-update screen.
type UpdateModel struct {
	phase        updatePhase
	info         *update.UpdateInfo
	err          error
	spinnerFrame int
	width        int
	height       int
}

func newUpdateModel() *UpdateModel {
	return &UpdateModel{width: 120, height: 40}
}

// Start initiates the update check.
func (m *UpdateModel) Start() tea.Cmd {
	m.phase = updateChecking
	m.info = nil
	m.err = nil
	m.spinnerFrame = 0
	return tea.Batch(startUpdateCheck, tickCmd())
}

func (m *UpdateModel) Init() tea.Cmd { return nil }

func (m *UpdateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		if m.phase == updateChecking || m.phase == updateInstalling {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerChars)
			return m, tickCmd()
		}
		return m, nil

	case checkUpdateMsg:
		if msg.err != nil {
			m.err = msg.err
			m.phase = updateError
		} else if msg.info == nil || !msg.info.Available {
			m.info = msg.info
			m.phase = updateSuccess
		} else {
			m.info = msg.info
			m.phase = updatePrompt
		}
		return m, nil

	case updateCompleteMsg:
		if msg.err != nil {
			m.err = msg.err
			m.phase = updateError
		} else {
			m.phase = updateSuccess
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg.String())
	}
	return m, nil
}

func (m *UpdateModel) handleKey(str string) (tea.Model, tea.Cmd) {
	switch m.phase {
	case updateChecking, updateInstalling:
		// Busy — ignore all input except ctrl+c (handled by AppModel).
		return m, nil

	case updatePrompt:
		switch str {
		case "y":
			m.phase = updateInstalling
			m.spinnerFrame = 0
			return m, tea.Batch(performUpdateCmd, tickCmd())
		case "n", "q", "esc":
			m.info, m.err = nil, nil
			return m, func() tea.Msg { return NavigateMsg{State: stateMenu} }
		}
		return m, nil

	case updateSuccess, updateError:
		m.info, m.err = nil, nil
		return m, func() tea.Msg { return NavigateMsg{State: stateMenu} }
	}
	return m, nil
}

func (m *UpdateModel) View() tea.View {
	return tea.NewView(m.render())
}

func (m *UpdateModel) render() string {
	modalWidth := min(80, max(m.width-2, 20))
	innerWidth := max(modalWidth-8, 1)

	spinner := spinnerChars[m.spinnerFrame]
	if reducedMotion() {
		spinner = "*"
	}

	heading := func(s string, c lipgloss.Color) string {
		return lipgloss.NewStyle().Foreground(c).Bold(true).Render(s)
	}
	dim := func(s string) string {
		return lipgloss.NewStyle().Foreground(colorDim).Render(s)
	}
	divider := func() string {
		return lipgloss.NewStyle().Foreground(colorDim).Render(strings.Repeat("─", innerWidth))
	}
	keyHint := func(key, label string) string {
		k := lipgloss.NewStyle().Foreground(colorBg).Background(colorAccentBlue).Bold(true).Padding(0, 1).Render(key)
		l := lipgloss.NewStyle().Foreground(colorDim).Render(" " + label)
		return k + l
	}

	baseStyle := lipgloss.NewStyle().Background(colorBg).Width(innerWidth)
	line := func(value string) string { return baseStyle.Render(truncate(value, innerWidth)) }
	styledLine := func(value string, colour lipgloss.Color, bold bool) string {
		style := lipgloss.NewStyle().Foreground(colour).Bold(bold)
		return baseStyle.Render(style.Render(truncate(value, innerWidth)))
	}
	emptyLine := line("")
	availableBodyHeight := max(m.height-4, 1) // border and vertical padding

	var body string
	switch m.phase {
	case updateChecking:
		body = lipgloss.JoinVertical(lipgloss.Left,
			styledLine(spinner+"  Checking for updates…", colorAccentBlue, true),
			emptyLine,
			styledLine("Querying GitHub releases for "+update.RepoOwner+"/"+update.RepoName, colorDim, false),
		)

	case updateInstalling:
		body = lipgloss.JoinVertical(lipgloss.Left,
			styledLine(spinner+"  Downloading and installing update…", colorAccentBlue, true),
			emptyLine,
			styledLine("Please wait — do not close the terminal.", colorDim, false),
			styledLine("The binary will be replaced once the download completes.", colorDim, false),
		)

	case updateSuccess:
		var successLines []string
		if m.info != nil && m.info.Available {
			successLines = []string{
				styledLine("✓  DebtDrone has been updated to v"+m.info.Version, colorOK, true),
				styledLine("Please restart the tool to use the new version.", colorDim, false),
			}
		} else {
			successLines = []string{styledLine("✓  You are already on the latest version.", colorOK, true)}
		}
		successLines = append(successLines,
			emptyLine,
			baseStyle.Render(divider()),
			emptyLine,
			styledLine("Press any key to return to the menu.", colorDim, false),
		)
		body = lipgloss.JoinVertical(lipgloss.Left, successLines...)

	case updateError:
		errText := "unknown error"
		if m.err != nil {
			errText = m.err.Error()
		}
		errorLines := strings.Split(lipgloss.NewStyle().Foreground(colorError).Width(innerWidth).Render(errText), "\n")
		maxErrorLines := max(availableBodyHeight-4, 1)
		if len(errorLines) > maxErrorLines {
			errorLines = append(errorLines[:maxErrorLines-1], dim("… (truncated)"))
		}
		body = lipgloss.JoinVertical(lipgloss.Left,
			baseStyle.Render(heading("✗  Update failed", colorError)),
			strings.Join(errorLines, "\n"),
			baseStyle.Render(divider()),
			line("Press any key to return to the menu."),
		)

	case updatePrompt:
		currentVer := version
		if currentVer == "" || currentVer == "dev" {
			currentVer = "dev"
		}
		newVer := ""
		if m.info != nil {
			newVer = m.info.Version
		}
		versionLine := dim("Current: ") +
			lipgloss.NewStyle().Foreground(colorText).Render("v"+truncate(currentVer, 12)) +
			lipgloss.NewStyle().Foreground(colorDim).Render("  →  ") +
			dim("New: ") +
			lipgloss.NewStyle().Foreground(colorOK).Bold(true).Render("v"+truncate(newVer, 12))

		notes := "(no release notes)"
		if m.info != nil && m.info.ReleaseNotes != "" {
			notes = strings.TrimSpace(m.info.ReleaseNotes)
		}
		noteLines := strings.Split(lipgloss.NewStyle().Foreground(colorText).Width(innerWidth).Render(notes), "\n")
		maxNoteLines := max(availableBodyHeight-6, 1)
		if len(noteLines) > maxNoteLines {
			noteLines = append(noteLines[:maxNoteLines-1], dim("… (truncated)"))
		}
		notesRendered := strings.Join(noteLines, "\n")

		footer := keyHint("y", "Install update") + "   " + keyHint("n", "Skip for now")

		body = lipgloss.JoinVertical(lipgloss.Left,
			baseStyle.Render(heading("Update Available", colorAccentBlue)),
			baseStyle.Render(versionLine),
			baseStyle.Render(divider()),
			baseStyle.Render(lipgloss.NewStyle().Foreground(colorDim).Bold(true).Render("Release Notes")),
			baseStyle.Render(notesRendered),
			baseStyle.Render(divider()),
			baseStyle.Render(footer),
		)
	}
	bodyLines := strings.Split(body, "\n")
	if len(bodyLines) > availableBodyHeight {
		bodyLines = bodyLines[:availableBodyHeight]
	}
	body = strings.Join(bodyLines, "\n")

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentBlue).
		Padding(1, 3).
		Width(modalWidth).
		Background(colorBg).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}

func startUpdateCheck() tea.Msg {
	info, err := update.CheckForUpdate(context.Background(), version)
	if err != nil {
		return checkUpdateMsg{err: err}
	}
	return checkUpdateMsg{info: info}
}

func performUpdateCmd() tea.Msg {
	err := update.PerformUpdate(context.Background())
	return updateCompleteMsg{err: err}
}
