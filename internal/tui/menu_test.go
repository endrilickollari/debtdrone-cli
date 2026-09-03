package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localhistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardLoadsRecentHistoryAndReopensSummary(t *testing.T) {
	completedAt := time.Date(2026, time.October, 12, 8, 30, 0, 0, time.UTC)
	records := []localhistory.Record{
		{
			ID:          "00000000-0000-0000-0000-000000000001",
			Repository:  "debtdrone-cli",
			CompletedAt: completedAt,
			Outcome:     localhistory.OutcomeCompleted,
			Summary: localhistory.Summary{
				Findings: 8, Critical: 1, High: 2, Medium: 3, Low: 2,
				TechnicalDebtHours: 6.5,
			},
		},
		{ID: "00000000-0000-0000-0000-000000000002", Repository: "api", CompletedAt: completedAt.Add(-time.Hour), Outcome: localhistory.OutcomePartial},
		{ID: "00000000-0000-0000-0000-000000000003", Repository: "web", CompletedAt: completedAt.Add(-2 * time.Hour), Outcome: localhistory.OutcomeCompleted},
		{ID: "00000000-0000-0000-0000-000000000004", Repository: "older", CompletedAt: completedAt.Add(-3 * time.Hour), Outcome: localhistory.OutcomeCompleted},
	}
	menu := newMenuModelWithHistory("/workspace/debtdrone-cli", func(context.Context) ([]localhistory.Record, error) {
		return records, nil
	})

	command := menu.Init()
	require.NotNil(t, command)
	message, ok := command().(recentHistoryLoadedMsg)
	require.True(t, ok)
	_, _ = menu.Update(message)

	require.Len(t, menu.recent, dashboardRecentLimit)
	dashboard := menu.render()
	assert.Contains(t, dashboard, "debtdrone-cli")
	assert.Contains(t, dashboard, "Oct 12 08:30 UTC")
	assert.Contains(t, dashboard, "8 findings")
	assert.Contains(t, dashboard, "C:1")

	menu.focus = len(dashboardActions)
	_, command = menu.Update(specialKeyMsg(tea.KeyEnter))
	assert.Nil(t, command)
	require.NotNil(t, menu.recentDetail)
	detail := menu.render()
	assert.Contains(t, detail, "RECENT SCAN SUMMARY")
	assert.Contains(t, detail, "6.50 hours")
	assert.Contains(t, detail, "source and finding details are never persisted")
}

func TestDashboardRendersIntentionalHistoryStates(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "first scan",
			want: []string{"No scan summaries yet.", "Choose Scan current directory"},
		},
		{
			name: "history unavailable",
			err:  errors.New("permission denied"),
			want: []string{"Recent scans unavailable", "permission denied"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			menu := newMenuModelWithHistory("/workspace/project", func(context.Context) ([]localhistory.Record, error) {
				return nil, test.err
			})
			message := menu.Init()().(recentHistoryLoadedMsg)
			_, _ = menu.Update(message)

			rendered := menu.render()
			assert.Contains(t, rendered, "Scan current directory")
			for _, expected := range test.want {
				assert.Contains(t, rendered, expected)
			}
		})
	}
}

func TestDashboardRefreshKeepsFocusVisibleAndDropsStaleHistoryOnFailure(t *testing.T) {
	menu := newMenuModelWithHistory("/workspace/current", func(context.Context) ([]localhistory.Record, error) {
		return nil, errors.New("permission denied")
	})
	_, _ = menu.Update(recentHistoryLoadedMsg{records: []localhistory.Record{{
		Repository: "stale-repository",
		Outcome:    localhistory.OutcomeCompleted,
	}}})
	menu.focus = len(dashboardActions)

	command := menu.RefreshHistory()
	reloading := menu.render()
	assert.Equal(t, len(dashboardActions), menu.focus)
	assert.Contains(t, reloading, "stale-repository")
	assert.NotContains(t, reloading, "Loading local scan summaries")

	require.NotNil(t, command)
	_, _ = menu.Update(command())
	assert.Empty(t, menu.recent)
	assert.Equal(t, 0, menu.focus)
	failed := menu.render()
	assert.Contains(t, failed, "Recent scans unavailable")
	assert.NotContains(t, failed, "stale-repository")

	_, command = menu.Update(specialKeyMsg(tea.KeyEnter))
	require.NotNil(t, command)
	_, ok := command().(StartScanMsg)
	assert.True(t, ok, "focus should return to the safe first action")
}

func TestDashboardPrimaryActionsWorkWithoutMemorizedCommands(t *testing.T) {
	currentPath := filepath.Join(t.TempDir(), "repository")
	menu := newMenuModelWithHistory(currentPath, nil)

	_, command := menu.Update(specialKeyMsg(tea.KeyEnter))
	require.NotNil(t, command)
	scan, ok := command().(StartScanMsg)
	require.True(t, ok)
	absolutePath, err := filepath.Abs(currentPath)
	require.NoError(t, err)
	assert.Equal(t, absolutePath, scan.Path)

	menu.focus = 1
	_, command = menu.Update(specialKeyMsg(tea.KeyEnter))
	assert.Nil(t, command)
	assert.True(t, menu.inputActive)
	assert.Equal(t, "/scan ", menu.input)
	assert.Contains(t, menu.render(), "COMMAND PALETTE")

	menu.closeInput()
	menu.focus = 3
	_, command = menu.Update(specialKeyMsg(tea.KeyEnter))
	require.NotNil(t, command)
	navigate, ok := command().(NavigateMsg)
	require.True(t, ok)
	assert.Equal(t, stateConfig, navigate.State)
}

func TestDashboardVisibleShortcutsRouteToTheirActions(t *testing.T) {
	for index, action := range dashboardActions {
		t.Run(action.label, func(t *testing.T) {
			menu := newMenuModelWithHistory(t.TempDir(), nil)

			_, command := menu.Update(keyMsg(rune(action.shortcut[0])))

			assert.Equal(t, index, menu.focus)
			switch action.kind {
			case dashboardChoosePath:
				assert.Nil(t, command)
				assert.True(t, menu.inputActive)
			case dashboardScanCurrent, dashboardHistory, dashboardSettings, dashboardHelp, dashboardQuit:
				require.NotNil(t, command)
			}
		})
	}
}

func TestDashboardDocumentsEveryActionAndPreservesFocus(t *testing.T) {
	menu := newMenuModelWithHistory("/workspace/project", nil)
	menu.focus = 4
	menu.Reset()
	assert.Equal(t, 4, menu.focus)

	help := menu.renderHelp()
	for _, action := range dashboardActions {
		assert.NotEmpty(t, action.shortcut)
		assert.Contains(t, help, action.label)
	}
	for _, guidance := range []string{"Reopen the newest", "Move focus", "Open the command palette"} {
		assert.Contains(t, help, guidance)
	}

	menu.openCommandInput()
	assert.Equal(t, "/", menu.input)
	assert.True(t, menu.inputActive)
	assert.True(t, strings.Contains(menu.render(), "COMMAND PALETTE"))
}
