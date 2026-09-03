package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/service"
)

// sortMode orders the findings table. Every mode falls back to a stable
// tiebreak so repeated renders of the same result set never reorder rows.
type sortMode int

const (
	sortBySeverity sortMode = iota
	sortByFile
	sortByDebt
	sortByCategory
)

var sortModeNames = map[sortMode]string{
	sortBySeverity: "severity",
	sortByFile:     "file",
	sortByDebt:     "debt hours",
	sortByCategory: "category",
}

func (s sortMode) String() string {
	if name, ok := sortModeNames[s]; ok {
		return name
	}
	return "severity"
}

func (s sortMode) next() sortMode {
	return sortMode((int(s) + 1) % len(sortModeNames))
}

// resultsFilter is the combined search, severity, and category state applied to
// a scan result. The zero value shows every finding ordered by severity.
type resultsFilter struct {
	query      string
	severities map[string]bool
	category   string
	sort       sortMode
}

func (f resultsFilter) active() bool {
	return f.query != "" || len(f.severities) > 0 || f.category != ""
}

// toggleSeverity adds or removes one severity from the filter. An empty set
// means "every severity" rather than "no severities", so clearing the last
// active entry restores the unfiltered view.
func (f *resultsFilter) toggleSeverity(severity string) {
	normalized := service.NormalizeSeverity(severity)
	if f.severities == nil {
		f.severities = map[string]bool{}
	}
	if f.severities[normalized] {
		delete(f.severities, normalized)
		if len(f.severities) == 0 {
			f.severities = nil
		}
		return
	}
	f.severities[normalized] = true
}

func (f resultsFilter) allowsSeverity(severity string) bool {
	if len(f.severities) == 0 {
		return true
	}
	return f.severities[service.NormalizeSeverity(severity)]
}

// clear resets search, severity, and category selections while preserving the
// chosen sort order, which is a display preference rather than a filter.
func (f *resultsFilter) clear() {
	f.query = ""
	f.severities = nil
	f.category = ""
}

