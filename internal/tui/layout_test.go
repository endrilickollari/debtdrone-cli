package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	updatecheck "github.com/endrilickollari/debtdrone-cli/v2/internal/update"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncateMeasuresDisplayWidthAndNeverSplitsARune(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		want     string
	}{
		{"fits untouched", "main.go", 20, "main.go"},
		{"exact fit", "main.go", 7, "main.go"},
		// Counted in bytes these overflow and would be cut; counted in columns
		// they fit exactly and must be left alone.
		{"accented value fits in columns but not in bytes", "café", 4, "café"},
		{"full-width value fits in columns but not in bytes", "项目", 4, "项目"},
		{"ascii is cut with an ellipsis", "internal/routing/handler.go", 12, "internal/ro…"},
		{"accented characters count as one column", "café-service-name", 10, "café-serv…"},
		{"full-width characters count as two columns", "项目网关服务", 7, "项目网…"},
		{"a single column leaves only the marker", "anything", 1, "…"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := truncate(test.input, test.maxWidth)
			assert.Equal(t, test.want, got)
			assert.True(t, utf8ValidString(got), "truncation produced invalid UTF-8: %q", got)
			assert.LessOrEqual(t, runewidth.StringWidth(got), test.maxWidth,
				"truncated value is wider than the space it was given")
		})
	}
}

func TestSeverityAndFocusAreLegibleWithoutColour(t *testing.T) {
	withColorProfile(t, termenv.Ascii)

	model := resultsScreen(t, wideWidth, wideHeight, fixtureIssues())
	model.handleKey("j") // select the second finding
	rendered := model.render()

	require.NotContains(t, rendered, "\x1b", "the ascii profile must emit no styling at all")

	// Severity is spelled out, so it does not depend on the row's colour.
	for _, severity := range []string{"critical", "high", "medium", "low"} {
		assert.Contains(t, rendered, severity)
	}

	// Exactly one row carries the focus marker, and it is the selected one.
	var marked []string
	for _, line := range strings.Split(rendered, "\n") {
		if strings.HasPrefix(line, "› ") {
			marked = append(marked, line)
		}
	}
	require.Len(t, marked, 1, "exactly one row is marked as focused")
	assert.Contains(t, marked[0], "handler.go", "the marker follows the selection")
}

func TestFocusHighlightSurvivesTheStylingInsideARow(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)

	list := newIssueList(fixtureIssues(), wideWidth, 10)
	selectedRow := ""
	for _, line := range strings.Split(list.view(), "\n") {
		if strings.Contains(line, "main.go") {
			selectedRow = line
		}
	}
	require.NotEmpty(t, selectedRow)

	// Every styled segment must re-assert the highlight. A single background
	// wrapped around the finished row would be cancelled by the first inner
	// reset, leaving the highlight on the padding alone.
	background := "48;2;30;42;64"
	segments := strings.Count(selectedRow, "\x1b[0m")
	assert.GreaterOrEqual(t, strings.Count(selectedRow, background), segments-1,
		"the selection highlight is dropped partway across the row")
}

func TestDashboardRecentRowsMarkFocusWithoutColour(t *testing.T) {
	withColorProfile(t, termenv.Ascii)

	menu := dashboardScreen(t, wideWidth, wideHeight, fixtureRecords(), nil)
	// Move focus past the primary actions onto the first recent scan.
	for range dashboardActions {
		menu.Update(keyMsg('j'))
	}
	rendered := menu.render()

	require.NotContains(t, rendered, "\x1b")
	assert.Contains(t, rendered, "› api-gateway", "the focused recent scan is marked in text")
}

func TestEverySelectableViewMarksFocusWithoutColour(t *testing.T) {
	withColorProfile(t, termenv.Ascii)

	history := historyScreen(t, wideWidth, wideHeight)
	assert.Contains(t, history.render(), "› 2026-09-04", "history selection needs a textual marker")

	config := configScreen(t, wideWidth, wideHeight)
	config.cursor = 1 // a boolean row has no option arrows to imply selection
	assert.Contains(t, config.render(), "› Auto-Update Checks", "configuration selection needs a textual marker")

	palette := dashboardScreen(t, wideWidth, wideHeight, fixtureRecords(), nil)
	palette.openCommandInput()
	palette.handleInputKey("tab")
	assert.Contains(t, palette.render(), "› /scan", "palette selection needs a textual marker")
}

