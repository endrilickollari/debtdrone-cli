package scancore

import (
	"context"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
)

type Result struct {
	Issues  []models.TechnicalDebtIssue
	Metrics map[string]interface{}
}

type Analyzer interface {
	Name() string
	Analyze(context.Context, *git.Repository) (*Result, error)
}
