package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/scancore"
	"github.com/google/uuid"
)

type trivyExecutor interface {
	LookPath() error
	Scan(context.Context, string) ([]byte, error)
}

type systemTrivyExecutor struct{}

func (systemTrivyExecutor) LookPath() error {
	_, err := exec.LookPath("trivy")
	return err
}

func (systemTrivyExecutor) Scan(ctx context.Context, root string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "trivy", "fs",
		"--scanners", "vuln,secret",
		"--format", "json",
		"--quiet",
		root)
	return executeTrivyCommand(cmd)
}

func executeTrivyCommand(cmd *exec.Cmd) ([]byte, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err == nil {
		return output, nil
	}
	if diagnostic := strings.TrimSpace(stderr.String()); diagnostic != "" {
		return output, fmt.Errorf("%w: %s", err, diagnostic)
	}
	return output, err
}

type TrivyAnalyzer struct {
	executor trivyExecutor
}

const (
	missingTrivyWarning  = "Security scanner (Trivy) not installed — security scan was skipped. Install Trivy to enable security scanning."
	inMemoryTrivyWarning = "Security scan skipped — Trivy requires filesystem access but this is an in-memory repository."
)

func NewTrivyAnalyzer() *TrivyAnalyzer {
	return &TrivyAnalyzer{executor: systemTrivyExecutor{}}
}

func (a *TrivyAnalyzer) Name() string {
	return "Trivy Security Scanner"
}

type TrivyOutput struct {
	Results []TrivyResult `json:"Results"`
}

type TrivyResult struct {
	Target          string               `json:"Target"`
	Vulnerabilities []TrivyVulnerability `json:"Vulnerabilities"`
	Secrets         []TrivySecret        `json:"Secrets"`
}

type TrivyVulnerability struct {
	VulnerabilityID  string `json:"VulnerabilityID"`
	PkgName          string `json:"PkgName"`
	InstalledVersion string `json:"InstalledVersion"`
	FixedVersion     string `json:"FixedVersion"`
	Title            string `json:"Title"`
	Description      string `json:"Description"`
	Severity         string `json:"Severity"`
	PrimaryURL       string `json:"PrimaryURL"`
}

type TrivySecret struct {
	RuleID    string `json:"RuleID"`
	Category  string `json:"Category"`
	Severity  string `json:"Severity"`
	Title     string `json:"Title"`
	StartLine int    `json:"StartLine"`
	EndLine   int    `json:"EndLine"`
	Match     string `json:"Match"`
}

func (a *TrivyAnalyzer) Analyze(ctx context.Context, repo *git.Repository) (*scancore.Result, error) {
	analysisRunID, ok := ctx.Value("analysisRunID").(uuid.UUID)
	if !ok {
		return nil, fmt.Errorf("analysisRunID not found in context")
	}

	repositoryID, ok := ctx.Value("repositoryID").(uuid.UUID)
	if !ok {
		return nil, fmt.Errorf("repositoryID not found in context")
	}

	userID, ok := ctx.Value("userID").(uuid.UUID)
	if !ok {
		return nil, fmt.Errorf("userID not found in context")
	}

	executor := a.executor
	if executor == nil {
		executor = systemTrivyExecutor{}
	}
	if err := executor.LookPath(); err != nil {
		return emptySecurityResult(false, "trivy not installed", missingTrivyWarning), nil
	}

	if repo.Path == "" {
		return emptySecurityResult(true, "in-memory repository not supported", inMemoryTrivyWarning), nil
	}

	scanRoot := repo.Path
	cleanup := func() {}
	var selectedTargets map[string]struct{}
	if targetFiles, _, selectionBound := scancore.TargetFiles(ctx); selectionBound {
		if len(targetFiles) == 0 {
			return emptySecurityResult(true, "no analyzable target files"), nil
		}
		selectedTargets = make(map[string]struct{}, len(targetFiles))
		for _, target := range targetFiles {
			selectedTargets[canonicalTrivyTarget("", target)] = struct{}{}
		}
		var err error
		scanRoot, cleanup, err = stageTargetFiles(ctx, repo, targetFiles)
		if err != nil {
			return nil, err
		}
		defer cleanup()
	}

	output, scanErr := executor.Scan(ctx, scanRoot)
	if scanErr != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		if len(output) == 0 {
			return nil, fmt.Errorf("trivy execution failed: %w", scanErr)
		}
	}

	var trivyResult TrivyOutput
	if err := json.Unmarshal(output, &trivyResult); err != nil {
		return nil, fmt.Errorf("failed to parse trivy output: %w", err)
	}

	var issues []models.TechnicalDebtIssue
	now := time.Now()

	for _, result := range trivyResult.Results {
		if selectedTargets != nil {
			canonicalTarget := canonicalTrivyTarget(scanRoot, result.Target)
			if _, selected := selectedTargets[canonicalTarget]; !selected {
				continue
			}
			result.Target = strings.TrimPrefix(canonicalTarget, "/")
		}
		for _, vuln := range result.Vulnerabilities {
			message := fmt.Sprintf("%s: %s (%s)", vuln.VulnerabilityID, vuln.Title, vuln.PkgName)

			description := vuln.Description
			if vuln.FixedVersion != "" {
				description += fmt.Sprintf("\n\nFixed Version: %s", vuln.FixedVersion)
			}
			if vuln.PrimaryURL != "" {
				description += fmt.Sprintf("\nMore info: %s", vuln.PrimaryURL)
			}

			issues = append(issues, models.TechnicalDebtIssue{
				ID:                 uuid.New(),
				UserID:             userID,
				RepositoryID:       repositoryID,
				AnalysisRunID:      analysisRunID,
				FilePath:           result.Target,
				IssueType:          "security",
				Category:           "vulnerability",
				Severity:           mapSeverity(vuln.Severity),
				Message:            message,
				Description:        &description,
				ToolName:           "trivy",
				ToolRuleID:         &vuln.VulnerabilityID,
				ConfidenceScore:    1.0,
				TechnicalDebtHours: calculateSecurityDebt(vuln.Severity),
				EffortMultiplier:   1.0,
				Status:             "open",
				CreatedAt:          now,
				UpdatedAt:          now,
			})
		}

		for _, secret := range result.Secrets {
			desc := fmt.Sprintf("Secret detected: %s\nCategory: %s\nThis credential should be removed from the codebase and rotated immediately.",
				secret.Title, secret.Category)

			ruleID := secret.RuleID

			issues = append(issues, models.TechnicalDebtIssue{
				ID:                 uuid.New(),
				UserID:             userID,
				RepositoryID:       repositoryID,
				AnalysisRunID:      analysisRunID,
				FilePath:           result.Target,
				LineNumber:         &secret.StartLine,
				IssueType:          "security",
				Category:           "secret",
				Severity:           "critical",
				Message:            fmt.Sprintf("Hardcoded Secret: %s", secret.Title),
				Description:        &desc,
				ToolName:           "trivy",
				ToolRuleID:         &ruleID,
				ConfidenceScore:    1.0,
				TechnicalDebtHours: 4.0,
				EffortMultiplier:   1.0,
				Status:             "open",
				CreatedAt:          now,
				UpdatedAt:          now,
			})
		}
	}

	metrics := securityMetrics(issues, true)
	warnings := []string(nil)
	if scanErr != nil {
		warnings = append(warnings, fmt.Sprintf("Trivy exited with an error; valid partial results were retained: %v", scanErr))
	}

	return &scancore.Result{
		Issues:   issues,
		Metrics:  metrics,
		Warnings: warnings,
	}, nil
}

