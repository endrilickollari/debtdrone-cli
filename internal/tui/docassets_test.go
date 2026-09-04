package tui

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/termsvg"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeDocAssets regenerates the documentation images:
//
//	go test ./internal/tui/ -run TestDocumentationAssets -write-doc-assets
//
// The images are produced from the same screen builders the golden fixtures
// use, so a visual can never drift from the interface it documents.
var writeDocAssets = flag.Bool("write-doc-assets", false, "regenerate the documentation screen images")

const docAssetDir = "../../src/assets/screens"

// docAsset is one image embedded in the documentation site.
type docAsset struct {
	// file is the name written under src/assets/screens.
	file string
	// title becomes the SVG accessible label.
	title string
	// screen names the golden screen this image mirrors, keeping the recorded
	// fixture and the published image in step.
	screen string
}

func documentationAssets() []docAsset {
	return []docAsset{
		{"dashboard.svg", "DebtDrone dashboard with primary actions and recent scans", "dashboard-populated"},
		{"scan-in-progress.svg", "DebtDrone scanning a repository, showing stage, elapsed time and analyzers completed", "scanning-in-progress"},
		{"scan-results.svg", "DebtDrone results workspace with summary band, findings table and detail pane", "results-populated"},
		{"scan-results-filtered.svg", "DebtDrone results filtered to high severity findings matching a search", "results-filtered"},
		{"scan-failure.svg", "DebtDrone reporting a failed scan with repository, elapsed time and retry options", "scan-failure"},
		{"history.svg", "DebtDrone session history browser showing a past scan summary", "history-populated"},
		{"config.svg", "DebtDrone interactive settings editor", "config-editor"},
	}
}

func TestDocumentationAssets(t *testing.T) {
	screens := map[string]goldenScreen{}
	for _, screen := range goldenScreens() {
		screens[screen.name] = screen
	}

	for _, asset := range documentationAssets() {
		t.Run(asset.file, func(t *testing.T) {
			screen, ok := screens[asset.screen]
			require.True(t, ok, "documentation asset references unknown screen %q", asset.screen)

			// Images are always rendered in colour, whatever profile the
			// screen's fixture records.
			withColorProfile(t, termenv.TrueColor)
			image := termsvg.Render(screen.render(t), termsvg.DefaultOptions(asset.title))

			path := filepath.Join(docAssetDir, asset.file)
			if *writeDocAssets {
				require.NoError(t, os.MkdirAll(docAssetDir, 0o755))
				require.NoError(t, os.WriteFile(path, image, 0o644))
				return
			}

			published, err := os.ReadFile(path)
			require.NoError(t, err, "missing documentation image %s; regenerate with -write-doc-assets", path)
			assert.Equal(t, normalizeLineEndings(string(published)), normalizeLineEndings(string(image)),
				"the published image for %q no longer matches the interface; regenerate with -write-doc-assets", asset.screen)
		})
	}
}

// TestDocumentationAssetsAreAllPublished catches an image left behind after the
// screen that produced it was renamed or removed.
func TestDocumentationAssetsAreAllPublished(t *testing.T) {
	published, err := filepath.Glob(filepath.Join(docAssetDir, "*.svg"))
	require.NoError(t, err)

	expected := map[string]struct{}{}
	for _, asset := range documentationAssets() {
		expected[asset.file] = struct{}{}
	}

	for _, path := range published {
		_, ok := expected[filepath.Base(path)]
		assert.True(t, ok, "%s is no longer generated; delete it or restore its entry", path)
	}
}
