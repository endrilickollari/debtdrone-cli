package tui

import (
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stringPtr(value string) *string { return &value }

func intPtr(value int) *int { return &value }

func sampleIssues() []models.TechnicalDebtIssue {
	return []models.TechnicalDebtIssue{
		{
			FingerprintHash:    "fp-low",
			FilePath:           "internal/app/server.go",
			LineNumber:         intPtr(10),
			Severity:           "low",
			Category:           "complexity",
			Message:            "function is slightly long",
			TechnicalDebtHours: 0.5,
		},
		{
			FingerprintHash:    "fp-critical",
			FilePath:           "cmd/main.go",
			LineNumber:         intPtr(42),
			Severity:           "critical",
			Category:           "security",
			Message:            "hardcoded credential detected",
			Description:        stringPtr("Move the secret into an environment variable"),
			TechnicalDebtHours: 4,
		},
		{
			FingerprintHash:    "fp-high",
			FilePath:           "internal/app/handler.go",
			LineNumber:         intPtr(7),
			Severity:           "high",
			Category:           "complexity",
			Message:            "cyclomatic complexity exceeds threshold",
			TechnicalDebtHours: 2,
		},
	}
}

func TestApplyResultsFilterOrdersBySeverityByDefault(t *testing.T) {
	filtered := applyResultsFilter(sampleIssues(), resultsFilter{})

	require.Len(t, filtered, 3)
	assert.Equal(t, "critical", filtered[0].Severity)
	assert.Equal(t, "high", filtered[1].Severity)
	assert.Equal(t, "low", filtered[2].Severity)
}

func TestApplyResultsFilterDoesNotMutateTheScanResult(t *testing.T) {
	issues := sampleIssues()
	original := issues[0].FingerprintHash

	applyResultsFilter(issues, resultsFilter{sort: sortByDebt})

	assert.Equal(t, original, issues[0].FingerprintHash, "filtering must leave the scan result untouched")
}

func TestApplyResultsFilterCombinesSearchSeverityAndCategory(t *testing.T) {
	issues := sampleIssues()

	filter := resultsFilter{}
	filter.toggleSeverity("critical")
	filter.toggleSeverity("high")
	assert.Len(t, applyResultsFilter(issues, filter), 2)

	filter.category = "complexity"
	filtered := applyResultsFilter(issues, filter)
	require.Len(t, filtered, 1)
	assert.Equal(t, "fp-high", filtered[0].FingerprintHash)

	filter.query = "hardcoded"
	assert.Empty(t, applyResultsFilter(issues, filter), "combined filters must intersect, not union")
}

func TestSearchMatchesMessagePathAndDescription(t *testing.T) {
	issues := sampleIssues()

	assert.Len(t, applyResultsFilter(issues, resultsFilter{query: "CREDENTIAL"}), 1, "search is case-insensitive")
	assert.Len(t, applyResultsFilter(issues, resultsFilter{query: "internal/app"}), 2, "search covers file paths")
	assert.Len(t, applyResultsFilter(issues, resultsFilter{query: "environment variable"}), 1, "search covers descriptions")
	assert.Empty(t, applyResultsFilter(issues, resultsFilter{query: "no-such-text"}))
}

func TestToggleSeverityClearsBackToEveryFindingAndFilterClearsAll(t *testing.T) {
	filter := resultsFilter{}
	filter.toggleSeverity("high")
	assert.True(t, filter.active())
	assert.Len(t, applyResultsFilter(sampleIssues(), filter), 1)

	filter.toggleSeverity("high")
	assert.False(t, filter.active(), "removing the last severity restores the unfiltered view")
	assert.Len(t, applyResultsFilter(sampleIssues(), filter), 3)

	filter.query = "main"
	filter.category = "security"
	filter.sort = sortByFile
	filter.clear()
	assert.False(t, filter.active())
	assert.Equal(t, sortByFile, filter.sort, "clearing filters preserves the chosen sort order")
}

func TestSortModesAreDeterministicAndKeepSeverityAsTiebreak(t *testing.T) {
	byFile := applyResultsFilter(sampleIssues(), resultsFilter{sort: sortByFile})
	assert.Equal(t, "cmd/main.go", byFile[0].FilePath)

	byDebt := applyResultsFilter(sampleIssues(), resultsFilter{sort: sortByDebt})
	assert.Equal(t, 4.0, byDebt[0].TechnicalDebtHours)
	assert.Equal(t, 0.5, byDebt[2].TechnicalDebtHours)

	// Equal debt falls back to severity before the remaining deterministic keys.
	tied := []models.TechnicalDebtIssue{
		{FilePath: "a.go", Severity: "low", TechnicalDebtHours: 1},
		{FilePath: "a.go", Severity: "critical", TechnicalDebtHours: 1},
	}
	sortIssues(tied, sortByDebt)
	assert.Equal(t, "critical", tied[0].Severity)
}

func TestSearchInputAcceptsPrintableUnicodeAndPathCharacters(t *testing.T) {
	for _, input := range []string{"é", `\`, "+", "[", "]", " "} {
		assert.True(t, isEditableChar(input), "expected %q to be accepted", input)
	}
	for _, input := range []string{"enter", "\n", ""} {
		assert.False(t, isEditableChar(input), "expected %q to be rejected", input)
	}
}

func TestIssueDetailDoesNotPresentGenericDescriptionAsRemediation(t *testing.T) {
	description := "Line Coverage: 20% (2/10 lines)"
	issue := models.TechnicalDebtIssue{
		FilePath:    "coverage.go",
		Severity:    "low",
		Message:     "File has low test coverage",
		Description: &description,
	}

	detail := formatIssueDetail(&issue, 100)
	assert.Contains(t, detail, "Details")
	assert.Contains(t, detail, description)
	assert.NotContains(t, detail, "Recommended Fix")
}

func TestSummarizeIssuesReportsTotalsFilesAndDebt(t *testing.T) {
	summary := summarizeIssues(sampleIssues())

	assert.Equal(t, 3, summary.total)
	assert.Equal(t, 3, summary.filesAffected)
	assert.InDelta(t, 6.5, summary.debtHours, 0.001)
	assert.Equal(t, 1, summary.severityCounts["critical"])
	assert.Equal(t, 0, summary.severityCounts["medium"])
}

func TestIssueCategoriesAreDistinctSortedAndCycleThroughAll(t *testing.T) {
	categories := issueCategories(sampleIssues())
	assert.Equal(t, []string{"complexity", "security"}, categories)

	assert.Equal(t, "complexity", nextCategory("", categories))
	assert.Equal(t, "security", nextCategory("complexity", categories))
	assert.Equal(t, "", nextCategory("security", categories), "cycling past the last category shows every category again")
	assert.Equal(t, "", nextCategory("complexity", nil))
}

func TestIssueIdentityFallsBackToLocationWithoutFingerprint(t *testing.T) {
	withFingerprint := models.TechnicalDebtIssue{FingerprintHash: "fp", FilePath: "a.go", Message: "m"}
	assert.Equal(t, "fp", issueIdentity(withFingerprint))

	legacy := models.TechnicalDebtIssue{FilePath: "a.go", LineNumber: intPtr(3), Message: "m"}
	assert.Equal(t, "a.go:3:m", issueIdentity(legacy))
}

func TestFilterDescriptionSummarizesActiveFiltersOnly(t *testing.T) {
	assert.Empty(t, filterDescription(resultsFilter{}, 3, 3))

	filter := resultsFilter{query: "creds", category: "security"}
	filter.toggleSeverity("critical")
	description := filterDescription(filter, 1, 3)
	assert.Contains(t, description, `search "creds"`)
	assert.Contains(t, description, "severity critical")
	assert.Contains(t, description, "category security")
	assert.Contains(t, description, "1 of 3 findings")
}

func TestExportFileNameSanitizesRepositoryPath(t *testing.T) {
	assert.Equal(t, "debtdrone-my-repo-20260903-120000.json", exportFileName("/home/user/my repo", "20260903-120000"))
	assert.Equal(t, "debtdrone-repository-20260903-120000.json", exportFileName(".", "20260903-120000"))
}
