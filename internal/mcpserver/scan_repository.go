package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/v2/scanner"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	scanRepositoryToolName      = "scan_repository"
	scanRepositorySchemaVersion = "debtdrone.scan_repository/v1"
	defaultMaxComplexity        = 15
	maximumMaxComplexity        = 10000
	defaultMaxFindings          = 200
	maximumMaxFindings          = 1000
	maximumWarnings             = 200
	maximumFailures             = 50
	maximumMetricGroups         = 25
	maximumMetricsPerGroup      = 100
	maximumResponseBytes        = 1024 * 1024
)

type scanFunc func(context.Context, string, scanner.Options) (scanner.Report, error)

// ScanRepositoryInput is the validated input contract for scan_repository.
type ScanRepositoryInput struct {
	Path          string `json:"path,omitempty" jsonschema:"Repository path relative to the configured MCP root. Defaults to the configured root."`
	MaxComplexity int    `json:"max_complexity,omitempty" jsonschema:"Cyclomatic complexity threshold. Defaults to 15, matching the CLI."`
	SecurityScan  *bool  `json:"security_scan,omitempty" jsonschema:"Run the optional Trivy security analyzer. Defaults to true, matching the CLI."`
	Coverage      bool   `json:"coverage,omitempty" jsonschema:"Parse existing coverage artifacts without executing repository tests. Defaults to false."`
	MaxFindings   int    `json:"max_findings,omitempty" jsonschema:"Maximum findings returned before truncation. Defaults to 200 and cannot exceed 1000."`
}

// ScanRepositoryOutput is the stable, versioned scan_repository result.
type ScanRepositoryOutput struct {
	SchemaVersion    string             `json:"schema_version" jsonschema:"Version of this MCP result contract."`
	Status           string             `json:"status" jsonschema:"Scan status: complete or partial."`
	Repository       string             `json:"repository" jsonschema:"Scanned path relative to the configured MCP root."`
	Findings         []ScanFinding      `json:"findings" jsonschema:"Deterministically ordered technical-debt findings."`
	Metrics          []ScanMetricGroup  `json:"metrics" jsonschema:"Analyzer metrics grouped and ordered by analyzer ID."`
	Warnings         []ScanWarning      `json:"warnings" jsonschema:"Non-fatal scan warnings."`
	Failures         []ScanFailure      `json:"failures" jsonschema:"Analyzer failures retained when a partial report is available."`
	TotalFindings    int                `json:"total_findings" jsonschema:"Total findings produced before response truncation."`
	ReturnedFindings int                `json:"returned_findings" jsonschema:"Number of findings included in this response."`
	Truncated        bool               `json:"truncated" jsonschema:"Whether any response content was omitted to satisfy limits."`
	Omitted          ScanOmittedCounts  `json:"omitted" jsonschema:"Counts of content omitted by response limits."`
	Limits           ScanResponseLimits `json:"limits" jsonschema:"Effective response limits for this call."`
}

type ScanFinding struct {
	Fingerprint        string       `json:"fingerprint"`
	AnalyzerID         string       `json:"analyzer_id"`
	RuleID             *string      `json:"rule_id,omitempty"`
	Type               string       `json:"type"`
	Category           string       `json:"category"`
	Severity           string       `json:"severity"`
	Message            string       `json:"message"`
	Description        *string      `json:"description,omitempty"`
	Location           ScanLocation `json:"location"`
	Confidence         float64      `json:"confidence"`
	EstimatedDebtHours float64      `json:"estimated_debt_hours"`
	EffortMultiplier   float64      `json:"effort_multiplier"`
	CodeSnippet        *string      `json:"code_snippet,omitempty"`
}

type ScanLocation struct {
	Path   string `json:"path"`
	Line   *int   `json:"line,omitempty"`
	Column *int   `json:"column,omitempty"`
}

type ScanMetricGroup struct {
	AnalyzerID string       `json:"analyzer_id"`
	Metrics    []ScanMetric `json:"metrics"`
}

type ScanMetric struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

type ScanWarning struct {
	AnalyzerID string `json:"analyzer_id"`
	Message    string `json:"message"`
}

type ScanFailure struct {
	AnalyzerID   string `json:"analyzer_id"`
	AnalyzerName string `json:"analyzer_name"`
	Error        string `json:"error"`
}

type ScanOmittedCounts struct {
	Findings     int `json:"findings"`
	Warnings     int `json:"warnings"`
	Failures     int `json:"failures"`
	MetricGroups int `json:"metric_groups"`
	Metrics      int `json:"metrics"`
	CodeSnippets int `json:"code_snippets"`
}

