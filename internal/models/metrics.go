package models

import (
	"time"

	"github.com/google/uuid"
)

type RepositoryMetricsSnapshot struct {
	ID                     uuid.UUID `json:"id" db:"id"`
	UserID                 uuid.UUID `json:"user_id" db:"user_id"`
	RepositoryID           uuid.UUID `json:"repository_id" db:"repository_id"`
	SnapshotDate           time.Time `json:"snapshot_date" db:"snapshot_date"`
	TotalIssuesCount       int       `json:"total_issues_count" db:"total_issues_count"`
	CriticalIssuesCount    int       `json:"critical_issues_count" db:"critical_issues_count"`
	HighIssuesCount        int       `json:"high_issues_count" db:"high_issues_count"`
	MediumIssuesCount      int       `json:"medium_issues_count" db:"medium_issues_count"`
	LowIssuesCount         int       `json:"low_issues_count" db:"low_issues_count"`
	TechnicalDebtHours     float64   `json:"technical_debt_hours" db:"technical_debt_hours"`
	TestCoveragePercentage float64   `json:"test_coverage_percentage" db:"test_coverage_percentage"`
	DuplicationPercentage  float64   `json:"duplication_percentage" db:"duplication_percentage"`
	ComplexityScore        float64   `json:"complexity_score" db:"complexity_score"`
	CreatedAt              time.Time `json:"created_at" db:"created_at"`
}

type UserActivity struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	UserID       uuid.UUID              `json:"user_id" db:"user_id"`
	ActivityType string                 `json:"activity_type" db:"activity_type"`
	ResourceType *string                `json:"resource_type,omitempty" db:"resource_type"`
	ResourceID   *uuid.UUID             `json:"resource_id,omitempty" db:"resource_id"`
	ActivityDate time.Time              `json:"activity_date" db:"activity_date"`
	Metadata     map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
}

type DashboardMetricsCache struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	UserID       uuid.UUID              `json:"user_id" db:"user_id"`
	MetricType   string                 `json:"metric_type" db:"metric_type"`
	MetricValue  map[string]interface{} `json:"metric_value" db:"metric_value"`
	CalculatedAt time.Time              `json:"calculated_at" db:"calculated_at"`
	ExpiresAt    time.Time              `json:"expires_at" db:"expires_at"`
}

type MetricsTrend struct {
	CurrentValue  float64 `json:"current_value"`
	PreviousValue float64 `json:"previous_value"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	Direction     string  `json:"direction"`
}

type DashboardStats struct {
	TotalRepositories int          `json:"total_repositories"`
	TotalIssues       int          `json:"total_issues"`
	AvgCodeCoverage   float64      `json:"avg_code_coverage"`
	AvgComplexity     float64      `json:"avg_complexity"`
	ActiveUsersCount  int          `json:"active_users_count"`
	ResolvedThisWeek  int          `json:"resolved_this_week"`
	ReportsGenerated  int          `json:"reports_generated"`
	RepositoriesTrend MetricsTrend `json:"repositories_trend"`
	IssuesTrend       MetricsTrend `json:"issues_trend"`
	CoverageTrend     MetricsTrend `json:"coverage_trend"`
	ComplexityTrend   MetricsTrend `json:"complexity_trend"`
}

type ActiveUsersSummary struct {
	UserID             uuid.UUID `json:"user_id" db:"user_id"`
	ActivityDate       time.Time `json:"activity_date" db:"activity_date"`
	ActivityTypesCount int       `json:"activity_types_count" db:"activity_types_count"`
	TotalActivities    int       `json:"total_activities" db:"total_activities"`
}

type WeeklyResolvedIssues struct {
	UserID        uuid.UUID `json:"user_id" db:"user_id"`
	WeekStart     time.Time `json:"week_start" db:"week_start"`
	ResolvedCount int       `json:"resolved_count" db:"resolved_count"`
}

const (
	ActivityTypeScanTriggered   = "scan_triggered"
	ActivityTypeIssueResolved   = "issue_resolved"
	ActivityTypeCommentAdded    = "comment_added"
	ActivityTypeRepoSynced      = "repo_synced"
	ActivityTypeConfigCreated   = "config_created"
	ActivityTypeConfigUpdated   = "config_updated"
	ActivityTypeIssueAssigned   = "issue_assigned"
	ActivityTypeReportGenerated = "report_generated"
)

const (
	MetricTypeTotalRepos     = "total_repos"
	MetricTypeTotalIssues    = "total_issues"
	MetricTypeAvgCoverage    = "avg_coverage"
	MetricTypeAvgComplexity  = "avg_complexity"
	MetricTypeActiveUsers    = "active_users"
	MetricTypeResolvedWeekly = "resolved_weekly"
	MetricTypeReportsCount   = "reports_count"
	MetricTypeTrendData      = "trend_data"
	MetricTypeDashboardStats = "dashboard_stats"
)
