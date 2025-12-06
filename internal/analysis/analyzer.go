package analysis

import (
	"context"

	"github.com/endrilickollari/debtdrone-cli/internal/git"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
)

type Result struct {
	Issues  []models.TechnicalDebtIssue
	Metrics map[string]interface{}
}

type Analyzer interface {
	Name() string
	Analyze(ctx context.Context, repo *git.Repository) (*Result, error)
}
