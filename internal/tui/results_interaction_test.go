package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newResultsModel builds a ScanModel already showing results, which is the
// state every interaction below starts from.
func newResultsModel(issues []models.TechnicalDebtIssue) *ScanModel {
	model := newScanModel()
	model.phase = scanResults
	model.outputFormat = "text"
	model.scanPath = "/tmp/example-repo"
	model.resetResultsView(issues, summarizeIssues(issues))
	return model
}

func selectedFingerprint(t *testing.T, model *ScanModel) string {
	t.Helper()
	selected := model.list.selected()
	require.NotNil(t, selected)
	return selected.FingerprintHash
}

func TestResultsKeepSelectedFindingWhenAFilterStillIncludesIt(t *testing.T) {
	model := newResultsModel(sampleIssues())

	// Select the low-severity complexity finding, which sits last by severity.
	model.handleKey("j")
	model.handleKey("j")
	require.Equal(t, "fp-low", selectedFingerprint(t, model))
	require.Equal(t, 2, model.list.cursor)

	// Filtering to complexity keeps it, but moves it to a different row. The
	// selection must follow the finding rather than the row index.
	model.handleKey("c") // cycle category: all -> complexity
	require.Equal(t, "complexity", model.filter.category)
	require.Len(t, model.list.items, 2)
	assert.Equal(t, "fp-low", selectedFingerprint(t, model))
	assert.Equal(t, 1, model.list.cursor, "the finding moved from row 2 to row 1 and the cursor followed it")
}

func TestResultsKeepSelectedFindingWhenSortOrderChanges(t *testing.T) {
	model := newResultsModel(sampleIssues())

	model.handleKey("j")
	model.handleKey("j")
	require.Equal(t, "fp-low", selectedFingerprint(t, model))

	// Sorting by file moves the low finding to the last row; by debt it lands
	// last as well, but the identity must survive both reorderings.
	model.handleKey("s")
	require.Equal(t, sortByFile, model.filter.sort)
	assert.Equal(t, "fp-low", selectedFingerprint(t, model))

	model.handleKey("s")
	require.Equal(t, sortByDebt, model.filter.sort)
	assert.Equal(t, "fp-low", selectedFingerprint(t, model))
}

func TestSearchKeepsTheSelectedFindingWhileTheListNarrows(t *testing.T) {
	model := newResultsModel(sampleIssues())

	model.handleKey("j")
	model.handleKey("j")
	require.Equal(t, "fp-low", selectedFingerprint(t, model))

	// "o" matches all three findings; the selection must stay put rather than
	// snapping back to the first match.
	model.handleKey("/")
	model.handleKey("o")
	require.Len(t, model.list.items, 3)
	assert.Equal(t, "fp-low", selectedFingerprint(t, model))
	assert.Equal(t, 2, model.list.cursor)
}

func TestResultsHoldRowPositionWhenSelectionIsFilteredOut(t *testing.T) {
	model := newResultsModel(sampleIssues())
	model.handleKey("j")
	require.Equal(t, "fp-high", selectedFingerprint(t, model))

	// Filtering to security excludes the selected complexity finding.
	model.filter.category = "security"
	model.applyFilters()

	require.Len(t, model.list.items, 1)
	assert.Equal(t, 0, model.list.cursor, "cursor clamps into range rather than pointing past the list")
	assert.Equal(t, "fp-critical", selectedFingerprint(t, model))
}

func TestResultsScrollOffsetStaysInsideTheListAfterFiltering(t *testing.T) {
	many := make([]models.TechnicalDebtIssue, 0, 40)
	for index := 0; index < 40; index++ {
		severity := "low"
		if index%2 == 0 {
			severity = "critical"
		}
		many = append(many, models.TechnicalDebtIssue{
			FingerprintHash: string(rune('a'+index%26)) + string(rune('0'+index/26)),
			FilePath:        "file.go",
			Severity:        severity,
			Category:        "complexity",
			Message:         "finding",
		})
	}

	model := newResultsModel(many)
	model.handleKey("G") // jump to the bottom, forcing a large scroll offset
	require.Greater(t, model.list.offset, 0)

	model.handleKey("1") // filter to critical only, halving the list
	assert.LessOrEqual(t, model.list.offset, max(len(model.list.items)-model.list.height, 0))
	assert.Less(t, model.list.cursor, len(model.list.items))
	assert.GreaterOrEqual(t, model.list.cursor, model.list.offset, "cursor must remain visible in the scroll window")
}