type ScanResponseLimits struct {
	MaxFindings int `json:"max_findings"`
	MaxBytes    int `json:"max_bytes"`
}

func (s *Server) addScanRepositoryTool() {
	openWorld := true
	nondestructive := false
	mcp.AddTool(s.protocol, &mcp.Tool{
		Name:        scanRepositoryToolName,
		Title:       "Scan repository",
		Description: "Scan a repository below the configured root for technical debt. Returns a deterministic, versioned report and never executes repository tests.",
		InputSchema: scanRepositoryInputSchema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			IdempotentHint:  true,
			OpenWorldHint:   &openWorld,
			DestructiveHint: &nondestructive,
		},
	}, s.scanRepository)
}

func scanRepositoryInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[ScanRepositoryInput](nil)
	if err != nil {
		panic(fmt.Sprintf("create scan_repository input schema: %v", err))
	}

	schema.Properties["path"].MaxLength = jsonschema.Ptr(4096)
	schema.Properties["max_complexity"].Minimum = jsonschema.Ptr(1.0)
	schema.Properties["max_complexity"].Maximum = jsonschema.Ptr(float64(maximumMaxComplexity))
	schema.Properties["security_scan"].Type = "boolean"
	schema.Properties["security_scan"].Types = nil
	schema.Properties["max_findings"].Minimum = jsonschema.Ptr(1.0)
	schema.Properties["max_findings"].Maximum = jsonschema.Ptr(float64(maximumMaxFindings))
	return schema
}

func (s *Server) scanRepository(ctx context.Context, _ *mcp.CallToolRequest, input ScanRepositoryInput) (*mcp.CallToolResult, ScanRepositoryOutput, error) {
	input = applyInputDefaults(input)
	if err := validateInput(input); err != nil {
		return nil, ScanRepositoryOutput{}, err
	}

	repositoryPath, repositoryLabel, err := resolveRepositoryPath(s.root, input.Path)
	if err != nil {
		return nil, ScanRepositoryOutput{}, err
	}

	scan := s.scan
	if scan == nil {
		scan = scanner.Scan
	}
	report, scanErr := scan(ctx, repositoryPath, scanner.Options{
		Scope:      scanner.FullScan(),
		Complexity: scanner.ComplexityOptions{Enabled: true, MaxCyclomatic: input.MaxComplexity},
		Security:   scanner.SecurityOptions{Enabled: *input.SecurityScan},
		Coverage:   scanner.CoverageOptions{Enabled: input.Coverage},
	})

	status := "complete"
	if scanErr != nil {
		var partial *scanner.PartialFailureError
		if !errors.As(scanErr, &partial) {
			if errors.Is(scanErr, context.Canceled) || errors.Is(scanErr, context.DeadlineExceeded) {
				return nil, ScanRepositoryOutput{}, fmt.Errorf("scan_repository cancelled while scanning %q: %w", repositoryLabel, scanErr)
			}
			return nil, ScanRepositoryOutput{}, fmt.Errorf("scan_repository could not scan %q: %w", repositoryLabel, scanErr)
		}
		status = "partial"
	}

	output, err := buildScanOutput(repositoryLabel, status, report, input.MaxFindings)
	if err != nil {
		return nil, ScanRepositoryOutput{}, fmt.Errorf("build scan_repository response: %w", err)
	}
	summary := fmt.Sprintf("DebtDrone scan %s: returned %d of %d findings", output.Status, output.ReturnedFindings, output.TotalFindings)
	if output.Truncated {
		summary += " (response truncated; inspect omitted and limits)"
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: summary}}}, output, nil
}

func applyInputDefaults(input ScanRepositoryInput) ScanRepositoryInput {
	if input.Path == "" {
		input.Path = "."
	}
	if input.MaxComplexity == 0 {
		input.MaxComplexity = defaultMaxComplexity
	}
	if input.SecurityScan == nil {
		input.SecurityScan = jsonschema.Ptr(true)
	}
	if input.MaxFindings == 0 {
		input.MaxFindings = defaultMaxFindings
	}
	return input
}

func validateInput(input ScanRepositoryInput) error {
	if input.MaxComplexity < 1 || input.MaxComplexity > maximumMaxComplexity {
		return fmt.Errorf("max_complexity must be between 1 and %d", maximumMaxComplexity)
	}
	if input.MaxFindings < 1 || input.MaxFindings > maximumMaxFindings {
		return fmt.Errorf("max_findings must be between 1 and %d", maximumMaxFindings)
	}
	return nil
}

