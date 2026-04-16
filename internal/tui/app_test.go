package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAppModel_Routing(t *testing.T) {
	app := NewAppModel()

	// Test Case 1: WindowSize propagation
	// We expect the AppModel to save dimensions and pass them to all children.
	msg := tea.WindowSizeMsg{Width: 100, Height: 50}
	app.Update(msg)

	if app.width != 100 || app.height != 50 {
		t.Errorf("AppModel dimensions not updated: got %dx%d, want 100x50", app.width, app.height)
	}

	// Verify propagation to a sample child (ConfigModel)
	if app.config.width != 100 || app.config.height != 50 {
		t.Errorf("ConfigModel dimensions not updated via propagation: got %dx%d, want 100x50", app.config.width, app.config.height)
	}

	// Test Case 2: Navigation via NavigateMsg
	// This tests the router logic that intercepts NavigateMsg to switch screens.
	navMsg := NavigateMsg{State: stateConfig}
	app.Update(navMsg)

	if app.activeState != stateConfig {
		t.Errorf("AppModel state not updated after NavigateMsg: got %v, want %v", app.activeState, stateConfig)
	}
}
