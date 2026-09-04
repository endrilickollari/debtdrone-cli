package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resizeTo(width, height int) tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: width, Height: height}
}

func TestResizingResultsKeepsTheSelectedFinding(t *testing.T) {
	model := resultsScreen(t, wideWidth, wideHeight, fixtureIssues())
	model.handleKey("j")
	model.handleKey("j")
	selected := selectedFingerprint(t, model)

	model.Update(resizeTo(narrowWidth, narrowHeight))

	assert.Equal(t, selected, selectedFingerprint(t, model), "a resize is not a navigation event")
	assert.Equal(t, narrowWidth, model.width)
	assert.LessOrEqual(t, model.list.offset, max(len(model.list.items)-model.list.height, 0))
	assert.GreaterOrEqual(t, model.list.cursor, model.list.offset, "the cursor stays inside the new scroll window")
}

func TestResizingResultsKeepsActiveFiltersAndSearchText(t *testing.T) {
	model := resultsScreen(t, wideWidth, wideHeight, fixtureIssues())
	model.handleKey("2") // high only
	model.handleKey("/")
	model.handleKey("c")
	require.True(t, model.searching)
	require.Equal(t, "c", model.filter.query)

	model.Update(resizeTo(narrowWidth, narrowHeight))

	assert.True(t, model.searching, "resizing does not close the search prompt")
	assert.Equal(t, "c", model.filter.query)
	assert.True(t, model.filter.active())
}

func TestResizingDuringAScanLeavesTheRunUntouched(t *testing.T) {
	model := scanningScreen(t, wideWidth, wideHeight, "ComplexityAnalyzer", 2, 5)
	runID := model.runID

	model.Update(resizeTo(narrowWidth, narrowHeight))

	assert.Equal(t, scanRunning, model.phase)
	assert.Equal(t, runID, model.runID, "a resize must not supersede the active scan")
	assert.NotNil(t, model.cancel)
	assert.Equal(t, 2, model.completedAnalyzers)
	assert.Equal(t, "ComplexityAnalyzer", model.stage)
}

func TestExtremeTerminalSizesRenderWithoutPanicking(t *testing.T) {
	// Degenerate sizes are reachable in practice: terminals report 0x0 while a
	// window is being created, and a split pane can be a couple of cells wide.
	sizes := []struct{ width, height int }{
		{0, 0}, {1, 1}, {20, 5}, {narrowWidth, narrowHeight}, {400, 120},
	}

	for _, size := range sizes {
		t.Run(sizeName(size.width, size.height), func(t *testing.T) {
			results := resultsScreen(t, wideWidth, wideHeight, fixtureIssues())
			scanning := scanningScreen(t, wideWidth, wideHeight, "ComplexityAnalyzer", 2, 5)
			dashboard := dashboardScreen(t, wideWidth, wideHeight, fixtureRecords(), nil)
			history := historyScreen(t, wideWidth, wideHeight)
			config := configScreen(t, wideWidth, wideHeight)

			assert.NotPanics(t, func() {
				results.Update(resizeTo(size.width, size.height))
				_ = results.render()

				scanning.Update(resizeTo(size.width, size.height))
				_ = scanning.render()

				dashboard.Update(resizeTo(size.width, size.height))
				_ = dashboard.render()
				_ = dashboard.renderHelp()

				history.Update(resizeTo(size.width, size.height))
				_ = history.render()

				config.Update(resizeTo(size.width, size.height))
				_ = config.render()
			})
		})
	}
}

func TestSupportedNarrowLayoutsFitTheTerminal(t *testing.T) {
	screens := map[string]string{
		"dashboard": dashboardScreen(t, narrowWidth, narrowHeight, fixtureRecords(), nil).render(),
		"scanning":  scanningScreen(t, narrowWidth, narrowHeight, "ComplexityAnalyzer", 2, 5).render(),
		"results":   resultsScreen(t, narrowWidth, narrowHeight, fixtureIssues()).render(),
	}

	for name, screen := range screens {
		t.Run(name, func(t *testing.T) {
			assertScreenFits(t, screen, narrowWidth, narrowHeight)
		})
	}

	assert.Contains(t, screens["dashboard"], "quit")
	assert.Contains(t, screens["scanning"], "cancel")
	assert.Contains(t, screens["scanning"], "ctrl+c")
	assert.Contains(t, screens["results"], "/ search")
	assert.Contains(t, screens["results"], "quit")
}

func TestEmptyAndFailedResultsSurviveAResize(t *testing.T) {
	clean := resultsScreen(t, wideWidth, wideHeight, nil)
	assert.NotPanics(t, func() {
		clean.Update(resizeTo(narrowWidth, narrowHeight))
		assertScreenFits(t, clean.render(), narrowWidth, narrowHeight)
	})

	failed := scanningScreen(t, wideWidth, wideHeight, "SecurityAnalyzer", 1, 5)
	failed.Update(scanCompleteMsg{runID: failed.runID, path: fixtureRepositoryPath, err: errTrivyUnavailable})
	assert.NotPanics(t, func() {
		failed.Update(resizeTo(narrowWidth, narrowHeight))
		assertScreenFits(t, failed.render(), narrowWidth, narrowHeight)
	})
	assert.ErrorIs(t, failed.err, errTrivyUnavailable, "the failure survives the resize")
}

func TestApplicationBroadcastsResizeToEveryScreen(t *testing.T) {
	app := NewConfiguredAppModel(localconfig.Defaults())

	app.Update(resizeTo(narrowWidth, narrowHeight))

	assert.Equal(t, narrowWidth, app.width)
	assert.Equal(t, narrowHeight, app.height)
	// Every child renders at the new size, so switching screens after a resize
	// never shows a pane laid out for the previous terminal.
	assert.Equal(t, narrowWidth, app.menu.width)
	assert.Equal(t, narrowWidth, app.scan.width)
	assert.Equal(t, narrowWidth, app.history.width)
	assert.Equal(t, narrowWidth, app.config.width)
}

func sizeName(width, height int) string {
	return fmt.Sprintf("%dx%d", width, height)
}

func assertScreenFits(t *testing.T, screen string, width, height int) {
	t.Helper()

	lines := strings.Split(strings.TrimRight(screen, "\n"), "\n")
	assert.LessOrEqual(t, len(lines), height, "screen is taller than the terminal")
	for index, line := range lines {
		assert.LessOrEqualf(t, lipgloss.Width(line), width, "line %d is wider than the terminal", index+1)
	}
}