func TestShortDashboardAlwaysShowsTheFocusedItem(t *testing.T) {
	withColorProfile(t, termenv.Ascii)
	menu := dashboardScreen(t, minimumWidth, minimumHeight, fixtureRecords(), nil)

	menu.focus = len(dashboardActions)
	rendered := menu.render()
	assert.Contains(t, rendered, "› api-gateway", "a focused recent scan must remain visible")

	menu.handleDashboardKey("k")
	rendered = menu.render()
	assert.Contains(t, rendered, "› Quit DebtDrone", "crossing back into actions must reveal the focused action")
}

func TestHelpCanReachEveryLineInAShortTerminal(t *testing.T) {
	withColorProfile(t, termenv.Ascii)
	menu := dashboardScreen(t, minimumWidth, minimumHeight, fixtureRecords(), nil)
	menu.ShowHelp()

	firstPage := menu.renderHelp()
	assert.Contains(t, firstPage, "q/esc back", "the exit hint stays pinned")
	assert.NotContains(t, firstPage, "/quit", "the fixture must require scrolling")

	menu.Update(keyMsg('G'))
	lastPage := menu.renderHelp()
	assert.Contains(t, lastPage, "/quit", "end reveals the final command")
	assert.Contains(t, lastPage, "q/esc back", "the exit hint remains visible after scrolling")
}

func TestShortRecentDetailCanReachItsFinalMetadata(t *testing.T) {
	withColorProfile(t, termenv.Ascii)
	menu := dashboardScreen(t, minimumWidth, minimumHeight, fixtureRecords(), nil)
	menu.openRecent(0)

	assert.NotContains(t, menu.renderRecentDetail(), "Only summary metadata",
		"the fixture must require scrolling")
	menu.handleRecentDetailKey("G")
	rendered := menu.renderRecentDetail()
	assert.Contains(t, rendered, "Only summary metadata")
	assert.Contains(t, rendered, "esc/enter dashboard", "actions stay pinned while content scrolls")
}

func TestShortPaletteKeepsTheSelectedSuggestionVisible(t *testing.T) {
	withColorProfile(t, termenv.Ascii)
	menu := dashboardScreen(t, minimumWidth, minimumHeight, fixtureRecords(), nil)
	menu.openCommandInput()
	menu.selectedSuggestion = len(menu.suggestions) - 1

	rendered := menu.renderCommandPalette()
	assert.Contains(t, rendered, "› /quit", "the suggestion window follows selection")
	assert.Contains(t, rendered, "esc back", "palette exit remains visible")
}

func TestHelpTruncationNeverCutsANSISequences(t *testing.T) {
	withColorProfile(t, termenv.TrueColor)
	menu := dashboardScreen(t, minimumWidth, minimumHeight, fixtureRecords(), nil)
	menu.ShowHelp()

	assert.True(t, hasValidANSISequences(menu.renderHelp()),
		"help must truncate plain values before applying terminal styles")
}

func TestShortUpdatePromptKeepsBothChoicesVisible(t *testing.T) {
	withColorProfile(t, termenv.Ascii)
	model := newUpdateModel()
	model.width, model.height, model.phase = minimumWidth, minimumHeight, updatePrompt
	model.info = &updatecheck.UpdateInfo{Available: true, Version: "2.1.0",
		ReleaseNotes: strings.Repeat("A release note that wraps. ", 30)}

	rendered := model.render()
	assert.Contains(t, rendered, "Install update")
	assert.Contains(t, rendered, "Skip for now")
	assertFitsWidth(t, "update prompt", rendered, minimumWidth)
	assert.LessOrEqual(t, lipgloss.Height(rendered), minimumHeight)
}

