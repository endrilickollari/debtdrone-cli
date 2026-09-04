package tui

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// updateGolden rewrites the fixtures instead of comparing against them:
//
//	go test ./internal/tui/ -run TestGoldenScreens -update-golden
//
// Review the resulting diff the same way you would review source: an
// unexplained change to a fixture is an unexplained change to the interface.
var updateGolden = flag.Bool("update-golden", false, "rewrite TUI golden fixtures instead of comparing against them")

const goldenDir = "testdata/golden"

// Terminal sizes the fixtures are recorded at. "narrow" is deliberately below
// the width at which the results table drops its category column.
const (
	wideWidth, wideHeight     = 120, 40
	narrowWidth, narrowHeight = 70, 24
)

var errTrivyUnavailable = errors.New("security analyzer failed: trivy executable not found in PATH")

type goldenScreen struct {
	name    string
	profile termenv.Profile
	render  func(t *testing.T) string
}

// goldenScreens is the single list of recorded screens. Both the comparison
// test and the orphan check read it, so a fixture can never drift away from
// the case that produces it.
func goldenScreens() []goldenScreen {
	failedScan := func(t *testing.T) *ScanModel {
		t.Helper()
		model := scanningScreen(t, wideWidth, wideHeight, "SecurityAnalyzer", 1, 5)
		model.Update(scanCompleteMsg{runID: model.runID, path: fixtureRepositoryPath, err: errTrivyUnavailable})
		model.elapsed = 9 * time.Second
		return model
	}

	return []goldenScreen{
		{"dashboard-populated", termenv.TrueColor, func(t *testing.T) string {
			return dashboardScreen(t, wideWidth, wideHeight, fixtureRecords(), nil).render()
		}},
		{"dashboard-empty", termenv.TrueColor, func(t *testing.T) string {
			return dashboardScreen(t, wideWidth, wideHeight, nil, nil).render()
		}},
		{"dashboard-history-unavailable", termenv.TrueColor, func(t *testing.T) string {
			return dashboardScreen(t, wideWidth, wideHeight, nil, os.ErrPermission).render()
		}},
		{"dashboard-narrow", termenv.TrueColor, func(t *testing.T) string {
			return dashboardScreen(t, narrowWidth, narrowHeight, fixtureRecords(), nil).render()
		}},
		{"dashboard-no-color", termenv.Ascii, func(t *testing.T) string {
			return dashboardScreen(t, wideWidth, wideHeight, fixtureRecords(), nil).render()
		}},
		{"dashboard-help", termenv.TrueColor, func(t *testing.T) string {
			menu := dashboardScreen(t, wideWidth, wideHeight, fixtureRecords(), nil)
			menu.ShowHelp()
			return menu.renderHelp()
		}},

		{"scanning-preparing", termenv.TrueColor, func(t *testing.T) string {
			return scanningScreen(t, wideWidth, wideHeight, "", 0, 0).render()
		}},
		{"scanning-in-progress", termenv.TrueColor, func(t *testing.T) string {
			return scanningScreen(t, wideWidth, wideHeight, "ComplexityAnalyzer", 2, 5).render()
		}},
		{"scanning-narrow", termenv.TrueColor, func(t *testing.T) string {
			return scanningScreen(t, narrowWidth, narrowHeight, "ComplexityAnalyzer", 2, 5).render()
		}},
		{"scanning-no-color", termenv.Ascii, func(t *testing.T) string {
			return scanningScreen(t, wideWidth, wideHeight, "ComplexityAnalyzer", 2, 5).render()
		}},

		{"results-populated", termenv.TrueColor, func(t *testing.T) string {
			return resultsScreen(t, wideWidth, wideHeight, fixtureIssues()).render()
		}},
		{"results-detail-selected", termenv.TrueColor, func(t *testing.T) string {
			// The first finding carries description, evidence, and surrounding
			// context, so this fixture covers a fully populated detail pane.
			model := resultsScreen(t, wideWidth, wideHeight, fixtureIssues())
			return formatIssueDetail(model.list.selected(), model.detail.width)
		}},
		{"results-filtered", termenv.TrueColor, func(t *testing.T) string {
			model := resultsScreen(t, wideWidth, wideHeight, fixtureIssues())
			model.handleKey("2") // high severity only
			model.handleKey("/")
			for _, key := range strings.Split("complex", "") {
				model.handleKey(key)
			}
			model.handleKey("enter")
			return model.render()
		}},
		{"results-no-matches", termenv.TrueColor, func(t *testing.T) string {
			model := resultsScreen(t, wideWidth, wideHeight, fixtureIssues())
			model.filter.query = "no-such-finding"
			model.applyFilters()
			return model.render()
		}},
		{"results-clean-scan", termenv.TrueColor, func(t *testing.T) string {
			return resultsScreen(t, wideWidth, wideHeight, nil).render()
		}},
		{"results-narrow", termenv.TrueColor, func(t *testing.T) string {
			return resultsScreen(t, narrowWidth, narrowHeight, fixtureIssues()).render()
		}},
		{"results-no-color", termenv.Ascii, func(t *testing.T) string {
			return resultsScreen(t, wideWidth, wideHeight, fixtureIssues()).render()
		}},

		{"scan-failure", termenv.TrueColor, func(t *testing.T) string {
			return failedScan(t).render()
		}},
		{"scan-failure-no-color", termenv.Ascii, func(t *testing.T) string {
			return failedScan(t).render()
		}},

		{"history-populated", termenv.TrueColor, func(t *testing.T) string {
			return historyScreen(t, wideWidth, wideHeight).render()
		}},
		{"config-editor", termenv.TrueColor, func(t *testing.T) string {
			return configScreen(t, wideWidth, wideHeight).render()
		}},
	}
}

