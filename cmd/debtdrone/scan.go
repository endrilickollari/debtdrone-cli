package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/endrilickollari/debtdrone-cli/internal/service"
	"github.com/spf13/cobra"
)

// newScanCmd constructs the 'debtdrone scan' subcommand for headless execution.
func newScanCmd() *cobra.Command {
	var (
		format        string
		failOn        string
		maxComplexity int
		securityScan  bool
	)

	cmd := &cobra.Command{
		Use:   "scan [path]",
		Short: "Run a headless technical debt scan",
		Long: `Scan a repository for technical debt without launching the TUI.
This command is optimized for CI/CD pipelines and automated workflows.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 1. Resolve Target Path
			targetPath := "."
			if len(args) > 0 {
				targetPath = args[0]
			}
			absPath, err := filepath.Abs(targetPath)
			if err != nil {
				return fmt.Errorf("failed to resolve path %q: %w", targetPath, err)
			}

			// 2. Engine Initialization & Execution
			svc := service.NewScanService()
			ctx := context.WithValue(context.Background(), "isCLI", true)
			opts := service.ScanOptions{
				MaxComplexity: maxComplexity,
				SecurityScan:  securityScan,
			}

			// Execute the scan synchronously (no progress bars in headless mode)
			issues, err := svc.Run(ctx, absPath, opts, nil)
			if err != nil {
				return fmt.Errorf("scan failed: %w", err)
			}

			// 3. Output Formatting
			switch strings.ToLower(format) {
			case "json":
				if err := printJSON(issues); err != nil {
					return err
				}
			default:
				if err := printText(issues); err != nil {
					return err
				}
			}

			// 4. CI/CD Quality Gate Logic
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
							// Return a custom error that Cobra will handle
							return fmt.Errorf("quality gate failed: found issues matching or exceeding severity '%s'", failOn)
						}
					}
				}
			}

			return nil
		},
	}

	// Flags
	cmd.Flags().StringVarP(&format, "format", "f", "text", "Output format: text or json")
	cmd.Flags().StringVar(&failOn, "fail-on", "", "Fail the build if issues with this severity or higher are found (critical, high, medium, low)")
	cmd.Flags().IntVar(&maxComplexity, "max-complexity", 15, "Cyclomatic complexity threshold per function")
	cmd.Flags().BoolVar(&securityScan, "security-scan", true, "Enable security vulnerability scanning")

	return cmd
}

// printJSON outputs the scan results as a pretty-printed JSON array.
func printJSON(issues []models.TechnicalDebtIssue) error {
	if issues == nil {
		issues = []models.TechnicalDebtIssue{}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(issues)
}

// printText outputs the scan results in a clean table using text/tabwriter.
func printText(issues []models.TechnicalDebtIssue) error {
	if len(issues) == 0 {
		fmt.Println("No technical debt issues found.")
		return nil
	}

	// Initialize tabwriter for a clean columnar layout
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	
	// Print Header
	fmt.Fprintln(w, "SEVERITY\tFILE:LINE\tRULE\tMESSAGE")
	fmt.Fprintln(w, "--------\t---------\t----\t-------")

	for _, issue := range issues {
		// Format File:Line
		location := issue.FilePath
		if issue.LineNumber != nil {
			location = fmt.Sprintf("%s:%d", issue.FilePath, *issue.LineNumber)
		}

		// Format Rule
		rule := "N/A"
		if issue.ToolRuleID != nil && *issue.ToolRuleID != "" {
			rule = *issue.ToolRuleID
		}

		// Print Row
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", 
			strings.ToUpper(issue.Severity),
			location,
			rule,
			issue.Message,
		)
	}

	return w.Flush()
}