// TestScreensNeverOverflowTheirTerminal is the layout guarantee: at every
// supported size, no rendered line may be wider than the terminal. A line that
// overflows wraps in a real terminal and destroys the layout below it.
func TestScreensNeverOverflowTheirTerminal(t *testing.T) {
	sizes := []struct{ width, height int }{
		{minimumWidth, minimumHeight},
		{narrowWidth, narrowHeight},
		{80, 24},
		{compactWidth, 30},
		{wideWidth, wideHeight},
		{200, 60},
	}

	for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.Ascii} {
		for _, size := range sizes {
			t.Run(sizeName(size.width, size.height)+"/"+profileName(profile), func(t *testing.T) {
				withColorProfile(t, profile)

				screens := responsiveScreenFixtures(t, size.width, size.height)

				for name, rendered := range screens {
					assertFitsWidth(t, name, rendered, size.width)
					// Height matters as much as width: rows past the bottom
					// edge cannot be read, however well they are laid out.
					assert.LessOrEqual(t, lipgloss.Height(rendered), size.height,
						"%s is taller than its %d row terminal", name, size.height)
				}
			})
		}
	}
}

func responsiveScreenFixtures(t *testing.T, width, height int) map[string]string {
	t.Helper()

	records := fixtureRecords()
	records[0].Repository = strings.Repeat("very-long-repository-name-", 5)
	dashboard := dashboardScreen(t, width, height, records, nil)
	dashboard.currentPath = "/workspace/" + strings.Repeat("very-long-directory-name-", 5)
	dashboardRecent := dashboardScreen(t, width, height, records, nil)
	dashboardRecent.focus = len(dashboardActions)

	palette := dashboardScreen(t, width, height, records, nil)
	palette.openCommandInput()
	palette.input = "/scan /" + strings.Repeat("deeply-nested-repository-path/", 8)
	palette.cursorPos = len(palette.input)
	palette.selectedSuggestion = len(palette.suggestions) - 1

	recentDetail := dashboardScreen(t, width, height, records, nil)
	recentDetail.openRecent(0)

	help := dashboardScreen(t, width, height, fixtureRecords(), nil)
	help.ShowHelp()
	help.helpOffset = 1 << 20

	noMatches := resultsScreen(t, width, height, fixtureIssues())
	noMatches.filter.query = "a query that deliberately matches no finding"
	noMatches.applyFilters()
	noMatches.resizePanes()

	searching := resultsScreen(t, width, height, fixtureIssues())
	searching.searching = true
	searching.filter.query = strings.Repeat("very-long-search-term-", 8)
	searching.applyFilters()
	searching.resizePanes()

	status := resultsScreen(t, width, height, fixtureIssues())
	status.status = strings.Repeat("export destination is deeply nested / ", 5)
	status.resizePanes()

	partial := resultsScreen(t, width, height, fixtureIssues())
	partial.warning = errors.New(strings.Repeat("analyzer unavailable; ", 8))
	partial.resizePanes()

	failure := scanningScreen(t, width, height, "SecurityAnalyzer", 1, 5)
	failure.Update(scanCompleteMsg{runID: failure.runID, path: fixtureRepositoryPath,
		err: errors.New(strings.Repeat("scanner process failed with detailed context; ", 8))})

	jsonResults := resultsScreen(t, width, height, fixtureIssues())
	jsonResults.outputFormat = "json"

	updateModel := func(phase updatePhase) *UpdateModel {
		model := newUpdateModel()
		model.width, model.height, model.phase = width, height, phase
		model.info = &updatecheck.UpdateInfo{
			Available: true,
			Version:   "2.0.0-responsive-terminal-layout",
			ReleaseNotes: strings.Repeat(
				"A long release-note paragraph must stay inside the modal at every supported size. ", 12),
		}
		model.err = errors.New(strings.Repeat("update endpoint returned a detailed error; ", 12))
		return model
	}

	return map[string]string{
		"dashboard":                dashboard.render(),
		"dashboard-focused-recent": dashboardRecent.render(),
		"dashboard-empty":          dashboardScreen(t, width, height, nil, nil).render(),
		"command-palette":          palette.render(),
		"recent-detail":            recentDetail.render(),
		"help-last-page":           help.renderHelp(),
		"scanning":                 scanningScreen(t, width, height, "ComplexityAnalyzer", 2, 5).render(),
		"results":                  resultsScreen(t, width, height, fixtureIssues()).render(),
		"results-no-matches":       noMatches.render(),
		"results-searching":        searching.render(),
		"results-status":           status.render(),
		"results-partial":          partial.render(),
		"results-json":             jsonResults.render(),
		"scan-failure":             failure.render(),
		"clean":                    resultsScreen(t, width, height, nil).render(),
		"history":                  historyScreen(t, width, height).render(),
		"config":                   configScreen(t, width, height).render(),
		"update-checking":          updateModel(updateChecking).render(),
		"update-prompt":            updateModel(updatePrompt).render(),
		"update-installing":        updateModel(updateInstalling).render(),
		"update-success":           updateModel(updateSuccess).render(),
		"update-error":             updateModel(updateError).render(),
	}
}

