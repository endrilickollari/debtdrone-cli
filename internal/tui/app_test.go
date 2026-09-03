package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localhistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppModel_Routing(t *testing.T) {
	app := NewAppModel()

	msg := tea.WindowSizeMsg{Width: 100, Height: 50}
	app.Update(msg)

	if app.width != 100 || app.height != 50 {
		t.Errorf("AppModel dimensions not updated: got %dx%d, want 100x50", app.width, app.height)
	}

	if app.config.width != 100 || app.config.height != 50 {
		t.Errorf("ConfigModel dimensions not updated via propagation: got %dx%d, want 100x50", app.config.width, app.config.height)
	}

	navMsg := NavigateMsg{State: stateConfig}
	app.Update(navMsg)

	if app.activeState != stateConfig {
		t.Errorf("AppModel state not updated after NavigateMsg: got %v, want %v", app.activeState, stateConfig)
	}
}

func TestAppModelRefreshesDashboardHistoryAndRestoresFocus(t *testing.T) {
	app := NewAppModel()
	loads := 0
	app.menu = newMenuModelWithHistory("/workspace/project", func(context.Context) ([]localhistory.Record, error) {
		loads++
		return []localhistory.Record{{Repository: "project", Outcome: localhistory.OutcomeCompleted}}, nil
	})
	app.menu.focus = 2

	model, command := app.navigateTo(stateMenu)

	assert.Same(t, app, model)
	assert.Equal(t, 2, app.menu.focus)
	require.NotNil(t, command)
	message, ok := command().(recentHistoryLoadedMsg)
	require.True(t, ok)
	_, _ = app.Update(message)
	assert.Equal(t, 1, loads)
	require.Len(t, app.menu.recent, 1)
}