// matchesQuery reports whether a finding matches the free-text search. The
// search spans the fields a reader can see in the table and detail pane.
func matchesQuery(issue models.TechnicalDebtIssue, query string) bool {
	if query == "" {
		return true
	}
	needle := strings.ToLower(query)
	fields := []string{
		issue.Message,
		issue.FilePath,
		issue.Category,
		issue.IssueType,
		issue.ToolName,
	}
	if issue.Description != nil {
		fields = append(fields, *issue.Description)
	}
	if issue.ToolRuleID != nil {
		fields = append(fields, *issue.ToolRuleID)
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
}

// applyResultsFilter selects and orders the findings a reader should see. It
// never mutates the input slice so the unfiltered scan result stays intact for
// the next filter change.
func applyResultsFilter(issues []models.TechnicalDebtIssue, filter resultsFilter) []models.TechnicalDebtIssue {
	filtered := make([]models.TechnicalDebtIssue, 0, len(issues))
	for _, issue := range issues {
		if !filter.allowsSeverity(issue.Severity) {
			continue
		}
		if filter.category != "" && !strings.EqualFold(issue.Category, filter.category) {
			continue
		}
		if !matchesQuery(issue, filter.query) {
			continue
		}
		filtered = append(filtered, issue)
	}
	sortIssues(filtered, filter.sort)
	return filtered
}

// sortIssues orders findings in place. The selected mode is the primary key;
// severity, file, and line provide deterministic tie-breaking.
func sortIssues(issues []models.TechnicalDebtIssue, mode sortMode) {
	sort.SliceStable(issues, func(i, j int) bool {
		left, right := issues[i], issues[j]
		switch mode {
		case sortByFile:
			if left.FilePath != right.FilePath {
				return left.FilePath < right.FilePath
			}
		case sortByDebt:
			if left.TechnicalDebtHours != right.TechnicalDebtHours {
				return left.TechnicalDebtHours > right.TechnicalDebtHours
			}
		case sortByCategory:
			if !strings.EqualFold(left.Category, right.Category) {
				return strings.ToLower(left.Category) < strings.ToLower(right.Category)
			}
		}

		leftRank, rightRank := service.SeverityRank(left.Severity), service.SeverityRank(right.Severity)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if left.FilePath != right.FilePath {
			return left.FilePath < right.FilePath
		}
		return lineNumber(left) < lineNumber(right)
	})
}

func lineNumber(issue models.TechnicalDebtIssue) int {
	if issue.LineNumber == nil {
		return 0
	}
	return *issue.LineNumber
}

// resultsSummary is the at-a-glance header derived from a scan result.
type resultsSummary struct {
	total          int
	severityCounts map[string]int
	debtHours      float64
	filesAffected  int
}

func summarizeIssues(issues []models.TechnicalDebtIssue) resultsSummary {
	summary := resultsSummary{
		total:          len(issues),
		severityCounts: map[string]int{},
	}
	files := map[string]struct{}{}
	for _, issue := range issues {
		summary.severityCounts[service.NormalizeSeverity(issue.Severity)]++
		summary.debtHours += issue.TechnicalDebtHours
		files[issue.FilePath] = struct{}{}
	}
	summary.filesAffected = len(files)
	return summary
}

// issueCategories lists the distinct categories present in a scan result so the
// category filter only ever offers values the reader can actually select.
func issueCategories(issues []models.TechnicalDebtIssue) []string {
	seen := map[string]struct{}{}
	categories := make([]string, 0)
	for _, issue := range issues {
		if issue.Category == "" {
			continue
		}
		key := strings.ToLower(issue.Category)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		categories = append(categories, issue.Category)
	}
	sort.Slice(categories, func(i, j int) bool {
		return strings.ToLower(categories[i]) < strings.ToLower(categories[j])
	})
	return categories
}

// nextCategory cycles through "every category" and each available category.
func nextCategory(current string, categories []string) string {
	if len(categories) == 0 {
		return ""
	}
	if current == "" {
		return categories[0]
	}
	for index, category := range categories {
		if strings.EqualFold(category, current) {
			if index+1 >= len(categories) {
				return ""
			}
			return categories[index+1]
		}
	}
	return ""
}

// issueIdentity is the stable key used to keep the same finding selected while
// filters change. The scanner fingerprint survives re-sorting and re-filtering;
// the location fallback covers findings recorded before fingerprints existed.
func issueIdentity(issue models.TechnicalDebtIssue) string {
	if issue.FingerprintHash != "" {
		return issue.FingerprintHash
	}
	return fmt.Sprintf("%s:%d:%s", issue.FilePath, lineNumber(issue), issue.Message)
}

// indexOfIdentity locates a finding within a filtered slice, returning -1 when
// the current selection has been filtered out.
func indexOfIdentity(issues []models.TechnicalDebtIssue, identity string) int {
	if identity == "" {
		return -1
	}
	for index, issue := range issues {
		if issueIdentity(issue) == identity {
			return index
		}
	}
	return -1
}

// filterDescription renders the active filters for the status line. It returns
// an empty string when nothing is filtered so the caller can omit the row.
func filterDescription(filter resultsFilter, matched, total int) string {
	if !filter.active() {
		return ""
	}
	parts := make([]string, 0, 3)
	if filter.query != "" {
		parts = append(parts, fmt.Sprintf("search %q", filter.query))
	}
	if len(filter.severities) > 0 {
		selected := make([]string, 0, len(filter.severities))
		for _, severity := range service.SeverityOrder() {
			if filter.severities[severity] {
				selected = append(selected, severity)
			}
		}
		parts = append(parts, "severity "+strings.Join(selected, "+"))
	}
	if filter.category != "" {
		parts = append(parts, "category "+filter.category)
	}
	return fmt.Sprintf("%s  ·  %d of %d findings", strings.Join(parts, "  ·  "), matched, total)
}

// exportFileName builds the timestamped name used when exporting the findings
// currently in view.
func exportFileName(repositoryPath string, stamp string) string {
	base := filepath.Base(strings.TrimSuffix(repositoryPath, string(filepath.Separator)))
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "repository"
	}
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, base)
	return fmt.Sprintf("debtdrone-%s-%s.json", safe, stamp)
}
