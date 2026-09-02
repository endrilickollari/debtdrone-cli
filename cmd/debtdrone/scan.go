package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/service"
	"github.com/spf13/cobra"
)

type detailedScanRunner interface {
	RunDetailed(context.Context, string, service.ScanOptions, func(service.ScanProgress)) (service.ScanResult, error)
}

func newScanCmd() *cobra.Command {
	return newScanCommandWithResolver(defaultConfigurationResolver, func(historyEnabled bool) detailedScanRunner {
		return service.NewScanServiceWithHistoryEnabled(historyEnabled)
	})
}

func newScanCommand(svc *service.ScanService) *cobra.Command {
	return newScanCommandWithResolver(
		func(flags localconfig.Overrides) (localconfig.Resolved, error) {
			return localconfig.Resolve(localconfig.Overrides{}, nil, flags)
		},
		func(bool) detailedScanRunner { return svc },
	)
}

func newScanCommandWithResolver(resolve configurationResolver, newService func(bool) detailedScanRunner) *cobra.Command {
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
			flags, err := scanFlagOverrides(cmd, format, failOn, maxComplexity, securityScan, coverage)
			if err != nil {
				return err
			}
			resolved, err := resolve(flags)
			if err != nil {
				return fmt.Errorf("resolve scan configuration: %w", err)
			}
			if newService == nil {
				return errors.New("scan service factory is required")
			}
			svc := newService(resolved.Values.HistoryEnabled)
			if svc == nil {
				return errors.New("scan service is unavailable")
			}

			targetPath := "."
			if len(args) > 0 {
				targetPath = args[0]
			}
			absPath, err := filepath.Abs(targetPath)
			if err != nil {
				return fmt.Errorf("failed to resolve path %q: %w", targetPath, err)
			}

			ctx := context.WithValue(cmd.Context(), "isCLI", true)
			opts := service.ScanOptions{
				MaxComplexity: resolved.Values.MaxComplexity,
				SecurityScan:  resolved.Values.SecurityScan,
				Coverage:      resolved.Values.Coverage,
			}

			result, scanErr := svc.RunDetailed(ctx, absPath, opts, nil)
			for _, warning := range result.Warnings {
				cmd.PrintErrf("warning [%s]: %s\n", warning.AnalyzerID, warning.Message)
			}
			issues := result.Issues
			if scanErr != nil && !service.IsPartialFailure(scanErr) {
				return fmt.Errorf("scan failed: %w", scanErr)
			}

			switch resolved.Values.OutputFormat {
			case "json":
				if err := printJSON(cmd, issues); err != nil {
					return err
				}
			default:
				if err := printText(cmd, issues); err != nil {
					return err
				}
			}

			if resolved.Values.FailOn != "none" {
				severityMap := map[string]int{
					"critical": 4,
					"high":     3,
					"medium":   2,
					"low":      1,
				}

				requestedThreshold, ok := severityMap[resolved.Values.FailOn]
				if !ok {
					return fmt.Errorf("invalid fail-on value: %q (valid: none, critical, high, medium, low)", resolved.Values.FailOn)
				}

				for _, issue := range issues {
					if issueSeverity, exists := severityMap[strings.ToLower(issue.Severity)]; exists {
						if issueSeverity >= requestedThreshold {
							gateErr := fmt.Errorf("quality gate failed: found issues matching or exceeding severity '%s'", resolved.Values.FailOn)
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

func scanFlagOverrides(cmd *cobra.Command, format, failOn string, maxComplexity int, securityScan, coverage bool) (localconfig.Overrides, error) {
	var overrides localconfig.Overrides
	values := []struct {
		flag  string
		key   localconfig.Key
		value string
	}{
		{"format", localconfig.KeyOutputFormat, format},
		{"fail-on", localconfig.KeyFailOn, failOn},
		{"max-complexity", localconfig.KeyMaxComplexity, strconv.Itoa(maxComplexity)},
		{"security-scan", localconfig.KeySecurityScan, strconv.FormatBool(securityScan)},
		{"coverage", localconfig.KeyCoverage, strconv.FormatBool(coverage)},
	}
	for _, value := range values {
		if !cmd.Flags().Changed(value.flag) {
			continue
		}
		if err := overrides.Set(value.key, value.value); err != nil {
			return localconfig.Overrides{}, fmt.Errorf("invalid --%s value: %w", value.flag, err)
		}
	}
	return overrides, nil
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
