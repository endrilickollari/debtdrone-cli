package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/internal/update"
)

// ─────────────────────────────────────────────────────────────────────────────
// Domain types
// ─────────────────────────────────────────────────────────────────────────────

type updatePhase int

const (
	updateChecking  updatePhase = iota
	updatePrompt               // update available; waiting for y/n
	updateInstalling
	updateSuccess
	updateError
)

// checkUpdateMsg is the internal result of the update-check goroutine.
type checkUpdateMsg struct {
	info *update.UpdateInfo
	err  error
}

// updateCompleteMsg is the internal result of the install goroutine.
type updateCompleteMsg struct{ err error }

// ─────────────────────────────────────────────────────────────────────────────
// UpdateModel
// ─────────────────────────────────────────────────────────────────────────────

// UpdateModel encapsulates the /update screen and its async operations.
//
// Navigation contract:
//   - On success/error/skip: returns NavigateMsg{State: stateMenu}
//   - Never mutates AppModel directly.
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

// Start is called by AppModel when it intercepts StartUpdateMsg.
// It resets state and returns the commands to kick off the update check.
func (m *UpdateModel) Start() tea.Cmd {
	m.phase = updateChecking
	m.info = nil
	m.err = nil
	m.spinnerFrame = 0
	return tea.Batch(startUpdateCheck, tickCmd())
}

// Init satisfies tea.Model.
func (m *UpdateModel) Init() tea.Cmd { return nil }

// Update handles both the async result messages and keyboard input.
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

// View satisfies tea.Model.
func (m *UpdateModel) View() tea.View {
	return tea.NewView(m.render())
}

// render produces the update-screen modal.
func (m *UpdateModel) render() string {
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
		return lipgloss.NewStyle().Foreground(colorDim).Render(strings.Repeat("─", innerWidth))
	}
	keyHint := func(key, label string) string {
		k := lipgloss.NewStyle().Foreground(colorBg).Background(colorAccentBlue).Bold(true).Padding(0, 1).Render(key)
		l := lipgloss.NewStyle().Foreground(colorDim).Render(" " + label)
		return k + l
	}

	baseStyle := lipgloss.NewStyle().Background(colorBg).Width(innerWidth)
	emptyLine := baseStyle.Render("")

	var body string
	switch m.phase {
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
		if m.info != nil && m.info.Available {
			successMsg = "DebtDrone has been updated to " +
				lipgloss.NewStyle().Foreground(colorOK).Bold(true).Render("v"+m.info.Version) +
				"\n" + dim("Please restart the tool to use the new version.")
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
		if m.err != nil {
			errText = m.err.Error()
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
		if m.info != nil {
			newVer = m.info.Version
		}
		versionLine := dim("Current: ") +
			lipgloss.NewStyle().Foreground(colorText).Render("v"+currentVer) +
			lipgloss.NewStyle().Foreground(colorDim).Render("  →  ") +
			dim("New: ") +
			lipgloss.NewStyle().Foreground(colorOK).Bold(true).Render("v"+newVer)

		notes := "(no release notes)"
		if m.info != nil && m.info.ReleaseNotes != "" {
			rawLines := strings.Split(strings.TrimSpace(m.info.ReleaseNotes), "\n")
			const maxNoteLines = 12
			if len(rawLines) > maxNoteLines {
				rawLines = append(rawLines[:maxNoteLines], dim("… (truncated)"))
			}
			notes = strings.Join(rawLines, "\n")
		}
		notesRendered := lipgloss.NewStyle().Foreground(colorText).Width(innerWidth).Render(notes)

		footer := keyHint("y", "Install update") + "   " + keyHint("n", "Skip for now")

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

// ─────────────────────────────────────────────────────────────────────────────
// Async helpers (pure functions; no model state)
// ─────────────────────────────────────────────────────────────────────────────

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
