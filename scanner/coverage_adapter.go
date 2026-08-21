package scanner

import (
	"context"
	"fmt"
	"strings"

	coveragecore "github.com/endrilickollari/debtdrone-cli/v2/scanner/coverage"
	"github.com/endrilickollari/debtdrone-cli/v2/scanner/repostructure"
)

const coverageAnalyzerID = "coverage_analyzer"

type coverageAnalyzer struct {
	repoRoot string
	roots    []repostructure.BuildRoot
	options  coveragecore.Options
}

func (coverageAnalyzer) ID() string   { return coverageAnalyzerID }
func (coverageAnalyzer) Name() string { return "Test Coverage" }

func (analyzer coverageAnalyzer) Analyze(ctx context.Context) (AnalyzerResult, error) {
	result, err := coveragecore.Analyze(ctx, analyzer.repoRoot, analyzer.roots, analyzer.options)
	if err != nil {
		return AnalyzerResult{}, err
	}
	converted := AnalyzerResult{Warnings: make([]Warning, 0, len(result.Warnings))}
	for _, warning := range result.Warnings {
		converted.Warnings = append(converted.Warnings, Warning{AnalyzerID: coverageAnalyzerID, Message: warning})
	}
	if result.Report == nil {
		return converted, nil
	}
	converted.Findings = coverageFindings(result.Report)
	converted.Metrics = coverageMetrics(result.Report)
	return converted, nil
}

func coverageFindings(report *coveragecore.Report) []Finding {
	var findings []Finding
	for _, file := range report.Files {
		if file.LinesTotal == 0 || file.LineCoverage >= 30 {
			continue
		}
		severity := "low"
		ruleID := "low-coverage"
		message := fmt.Sprintf("File has low test coverage: %.1f%% (%d/%d lines covered)", file.LineCoverage, file.LinesCovered, file.LinesTotal)
		if file.LineCoverage == 0 {
			severity = "medium"
			ruleID = "zero-coverage"
			message = fmt.Sprintf("File has 0%% test coverage (%d executable lines)", file.LinesTotal)
		}
		uncovered := file.LinesTotal - file.LinesCovered
		description := coverageDescription(file, report.Format)
		finding := Finding{
			AnalyzerID:         coverageAnalyzerID,
			RuleID:             &ruleID,
			Type:               "test-coverage",
			Category:           "test-coverage",
			Severity:           severity,
			Message:            message,
			Description:        &description,
			Location:           Location{Path: "/" + strings.TrimPrefix(file.Path, "/")},
			Confidence:         1,
			EstimatedDebtHours: float64(uncovered) / 50,
			EffortMultiplier:   1,
		}
		finding.Fingerprint = fingerprint(finding)
		findings = append(findings, finding)
	}
	return findings
}

func coverageMetrics(report *coveragecore.Report) []Metric {
	var totalLines, coveredLines, zeroFiles, lowFiles int
	for _, file := range report.Files {
		totalLines += file.LinesTotal
		coveredLines += file.LinesCovered
		if file.LinesTotal == 0 {
			continue
		}
		if file.LineCoverage == 0 {
			zeroFiles++
		} else if file.LineCoverage < 30 {
			lowFiles++
		}
	}
	metrics := []Metric{
		{Name: "test_coverage_percentage", Value: report.OverallLinePct},
		{Name: "coverage_format", Value: report.Format},
		{Name: "coverage_files_total", Value: len(report.Files)},
		{Name: "coverage_files_zero", Value: zeroFiles},
		{Name: "coverage_files_low", Value: lowFiles},
		{Name: "coverage_lines_total", Value: totalLines},
		{Name: "coverage_lines_covered", Value: coveredLines},
		{Name: "coverage_files", Value: report.Files},
	}
	if report.OverallBranchPct >= 0 {
		metrics = append(metrics, Metric{Name: "coverage_branch_percentage", Value: report.OverallBranchPct})
	}
	return metrics
}

func coverageDescription(file coveragecore.FileCoverage, format string) string {
	var description strings.Builder
	fmt.Fprintf(&description, "File: %s\n", file.Path)
	fmt.Fprintf(&description, "Line Coverage: %.1f%% (%d/%d lines)\n", file.LineCoverage, file.LinesCovered, file.LinesTotal)
	if file.BranchCoverage >= 0 {
		fmt.Fprintf(&description, "Branch Coverage: %.1f%%\n", file.BranchCoverage)
	}
	if file.FunctionCoverage >= 0 {
		fmt.Fprintf(&description, "Function Coverage: %.1f%%\n", file.FunctionCoverage)
	}
	if len(file.UncoveredLines) > 0 {
		lines := file.UncoveredLines
		label := "Uncovered Lines"
		if len(lines) > 20 {
			lines = lines[:20]
			label = "Uncovered Lines (first 20)"
		}
		fmt.Fprintf(&description, "%s: %v\n", label, lines)
	}
	fmt.Fprintf(&description, "Coverage Format: %s", format)
	return description.String()
}
