package tui

import tea "charm.land/bubbletea/v2"

// View satisfies tea.Model for AppModel and is the single point where the
// active child's rendered string is wrapped in a Bubble Tea View with
// alt-screen mode. The detailed rendering logic lives in each child model's
// render() method; this function is intentionally minimal.
func (m *AppModel) View() tea.View {
	// A terminal below the supported size is reported once, here, rather than
	// leaving every screen to degrade into an unreadable layout of its own.
	if layoutFor(m.width, m.height).tooSmall() {
		v := tea.NewView(renderTooSmall(m.width, m.height))
		v.AltScreen = true
		return v
	}

	var body string
	switch m.activeState {
	case stateMenu:
		body = m.menu.render()
	case stateHelp:
		body = m.menu.renderHelp()
	case stateScanning, stateResults:
		body = m.scan.render()
	case stateHistory:
		body = m.history.render()
	case stateConfig:
		body = m.config.render()
	case stateUpdating:
		body = m.update.render()
	}
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}