func TestLongPathsAndMessagesStayInspectable(t *testing.T) {
	withColorProfile(t, termenv.Ascii)

	path := "/Users/dev/workspace/platform/api-gateway/internal/routing/middleware/verify_bearer_token_handler.go"
	message := "a very long finding message that will certainly not fit inside the message column of the findings table"
	line := 128
	issue := models.TechnicalDebtIssue{
		FilePath: path, LineNumber: &line, Severity: "critical",
		Category: "security", Message: message, ToolName: "trivy",
	}

	detail := formatIssueDetail(&issue, 60)

	// Nothing is lost: the wrapped detail still contains the whole value, so a
	// path the table had to shorten can always be read in full.
	assert.Equal(t, path, joinWrapped(detail, path))
	assert.Equal(t, message, joinWrapped(detail, message))

	// Continuation lines are indented to the value column rather than resuming
	// under the labels.
	lines := strings.Split(detail, "\n")
	for index, current := range lines {
		if strings.HasPrefix(current, "Full Path") && index+1 < len(lines) {
			assert.True(t, strings.HasPrefix(lines[index+1], "    "),
				"a wrapped path must not resume in the label column: %q", lines[index+1])
			break
		}
	}
}

func TestTerminalsBelowTheSupportedSizeSaySo(t *testing.T) {
	withColorProfile(t, termenv.Ascii)

	app := NewConfiguredAppModel(localconfigDefaults())
	app.Update(resizeTo(minimumWidth-1, minimumHeight))
	rendered := app.View().Content

	assert.Contains(t, rendered, "Terminal too small")
	assert.Contains(t, rendered, "59×16", "the message reports the actual size")
	assert.Contains(t, rendered, "60×16", "and the size that is required")

	// At the supported size the interface renders normally again.
	app.Update(resizeTo(minimumWidth, minimumHeight))
	assert.NotContains(t, app.View().Content, "Terminal too small")
}

func TestReducedMotionReplacesTheSpinnerWhenColourIsUnavailable(t *testing.T) {
	withColorProfile(t, termenv.Ascii)
	still := scanningScreen(t, wideWidth, wideHeight, "ComplexityAnalyzer", 2, 5).render()
	for _, frame := range spinnerChars {
		assert.NotContains(t, still, frame, "an unstyled terminal shows no animated frame")
	}
	assert.Contains(t, still, "Analyzing Repository")
	assert.Contains(t, still, "2/5 analyzers", "progress is still reported")

	withColorProfile(t, termenv.TrueColor)
	animated := scanningScreen(t, wideWidth, wideHeight, "ComplexityAnalyzer", 2, 5).render()
	assert.Contains(t, animated, spinnerChars[0])
}

func TestCompactBreakpointStacksTheDashboardPanels(t *testing.T) {
	withColorProfile(t, termenv.Ascii)

	compact := dashboardScreen(t, compactWidth-1, 30, fixtureRecords(), nil).render()
	wide := dashboardScreen(t, compactWidth+20, 30, fixtureRecords(), nil).render()

	// Two panels sharing a row open with two top-left corners on one line;
	// stacked panels open one corner per line.
	assert.Equal(t, 0, linesWithTwoPanelCorners(compact),
		"below the breakpoint the panels stack, one per row")
	assert.Equal(t, 1, linesWithTwoPanelCorners(wide),
		"above the breakpoint the panels sit side by side")
}