func TestSeverityShortcutsToggleAndCombineWithSearch(t *testing.T) {
	model := newResultsModel(sampleIssues())

	model.handleKey("1") // critical
	assert.Len(t, model.list.items, 1)

	model.handleKey("2") // critical + high
	assert.Len(t, model.list.items, 2)

	// With critical+high shown, select the high finding at row 1, then drop
	// critical: the high finding becomes row 0 and must stay selected.
	model.handleKey("j")
	require.Equal(t, "fp-high", selectedFingerprint(t, model))
	model.handleKey("1") // remove critical
	require.Len(t, model.list.items, 1)
	assert.Equal(t, "fp-high", selectedFingerprint(t, model))
	assert.Equal(t, 0, model.list.cursor)

	model.handleKey("2") // remove high, back to everything
	assert.Len(t, model.list.items, 3)
	assert.False(t, model.filter.active())
}

func TestSearchModeFiltersWhileTypingAndEscapeRestoresPreviousQuery(t *testing.T) {
	model := newResultsModel(sampleIssues())

	model.handleKey("/")
	require.True(t, model.searching)
	for _, key := range []string{"m", "a", "i", "n"} {
		model.handleKey(key)
	}
	require.Len(t, model.list.items, 1, "results narrow while the reader types")
	assert.Equal(t, "fp-critical", selectedFingerprint(t, model))

	model.handleKey("enter")
	assert.False(t, model.searching)
	assert.Equal(t, "main", model.filter.query)

	// Reopening and cancelling restores the committed query.
	model.handleKey("/")
	model.handleKey("backspace")
	require.Equal(t, "mai", model.filter.query)
	model.handleKey("esc")
	assert.False(t, model.searching)
	assert.Equal(t, "main", model.filter.query, "esc cancels the edit instead of clearing the search")
}

func TestSearchModeAcceptsUnicodeAndWindowsPathSeparators(t *testing.T) {
	issues := []models.TechnicalDebtIssue{{
		FingerprintHash: "fp-unicode",
		FilePath:        `C:\repo\café.go`,
		Severity:        "high",
		Message:         "unicode path",
	}}
	model := newResultsModel(issues)

	model.handleKey("/")
	query := `C:\repo\café`
	for _, key := range []rune(query) {
		model.handleKey(string(key))
	}

	assert.Equal(t, query, model.filter.query)
	require.Len(t, model.list.items, 1)
	assert.Equal(t, "fp-unicode", selectedFingerprint(t, model))
}

func TestClearRestoresEveryFindingButKeepsSortOrder(t *testing.T) {
	model := newResultsModel(sampleIssues())

	model.handleKey("s") // sort by file
	model.handleKey("1") // severity critical
	model.filter.query = "main"
	model.applyFilters()
	require.True(t, model.filter.active())

	model.handleKey("x")
	assert.False(t, model.filter.active())
	assert.Len(t, model.list.items, 3)
	assert.Equal(t, sortByFile, model.filter.sort, "clearing filters is not a sort reset")
}

func TestSortShortcutCyclesThroughEveryMode(t *testing.T) {
	model := newResultsModel(sampleIssues())
	require.Equal(t, sortBySeverity, model.filter.sort)

	for _, expected := range []sortMode{sortByFile, sortByDebt, sortByCategory, sortBySeverity} {
		model.handleKey("s")
		assert.Equal(t, expected, model.filter.sort)
	}
}

func TestNoMatchesStateExplainsFiltersAndHowToRecover(t *testing.T) {
	model := newResultsModel(sampleIssues())
	model.filter.query = "nothing-matches-this"
	model.applyFilters()

	rendered := model.renderResults()
	assert.Contains(t, rendered, "No findings match the current filters")
	assert.Contains(t, rendered, "clear all filters")
	assert.Contains(t, rendered, "0 of 3 findings")
}

func TestZeroFindingsStateReportsACleanScan(t *testing.T) {
	model := newResultsModel(nil)

	rendered := model.renderResults()
	assert.Contains(t, rendered, "clean scan")
	assert.Contains(t, rendered, "example-repo")
}

func TestSummaryBandIsVisibleWithoutNavigation(t *testing.T) {
	model := newResultsModel(sampleIssues())

	rendered := model.renderResults()
	assert.Contains(t, rendered, "3 findings")
	assert.Contains(t, rendered, "across 3 files")
	assert.Contains(t, rendered, "6.5h estimated debt")
	assert.Contains(t, rendered, "Critical 1")
}

func TestExportWritesOnlyTheFindingsCurrentlyInView(t *testing.T) {
	working := t.TempDir()
	previous, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(working))
	t.Cleanup(func() { _ = os.Chdir(previous) })

	model := newResultsModel(sampleIssues())
	model.handleKey("1") // critical only
	require.Len(t, model.list.items, 1)

	model.handleKey("e")
	assert.Contains(t, model.status, "Exported 1 findings")

	matches, err := filepath.Glob(filepath.Join(working, "debtdrone-example-repo-*.json"))
	require.NoError(t, err)
	require.Len(t, matches, 1)

	raw, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	var exported []models.TechnicalDebtIssue
	require.NoError(t, json.Unmarshal(raw, &exported))
	require.Len(t, exported, 1)
	assert.Equal(t, "fp-critical", exported[0].FingerprintHash)
}

