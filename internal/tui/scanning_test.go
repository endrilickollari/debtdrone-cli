package tui

import (
	"errors"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/scanner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanModelDisplaysPartialResultsWithWarning(t *testing.T) {
	model := newScanModel()
	model.outputFormat = "text"
	partial := &scanner.PartialFailureError{Failures: []scanner.AnalyzerFailure{{AnalyzerID: "trivy", Error: "failed"}}}
	issue := models.TechnicalDebtIssue{FilePath: "/main.go", Severity: "high", Message: "complex function"}

	_, command := model.Update(scanCompleteMsg{path: "/repo", issues: []models.TechnicalDebtIssue{issue}, err: partial})
	require.NotNil(t, command)
	assert.NoError(t, model.err)
	assert.True(t, errors.Is(model.warning, partial))
	assert.Len(t, model.issues, 1)
	assert.Contains(t, model.renderResults(), "Partial scan:")

	message, ok := command().(ScanFinishedMsg)
	require.True(t, ok)
	assert.NoError(t, message.Err)
	assert.Len(t, message.Entry.issues, 1)
}