// assertFitsWidth fails with the offending line when a screen overflows.
func assertFitsWidth(t *testing.T, name, rendered string, width int) {
	t.Helper()
	for index, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("%s line %d is %d columns wide in a %d column terminal:\n%s",
				name, index+1, got, width, stripANSI(line))
			return
		}
	}
}

// linesWithTwoPanelCorners counts rows that open two bordered panels, which is
// what a side-by-side layout produces and a stacked one never does.
func linesWithTwoPanelCorners(rendered string) int {
	count := 0
	for _, line := range strings.Split(stripANSI(rendered), "\n") {
		if strings.Count(line, "\u256d") >= 2 {
			count++
		}
	}
	return count
}

// joinWrapped reconstructs a value that rendering may have split across lines,
// so a test can assert the whole value survived.
func joinWrapped(rendered, want string) string {
	collapsed := strings.Join(strings.Fields(strings.ReplaceAll(stripANSI(rendered), "\n", " ")), " ")
	compact := strings.Join(strings.Fields(want), " ")
	if strings.Contains(collapsed, compact) {
		return want
	}
	// Values wrapped mid-token rejoin without a separator.
	if strings.Contains(strings.ReplaceAll(collapsed, " ", ""), strings.ReplaceAll(compact, " ", "")) {
		return want
	}
	return collapsed
}

func profileName(profile termenv.Profile) string {
	if profile == termenv.Ascii {
		return "no-color"
	}
	return "color"
}

func stripANSI(text string) string {
	var (
		out       strings.Builder
		runes     = []rune(text)
		inControl bool
	)
	for index := 0; index < len(runes); index++ {
		switch {
		case runes[index] == 0x1b:
			inControl = true
		case inControl && runes[index] >= 0x40 && runes[index] <= 0x7e && runes[index] != '[':
			inControl = false
		case !inControl:
			out.WriteRune(runes[index])
		}
	}
	return out.String()
}

func hasValidANSISequences(text string) bool {
	for index := 0; index < len(text); index++ {
		if text[index] != 0x1b {
			continue
		}
		if index+1 >= len(text) || text[index+1] != '[' {
			return false
		}
		terminated := false
		for index += 2; index < len(text); index++ {
			current := text[index]
			if current >= 0x40 && current <= 0x7e {
				terminated = true
				break
			}
			if current < 0x20 || current > 0x3f {
				return false
			}
		}
		if !terminated {
			return false
		}
	}
	return true
}

func utf8ValidString(text string) bool { return utf8.ValidString(text) }

func localconfigDefaults() localconfig.Values { return localconfig.Defaults() }

// TestDocumentedMinimumMatchesTheEnforcedOne keeps the published requirement
// and the constant that enforces it from drifting apart.
func TestDocumentedMinimumMatchesTheEnforcedOne(t *testing.T) {
	doc, err := os.ReadFile("../../src/content/docs/tui-usage.md")
	require.NoError(t, err)

	documented := fmt.Sprintf("%d×%d", minimumWidth, minimumHeight)
	assert.Contains(t, string(doc), documented,
		"the TUI guide must state the supported minimum terminal size (%s)", documented)
}

func TestHintsWrapRatherThanLosingKeys(t *testing.T) {
	segments := []string{"j/k navigate", "J/K scroll detail", "g/G top/bottom", "/ search", "q quit"}

	// Narrow enough that the hints cannot fit on one line: every key must still
	// be reachable, moved onto another line rather than cut off the end.
	rendered := stripANSI(renderHints(30, segments))
	for _, segment := range segments {
		assert.Contains(t, rendered, segment, "a hint was dropped instead of wrapped")
	}
	for _, line := range strings.Split(rendered, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 30, "a wrapped hint line still overflows")
	}
	assert.Greater(t, len(strings.Split(rendered, "\n")), 1, "the hints were expected to wrap")

	// Given room, they stay on one line.
	assert.Len(t, strings.Split(stripANSI(renderHints(200, segments)), "\n"), 1)
}
