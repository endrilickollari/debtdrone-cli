package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/service"
	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	var (
		format        string
		failOn        string
		maxComplexity int
		securityScan  bool
		coverage      bool
	)

	cmd := &cobra.Command{
		Use:   "scan [path]",
		Short: "Run a headless technical debt scan",
		Long: `Scan a repository for technical debt without launching the TUI.
This command is optimized for CI/CD pipelines and automated workflows.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath := "."
			if len(args) > 0 {
				targetPath = args[0]
			}
			absPath, err := filepath.Abs(targetPath)
			if err != nil {
				return fmt.Errorf("failed to resolve path %q: %w", targetPath, err)
			}

			svc := service.NewScanService()
			ctx := context.WithValue(context.Background(), "isCLI", true)
			opts := service.ScanOptions{
				MaxComplexity: maxComplexity,
				SecurityScan:  securityScan,
				Coverage:      coverage,
			}

			result, scanErr := svc.RunDetailed(ctx, absPath, opts, nil)
			for _, warning := range result.Warnings {
				cmd.PrintErrf("warning [%s]: %s\n", warning.AnalyzerID, warning.Message)
			}
			issues := result.Issues
			if scanErr != nil && !service.IsPartialFailure(scanErr) {
				return fmt.Errorf("scan failed: %w", scanErr)
			}

			switch strings.ToLower(format) {
			case "json":
				if err := printJSON(cmd, issues); err != nil {
					return err
				}
			default:
				if err := printText(cmd, issues); err != nil {
					return err
				}
			}

			if failOn != "" {
				severityMap := map[string]int{
					"critical": 4,
					"high":     3,
					"medium":   2,
					"low":      1,
				}

				requestedThreshold, ok := severityMap[strings.ToLower(failOn)]
				if !ok {
					return fmt.Errorf("invalid --fail-on value: %q (valid: critical, high, medium, low)", failOn)
				}

				for _, issue := range issues {
					if issueSeverity, exists := severityMap[strings.ToLower(issue.Severity)]; exists {
						if issueSeverity >= requestedThreshold {
							gateErr := fmt.Errorf("quality gate failed: found issues matching or exceeding severity '%s'", failOn)
							if scanErr != nil {
								return errors.Join(fmt.Errorf("scan completed with partial results: %w", scanErr), gateErr)
							}
							return gateErr
						}
					}
				}
			}

			if scanErr != nil {
				return fmt.Errorf("scan completed with partial results: %w", scanErr)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "text", "Output format: text or json")
	cmd.Flags().StringVar(&failOn, "fail-on", "", "Fail the build if issues with this severity or higher are found (critical, high, medium, low)")
	cmd.Flags().IntVar(&maxComplexity, "max-complexity", 15, "High cyclomatic complexity threshold per function (critical above 2x)")
	cmd.Flags().BoolVar(&securityScan, "security-scan", true, "Enable security vulnerability scanning")
	cmd.Flags().BoolVar(&coverage, "coverage", false, "Parse existing coverage artifacts without running repository tests")

	return cmd
}

func printJSON(cmd *cobra.Command, issues []models.TechnicalDebtIssue) error {
	if issues == nil {
		issues = []models.TechnicalDebtIssue{}
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(issues)
}

func printText(cmd *cobra.Command, issues []models.TechnicalDebtIssue) error {
	if len(issues) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No technical debt issues found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)

	fmt.Fprintln(w, "SEVERITY\tFILE:LINE\tRULE\tMESSAGE")
	fmt.Fprintln(w, "--------\t---------\t----\t-------")

	for _, issue := range issues {
		location := issue.FilePath
		if issue.LineNumber != nil {
			location = fmt.Sprintf("%s:%d", issue.FilePath, *issue.LineNumber)
		}

		rule := "N/A"
		if issue.ToolRuleID != nil && *issue.ToolRuleID != "" {
			rule = *issue.ToolRuleID
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			strings.ToUpper(issue.Severity),
			location,
			rule,
			issue.Message,
		)
	}

	return w.Flush()
}