func TestGoldenScreens(t *testing.T) {
	for _, screen := range goldenScreens() {
		t.Run(screen.name, func(t *testing.T) {
			withColorProfile(t, screen.profile)
			assertGolden(t, screen.name, screen.render(t))
		})
	}
}

// TestGoldenFixturesAreAllExercised guards against a fixture outliving the
// screen that produced it: a stale file would otherwise sit in testdata
// forever without any test reading it.
func TestGoldenFixturesAreAllExercised(t *testing.T) {
	recorded, err := filepath.Glob(filepath.Join(goldenDir, "*.txt"))
	require.NoError(t, err)
	require.NotEmpty(t, recorded, "no golden fixtures found; regenerate with -update-golden")

	produced := map[string]struct{}{}
	for _, screen := range goldenScreens() {
		produced[screen.name] = struct{}{}
	}

	for _, path := range recorded {
		name := strings.TrimSuffix(filepath.Base(path), ".txt")
		_, ok := produced[name]
		assert.True(t, ok, "fixture %s is no longer produced by any screen; delete it or restore its case", path)
	}
}

// assertGolden compares a rendered screen against its recorded fixture. Escape
// sequences are kept, so the fixtures capture colour and styling rather than
// text alone.
func assertGolden(t *testing.T, name, rendered string) {
	t.Helper()

	path := filepath.Join(goldenDir, name+".txt")
	content := normalizeScreen(rendered)

	if *updateGolden {
		require.NoError(t, os.MkdirAll(goldenDir, 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		return
	}

	expected, err := os.ReadFile(path)
	require.NoError(t, err, "missing fixture %s; regenerate with -update-golden", path)
	assert.Equal(t, normalizeLineEndings(string(expected)), content,
		"screen %q changed; if the change is intended, rerun with -update-golden and review the diff", name)
}

// normalizeScreen drops the padding a rendered screen carries to the terminal
// edge. Trailing blanks hold no information and are the first thing an editor
// or a diff tool would silently rewrite.
func normalizeScreen(rendered string) string {
	rendered = normalizeLineEndings(rendered)
	lines := strings.Split(rendered, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimRight(line, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func normalizeLineEndings(content string) string {
	return strings.ReplaceAll(content, "\r\n", "\n")
}