func resolveRepositoryPath(root, requested string) (string, string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "."
	}

	root = filepath.Clean(root)
	candidate := requested
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate = filepath.Clean(candidate)
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("repository path %q is outside the configured MCP root", requested)
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", "", fmt.Errorf("inspect repository path %q: %w", requested, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("repository path %q is not a directory", requested)
	}

	return candidate, filepath.ToSlash(relative), nil
}

func buildScanOutput(repository, status string, report scanner.Report, maxFindings int) (ScanRepositoryOutput, error) {
	findings := make([]ScanFinding, 0, len(report.Findings))
	for _, finding := range report.Findings {
		findings = append(findings, ScanFinding{
			Fingerprint:        finding.Fingerprint,
			AnalyzerID:         finding.AnalyzerID,
			RuleID:             finding.RuleID,
			Type:               finding.Type,
			Category:           finding.Category,
			Severity:           finding.Severity,
			Message:            finding.Message,
			Description:        finding.Description,
			Location:           ScanLocation{Path: finding.Location.Path, Line: finding.Location.Line, Column: finding.Location.Column},
			Confidence:         finding.Confidence,
			EstimatedDebtHours: finding.EstimatedDebtHours,
			EffortMultiplier:   finding.EffortMultiplier,
			CodeSnippet:        finding.CodeSnippet,
		})
	}
	sortFindings(findings)

	warnings := make([]ScanWarning, 0, len(report.Warnings))
	for _, warning := range report.Warnings {
		warnings = append(warnings, ScanWarning{AnalyzerID: warning.AnalyzerID, Message: warning.Message})
	}
	sort.Slice(warnings, func(i, j int) bool {
		if warnings[i].AnalyzerID != warnings[j].AnalyzerID {
			return warnings[i].AnalyzerID < warnings[j].AnalyzerID
		}
		return warnings[i].Message < warnings[j].Message
	})

	failures := make([]ScanFailure, 0, len(report.Failures))
	for _, failure := range report.Failures {
		failures = append(failures, ScanFailure{AnalyzerID: failure.AnalyzerID, AnalyzerName: failure.AnalyzerName, Error: failure.Error})
	}
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].AnalyzerID != failures[j].AnalyzerID {
			return failures[i].AnalyzerID < failures[j].AnalyzerID
		}
		return failures[i].Error < failures[j].Error
	})

	metricIDs := make([]string, 0, len(report.Metrics))
	for analyzerID := range report.Metrics {
		metricIDs = append(metricIDs, analyzerID)
	}
	sort.Strings(metricIDs)
	metrics := make([]ScanMetricGroup, 0, len(metricIDs))
	for _, analyzerID := range metricIDs {
		group := ScanMetricGroup{AnalyzerID: analyzerID, Metrics: make([]ScanMetric, 0, len(report.Metrics[analyzerID]))}
		for _, metric := range report.Metrics[analyzerID] {
			group.Metrics = append(group.Metrics, ScanMetric{Name: metric.Name, Value: metric.Value})
		}
		sort.SliceStable(group.Metrics, func(i, j int) bool {
			if group.Metrics[i].Name != group.Metrics[j].Name {
				return group.Metrics[i].Name < group.Metrics[j].Name
			}
			return stableJSON(group.Metrics[i].Value) < stableJSON(group.Metrics[j].Value)
		})
		metrics = append(metrics, group)
	}

	output := ScanRepositoryOutput{
		SchemaVersion: scanRepositorySchemaVersion,
		Status:        status,
		Repository:    repository,
		Findings:      findings,
		Metrics:       metrics,
		Warnings:      warnings,
		Failures:      failures,
		TotalFindings: len(findings),
		Limits: ScanResponseLimits{
			MaxFindings: maxFindings,
			MaxBytes:    maximumResponseBytes,
		},
	}
	applyCollectionLimits(&output)
	output.ReturnedFindings = len(output.Findings)
	if err := applyByteLimit(&output); err != nil {
		return ScanRepositoryOutput{}, err
	}
	output.ReturnedFindings = len(output.Findings)
	updateTruncated(&output)
	return output, nil
}