func TestExportReportsWhenFiltersLeaveNothingToWrite(t *testing.T) {
	model := newResultsModel(sampleIssues())
	model.filter.query = "nothing-matches-this"
	model.applyFilters()

	model.handleKey("e")
	assert.Contains(t, model.status, "Nothing to export")
}

func TestExportUsesPrivateUniqueFilesWithoutOverwriting(t *testing.T) {
	working := t.TempDir()
	previous, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(working))
	t.Cleanup(func() { _ = os.Chdir(previous) })

	const stamp = "20260903-120000"
	firstName := exportFileName("/tmp/example-repo", stamp)
	require.NoError(t, os.WriteFile(firstName, []byte("keep me"), 0o600))

	secondName, err := writeFindingsExport("/tmp/example-repo", stamp, []byte(`[{"message":"new export"}]`))
	require.NoError(t, err)
	assert.Equal(t, "debtdrone-example-repo-20260903-120000-2.json", secondName)

	original, err := os.ReadFile(firstName)
	require.NoError(t, err)
	assert.Equal(t, "keep me", string(original))

	info, err := os.Stat(secondName)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestExportDoesNotFollowExistingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional privileges on some Windows hosts")
	}

	working := t.TempDir()
	previous, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(working))
	t.Cleanup(func() { _ = os.Chdir(previous) })

	const stamp = "20260903-120000"
	target := filepath.Join(working, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("unchanged"), 0o600))
	require.NoError(t, os.Symlink(target, exportFileName("/tmp/example-repo", stamp)))

	name, err := writeFindingsExport("/tmp/example-repo", stamp, []byte("export"))
	require.NoError(t, err)
	assert.Equal(t, "debtdrone-example-repo-20260903-120000-2.json", name)

	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", string(content))
}

func TestScanCompletionSummarizesAllFindingsBeforeDisplayLimit(t *testing.T) {
	model := newScanModel()
	model.phase = scanRunning
	model.scanPath = "/tmp/example-repo"
	model.display = scanDisplayOptions{maxResults: 1, showLineNumbers: true}

	_, command := model.Update(scanCompleteMsg{path: model.scanPath, issues: sampleIssues()})
	require.NotNil(t, command)
	require.Len(t, model.list.items, 1, "the configured render limit still bounds the table")
	assert.Equal(t, 3, model.summary.total)
	assert.Equal(t, 3, model.summary.filesAffected)
	assert.InDelta(t, 6.5, model.summary.debtHours, 0.001)

	finished, ok := command().(ScanFinishedMsg)
	require.True(t, ok)
	assert.Equal(t, model.summary, finished.Entry.summary)
	assert.InDelta(t, 6.5, finished.Entry.run.TotalTechnicalDebtHours, 0.001)
}

func TestRescanStartsTheSameRepository(t *testing.T) {
	model := newResultsModel(sampleIssues())

	_, command := model.handleKey("r")
	require.NotNil(t, command)
	message, ok := command().(StartScanMsg)
	require.True(t, ok)
	assert.Equal(t, model.scanPath, message.Path)
}

func TestJSONOutputIgnoresFilterShortcutsAndKeepsScrolling(t *testing.T) {
	model := newResultsModel(sampleIssues())
	model.outputFormat = "json"
	model.detail.setContent("line one\nline two\nline three\nline four")
	model.detail.height = 2

	model.handleKey("/")
	assert.False(t, model.searching, "raw JSON output has no findings table to search")

	model.handleKey("1")
	assert.False(t, model.filter.active())

	model.handleKey("j")
	assert.Equal(t, 1, model.detail.offset, "j/k still scrolls the JSON viewport")
}

func TestLoadingHistoryInJSONModeRestoresTheRawFindings(t *testing.T) {
	issues := sampleIssues()
	model := newScanModel()
	model.LoadResults(historyEntry{
		path:    "/tmp/example-repo",
		issues:  issues,
		summary: summarizeIssues(issues),
	}, "json")

	raw := strings.Join(model.detail.lines, "\n")
	assert.Contains(t, raw, `"fingerprint_hash": "fp-critical"`)
	assert.Contains(t, raw, `"file_path": "cmd/main.go"`)
}

func TestNavigatingDismissesTheExportStatusAndRestoresTheFilterSummary(t *testing.T) {
	working := t.TempDir()
	previous, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(working))
	t.Cleanup(func() { _ = os.Chdir(previous) })

	model := newResultsModel(sampleIssues())
	model.handleKey("1") // critical only
	model.handleKey("e")
	require.Contains(t, model.renderResults(), "Exported 1 findings")

	model.handleKey("j")
	assert.Empty(t, model.status)
	rendered := model.renderResults()
	assert.NotContains(t, rendered, "Exported 1 findings")
	assert.Contains(t, rendered, "severity critical", "the filter summary reclaims the status row")
}
