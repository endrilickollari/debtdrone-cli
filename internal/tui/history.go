package tui

import "github.com/endrilickollari/debtdrone-cli/internal/models"

type historyEntry struct {
	run    models.AnalysisRun
	path   string
	issues []models.TechnicalDebtIssue
}