func applyCollectionLimits(output *ScanRepositoryOutput) {
	if len(output.Findings) > output.Limits.MaxFindings {
		output.Findings = output.Findings[:output.Limits.MaxFindings]
	}
	output.Omitted.Findings = output.TotalFindings - len(output.Findings)

	if len(output.Warnings) > maximumWarnings {
		output.Omitted.Warnings += len(output.Warnings) - maximumWarnings
		output.Warnings = output.Warnings[:maximumWarnings]
	}
	if len(output.Failures) > maximumFailures {
		output.Omitted.Failures += len(output.Failures) - maximumFailures
		output.Failures = output.Failures[:maximumFailures]
	}
	if len(output.Metrics) > maximumMetricGroups {
		for _, group := range output.Metrics[maximumMetricGroups:] {
			output.Omitted.Metrics += len(group.Metrics)
		}
		output.Omitted.MetricGroups += len(output.Metrics) - maximumMetricGroups
		output.Metrics = output.Metrics[:maximumMetricGroups]
	}
	for index := range output.Metrics {
		if len(output.Metrics[index].Metrics) <= maximumMetricsPerGroup {
			continue
		}
		output.Omitted.Metrics += len(output.Metrics[index].Metrics) - maximumMetricsPerGroup
		output.Metrics[index].Metrics = output.Metrics[index].Metrics[:maximumMetricsPerGroup]
	}
}

func applyByteLimit(output *ScanRepositoryOutput) error {
	for {
		output.ReturnedFindings = len(output.Findings)
		encoded, err := json.Marshal(output)
		if err != nil {
			return fmt.Errorf("encode response: %w", err)
		}
		if len(encoded) <= output.Limits.MaxBytes {
			return nil
		}

		if removeLargestMetric(output) {
			continue
		}
		removedSnippets := 0
		for index := range output.Findings {
			if output.Findings[index].CodeSnippet != nil {
				output.Findings[index].CodeSnippet = nil
				removedSnippets++
			}
		}
		if removedSnippets > 0 {
			output.Omitted.CodeSnippets += removedSnippets
			continue
		}
		if len(output.Warnings) > 0 {
			keep := len(output.Warnings) / 2
			output.Omitted.Warnings += len(output.Warnings) - keep
			output.Warnings = output.Warnings[:keep]
			continue
		}
		if len(output.Findings) > 0 {
			keep := len(output.Findings) / 2
			output.Findings = output.Findings[:keep]
			output.Omitted.Findings = output.TotalFindings - keep
			continue
		}
		if len(output.Failures) > 0 {
			keep := len(output.Failures) / 2
			output.Omitted.Failures += len(output.Failures) - keep
			output.Failures = output.Failures[:keep]
			continue
		}
		return fmt.Errorf("response metadata exceeds %d-byte limit", output.Limits.MaxBytes)
	}
}

func removeLargestMetric(output *ScanRepositoryOutput) bool {
	groupIndex, metricIndex, largestSize := -1, -1, -1
	for currentGroup := range output.Metrics {
		for currentMetric := range output.Metrics[currentGroup].Metrics {
			encoded, err := json.Marshal(output.Metrics[currentGroup].Metrics[currentMetric])
			size := len(encoded)
			if err != nil {
				size = maximumResponseBytes + 1
			}
			if size > largestSize {
				groupIndex, metricIndex, largestSize = currentGroup, currentMetric, size
			}
		}
	}
	if groupIndex < 0 {
		return false
	}

	metrics := output.Metrics[groupIndex].Metrics
	output.Metrics[groupIndex].Metrics = append(metrics[:metricIndex], metrics[metricIndex+1:]...)
	output.Omitted.Metrics++
	if len(output.Metrics[groupIndex].Metrics) == 0 {
		output.Metrics = append(output.Metrics[:groupIndex], output.Metrics[groupIndex+1:]...)
		output.Omitted.MetricGroups++
	}
	return true
}

func updateTruncated(output *ScanRepositoryOutput) {
	output.Truncated = output.Omitted.Findings > 0 || output.Omitted.Warnings > 0 ||
		output.Omitted.Failures > 0 || output.Omitted.MetricGroups > 0 ||
		output.Omitted.Metrics > 0 || output.Omitted.CodeSnippets > 0
}

func sortFindings(findings []ScanFinding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.AnalyzerID != b.AnalyzerID {
			return a.AnalyzerID < b.AnalyzerID
		}
		if a.Location.Path != b.Location.Path {
			return a.Location.Path < b.Location.Path
		}
		lineA, lineB := 0, 0
		if a.Location.Line != nil {
			lineA = *a.Location.Line
		}
		if b.Location.Line != nil {
			lineB = *b.Location.Line
		}
		if lineA != lineB {
			return lineA < lineB
		}
		if a.Message != b.Message {
			return a.Message < b.Message
		}
		return a.Fingerprint < b.Fingerprint
	})
}

func stableJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%T", value)
	}
	return string(encoded)
}