func securityMetrics(issues []models.TechnicalDebtIssue, available bool) map[string]interface{} {
	return map[string]interface{}{
		"security_issues_count":   len(issues),
		"vulnerabilities_count":   countByCategory(issues, "vulnerability"),
		"secrets_count":           countByCategory(issues, "secret"),
		"security_critical_count": countBySeverity(issues, "critical"),
		"security_high_count":     countBySeverity(issues, "high"),
		"security_medium_count":   countBySeverity(issues, "medium"),
		"security_low_count":      countBySeverity(issues, "low"),
		"trivy_available":         available,
	}
}

func canonicalTrivyTarget(scanRoot, target string) string {
	target = strings.TrimSpace(target)
	if scanRoot != "" && filepath.IsAbs(target) {
		relative, err := filepath.Rel(scanRoot, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return ""
		}
		target = relative
	}
	target = strings.ReplaceAll(target, "\\", "/")
	cleaned := path.Clean(target)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return "/" + strings.TrimPrefix(cleaned, "/")
}

func emptySecurityResult(available bool, reason string, warnings ...string) *scancore.Result {
	metrics := securityMetrics(nil, available)
	metrics["skip_reason"] = reason
	return &scancore.Result{Issues: []models.TechnicalDebtIssue{}, Metrics: metrics, Warnings: warnings}
}

func stageTargetFiles(ctx context.Context, repo *git.Repository, targetFiles []string) (string, func(), error) {
	root, err := os.MkdirTemp("", "debtdrone-trivy-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create Trivy target directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	fail := func(err error) (string, func(), error) {
		cleanup()
		return "", func() {}, err
	}

	for _, target := range targetFiles {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		relative := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(target, "/")))
		if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fail(fmt.Errorf("invalid Trivy target path %q", target))
		}
		destination := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return fail(fmt.Errorf("create Trivy target directory for %q: %w", target, err))
		}
		if err := copyTargetFile(ctx, repo, target, destination); err != nil {
			return fail(err)
		}
	}
	return root, cleanup, nil
}

func copyTargetFile(ctx context.Context, repo *git.Repository, sourcePath, destination string) error {
	source, err := repo.FS.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open Trivy target %q: %w", sourcePath, err)
	}
	defer source.Close()

	target, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create staged Trivy target %q: %w", sourcePath, err)
	}
	defer target.Close()

	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			if _, err := target.Write(buffer[:read]); err != nil {
				return fmt.Errorf("stage Trivy target %q: %w", sourcePath, err)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read Trivy target %q: %w", sourcePath, readErr)
		}
	}
}

func mapSeverity(trivySeverity string) string {
	switch strings.ToUpper(trivySeverity) {
	case "CRITICAL":
		return "critical"
	case "HIGH":
		return "high"
	case "MEDIUM":
		return "medium"
	case "LOW":
		return "low"
	default:
		return "info"
	}
}

func calculateSecurityDebt(severity string) float64 {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return 8.0
	case "HIGH":
		return 4.0
	case "MEDIUM":
		return 2.0
	default:
		return 1.0
	}
}

func countByCategory(issues []models.TechnicalDebtIssue, category string) int {
	count := 0
	for _, issue := range issues {
		if issue.Category == category {
			count++
		}
	}
	return count
}

func countBySeverity(issues []models.TechnicalDebtIssue, severity string) int {
	count := 0
	for _, issue := range issues {
		if issue.Severity == severity {
			count++
		}
	}
	return count
}
