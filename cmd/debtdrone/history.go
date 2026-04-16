package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// mockAnalysisRuns provides sample data for the history command
func mockAnalysisRuns(limit int) []models.AnalysisRun {
	repoPath1 := "./debtdrone-cli"
	repoPath2 := "./webapp"
	
	runs := []models.AnalysisRun{
		{
			ID:                      uuid.New(),
			RepositoryName:          &repoPath1,
			StartedAt:               time.Now().Add(-2 * time.Hour),
			Status:                  "completed",
			TotalIssuesFound:        15,
			CriticalIssuesCount:     1,
			HighIssuesCount:         3,
			MediumIssuesCount:       5,
			LowIssuesCount:          6,
			TotalTechnicalDebtHours: 4.5,
		},
		{
			ID:                      uuid.New(),
			RepositoryName:          &repoPath2,
			StartedAt:               time.Now().Add(-24 * time.Hour),
			Status:                  "completed",
			TotalIssuesFound:        8,
			CriticalIssuesCount:     0,
			HighIssuesCount:         1,
			MediumIssuesCount:       3,
			LowIssuesCount:          4,
			TotalTechnicalDebtHours: 1.2,
		},
	}

	if limit > len(runs) {
		limit = len(runs)
	}
	return runs[:limit]
}

func newHistoryCmd() *cobra.Command {
	var (
		format string
		limit  int
	)

	cmd := &cobra.Command{
		Use:   "history",
		Short: "View past technical debt scans",
		RunE: func(cmd *cobra.Command, args []string) error {
			runs := mockAnalysisRuns(limit)

			if format == "json" {
				return printHistoryJSON(runs)
			}
			return printHistoryTable(runs)
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or json")
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum number of historical entries to list")

	return cmd
}

func printHistoryJSON(runs []models.AnalysisRun) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(runs)
}

func printHistoryTable(runs []models.AnalysisRun) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	
	// Print Header
	fmt.Fprintln(w, "DATE\tREPOSITORY\tISSUES\tCRITICAL\tHIGH")
	fmt.Fprintln(w, "----\t----------\t------\t--------\t----")

	for _, run := range runs {
		repo := "unknown"
		if run.RepositoryName != nil {
			repo = *run.RepositoryName
		}

		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\n",
			run.StartedAt.Format("2006-01-02 15:04"),
			repo,
			run.TotalIssuesFound,
			run.CriticalIssuesCount,
			run.HighIssuesCount,
		)
	}

	return w.Flush()
}
