package security

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/analysis"
	"github.com/endrilickollari/debtdrone-cli/internal/git"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

type TrivyAnalyzer struct{}

func NewTrivyAnalyzer() *TrivyAnalyzer {
	return &TrivyAnalyzer{}
}

func (a *TrivyAnalyzer) Name() string {
	return "Trivy Security Scanner"
}

// Trivy JSON Output Structures
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

func (a *TrivyAnalyzer) Analyze(ctx context.Context, repo *git.Repository) (*analysis.Result, error) {
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

	if _, err := exec.LookPath("trivy"); err != nil {
		log.Println("⚠️  Trivy not installed - skipping security scan. Install with: brew install aquasecurity/trivy/trivy")
		return &analysis.Result{
			Issues: []models.TechnicalDebtIssue{},
			Metrics: map[string]interface{}{
				"security_issues_count": 0,
				"trivy_available":       false,
				"skip_reason":           "trivy not installed",
			},
		}, nil
	}

	if repo.Path == "" {
		log.Println("⚠️  Trivy requires filesystem path - skipping in-memory repository")
		return &analysis.Result{
			Issues: []models.TechnicalDebtIssue{},
			Metrics: map[string]interface{}{
				"security_issues_count": 0,
				"trivy_available":       true,
				"skip_reason":           "in-memory repository not supported",
			},
		}, nil
	}

	cmd := exec.CommandContext(ctx, "trivy", "fs",
		"--scanners", "vuln,secret",
		"--format", "json",
		"--quiet",
		repo.Path)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) == 0 {
			return nil, fmt.Errorf("trivy execution failed: %w, output: %s", err, string(output))
		}
	}

	var trivyResult TrivyOutput
	if err := json.Unmarshal(output, &trivyResult); err != nil {
		return nil, fmt.Errorf("failed to parse trivy output: %w", err)
	}

	var issues []models.TechnicalDebtIssue
	now := time.Now()

	for _, result := range trivyResult.Results {
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

	metrics := map[string]interface{}{
		"security_issues_count": len(issues),
		"vulnerabilities_count": countByCategory(issues, "vulnerability"),
		"secrets_count":         countByCategory(issues, "secret"),
		"critical_issues_count": countBySeverity(issues, "critical"),
		"high_issues_count":     countBySeverity(issues, "high"),
		"medium_issues_count":   countBySeverity(issues, "medium"),
		"low_issues_count":      countBySeverity(issues, "low"),
		"trivy_available":       true,
	}

	return &analysis.Result{
		Issues:  issues,
		Metrics: metrics,
	}, nil
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
