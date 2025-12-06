package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID                     uuid.UUID  `json:"id" db:"id"`
	Email                  string     `json:"email" db:"email"`
	FirstName              *string    `json:"first_name" db:"first_name"`
	LastName               *string    `json:"last_name" db:"last_name"`
	FullName               *string    `json:"full_name" db:"full_name"`
	Organization           *string    `json:"organization" db:"organization"`
	AvatarURL              *string    `json:"avatar_url" db:"avatar_url"`
	TypeOfLogin            string     `json:"type_of_login" db:"type_of_login"`
	ProviderID             *string    `json:"provider_id" db:"provider_id"`
	ProviderName           *string    `json:"provider_name" db:"provider_name"`
	ProviderAccessToken    *string    `json:"-" db:"provider_access_token"`
	ProviderRefreshToken   *string    `json:"-" db:"provider_refresh_token"`
	ProviderTokenExpiresAt *time.Time `json:"-" db:"provider_token_expires_at"`
	PasswordHash           *string    `json:"-" db:"password_hash"`
	EmailVerified          bool       `json:"email_verified" db:"email_verified"`
	IsActive               bool       `json:"is_active" db:"is_active"`
	IsAdmin                bool       `json:"is_admin" db:"is_admin"`
	MFAEnabled             bool       `json:"mfa_enabled" db:"mfa_enabled"`
	MFASecret              *string    `json:"-" db:"mfa_secret"`
	LastLoginAt            *time.Time `json:"last_login_at" db:"last_login_at"`
	CreatedAt              time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at" db:"updated_at"`
	LastPasswordChangeAt   *time.Time `json:"last_password_change_at" db:"last_password_change_at"`
	FailedLoginAttempts    int        `json:"failed_login_attempts" db:"failed_login_attempts"`
	LockedUntil            *time.Time `json:"locked_until" db:"locked_until"`
}

type PendingRegistration struct {
	ID           uuid.UUID `json:"id" db:"id"`
	FirstName    string    `json:"first_name" db:"first_name"`
	LastName     string    `json:"last_name" db:"last_name"`
	Email        string    `json:"email" db:"email"`
	Organization string    `json:"organization" db:"organization"`
	PasswordHash string    `json:"-" db:"password_hash"`
	OTPCode      string    `json:"-" db:"otp_code"`
	OTPExpiresAt time.Time `json:"otp_expires_at" db:"otp_expires_at"`
	OTPAttempts  int       `json:"otp_attempts" db:"otp_attempts"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
}

type UserConfiguration struct {
	ID                        uuid.UUID  `json:"id" db:"id"`
	UserID                    uuid.UUID  `json:"user_id" db:"user_id"`
	OrganizationName          string     `json:"organization_name" db:"organization_name"`
	OrganizationURL           *string    `json:"organization_url" db:"organization_url"`
	PlatformType              string     `json:"platform_type" db:"platform_type"`
	IsConnected               bool       `json:"is_connected" db:"is_connected"`
	ConnectedAt               *time.Time `json:"connected_at" db:"connected_at"`
	AccessTokenEncrypted      *string    `json:"-" db:"access_token_encrypted"`
	RefreshTokenEncrypted     *string    `json:"-" db:"refresh_token_encrypted"`
	TokenExpiresAt            *time.Time `json:"token_expires_at" db:"token_expires_at"`
	TokenScopes               []string   `json:"token_scopes" db:"token_scopes"`
	AutoSyncEnabled           bool       `json:"auto_sync_enabled" db:"auto_sync_enabled"`
	SyncFrequencyMinutes      int        `json:"sync_frequency_minutes" db:"sync_frequency_minutes"`
	LastSyncAt                *time.Time `json:"last_sync_at" db:"last_sync_at"`
	NextSyncAt                *time.Time `json:"next_sync_at" db:"next_sync_at"`
	RepositoryFilters         *string    `json:"repository_filters" db:"repository_filters"`
	ExcludedRepositories      []string   `json:"excluded_repositories" db:"excluded_repositories"`
	IncludedRepositories      []string   `json:"included_repositories" db:"included_repositories"`
	AnalysisDepth             string     `json:"analysis_depth" db:"analysis_depth"`
	ComplexityThreshold       int        `json:"complexity_threshold" db:"complexity_threshold"`
	DuplicationThreshold      int        `json:"duplication_threshold" db:"duplication_threshold"`
	SecurityScanEnabled       bool       `json:"security_scan_enabled" db:"security_scan_enabled"`
	CoverageAnalysisEnabled   bool       `json:"coverage_analysis_enabled" db:"coverage_analysis_enabled"`
	EmailNotificationsEnabled bool       `json:"email_notifications_enabled" db:"email_notifications_enabled"`
	SlackWebhookURLEncrypted  *string    `json:"-" db:"slack_webhook_url_encrypted"`
	NotificationPreferences   *string    `json:"notification_preferences" db:"notification_preferences"`
	CreatedAt                 time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at" db:"updated_at"`
}

type UserRepository struct {
	ID                            uuid.UUID  `json:"id" db:"id"`
	UserID                        uuid.UUID  `json:"user_id" db:"user_id"`
	UserConfigID                  uuid.UUID  `json:"user_config_id" db:"user_config_id"`
	Name                          string     `json:"name" db:"name"`
	FullName                      string     `json:"full_name" db:"full_name"`
	URL                           string     `json:"url" db:"url"`
	PlatformType                  string     `json:"platform_type" db:"platform_type"`
	PrimaryLanguage               *string    `json:"primary_language" db:"primary_language"`
	SizeBytes                     *int64     `json:"size_bytes" db:"size_bytes"`
	DefaultBranch                 string     `json:"default_branch" db:"default_branch"`
	LastCommitDate                *time.Time `json:"last_commit_date" db:"last_commit_date"`
	IsPrivate                     bool       `json:"is_private" db:"is_private"`
	IsFork                        bool       `json:"is_fork" db:"is_fork"`
	LanguageBreakdown             *string    `json:"language_breakdown" db:"language_breakdown"`
	ConfigFiles                   *string    `json:"config_files" db:"config_files"`
	LastAnalysisRunID             *uuid.UUID `json:"last_analysis_run_id" db:"last_analysis_run_id"`
	AnalysisEnabled               bool       `json:"analysis_enabled" db:"analysis_enabled"`
	LastAnalysisStatus            *string    `json:"last_analysis_status" db:"last_analysis_status"`
	LastAnalysisAt                *time.Time `json:"last_analysis_at" db:"last_analysis_at"`
	LastAnalysisDurationSeconds   *int       `json:"last_analysis_duration_seconds" db:"last_analysis_duration_seconds"`
	LatestTotalTechnicalDebtHours float64    `json:"latest_total_technical_debt_hours" db:"latest_total_technical_debt_hours"`
	LatestCriticalIssuesCount     int        `json:"latest_critical_issues_count" db:"latest_critical_issues_count"`
	LatestHighIssuesCount         int        `json:"latest_high_issues_count" db:"latest_high_issues_count"`
	LatestMediumIssuesCount       int        `json:"latest_medium_issues_count" db:"latest_medium_issues_count"`
	LatestLowIssuesCount          int        `json:"latest_low_issues_count" db:"latest_low_issues_count"`
	LatestTestCoveragePercentage  float64    `json:"latest_test_coverage_percentage" db:"latest_test_coverage_percentage"`
	LatestDuplicationPercentage   float64    `json:"latest_duplication_percentage" db:"latest_duplication_percentage"`
	LatestComplexityScore         float64    `json:"latest_complexity_score" db:"latest_complexity_score"`
	CreatedAt                     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt                     time.Time  `json:"updated_at" db:"updated_at"`
}

type AnalysisRun struct {
	ID                      uuid.UUID  `json:"id" db:"id"`
	UserID                  uuid.UUID  `json:"user_id" db:"user_id"`
	RepositoryID            uuid.UUID  `json:"repository_id" db:"repository_id"`
	UserConfigID            uuid.UUID  `json:"user_config_id" db:"user_config_id"`
	RunType                 string     `json:"run_type" db:"run_type"`
	TriggerSource           *string    `json:"trigger_source" db:"trigger_source"`
	StartedAt               time.Time  `json:"started_at" db:"started_at"`
	CompletedAt             *time.Time `json:"completed_at" db:"completed_at"`
	DurationSeconds         *int       `json:"duration_seconds" db:"duration_seconds"`
	Status                  string     `json:"status" db:"status"`
	AnalysisConfig          *string    `json:"analysis_config" db:"analysis_config"`
	TotalIssuesFound        int        `json:"total_issues_found" db:"total_issues_found"`
	CriticalIssuesCount     int        `json:"critical_issues_count" db:"critical_issues_count"`
	HighIssuesCount         int        `json:"high_issues_count" db:"high_issues_count"`
	MediumIssuesCount       int        `json:"medium_issues_count" db:"medium_issues_count"`
	LowIssuesCount          int        `json:"low_issues_count" db:"low_issues_count"`
	TotalTechnicalDebtHours float64    `json:"total_technical_debt_hours" db:"total_technical_debt_hours"`
	TestCoveragePercentage  float64    `json:"test_coverage_percentage" db:"test_coverage_percentage"`
	DuplicationPercentage   float64    `json:"duplication_percentage" db:"duplication_percentage"`
	ErrorMessage            *string    `json:"error_message" db:"error_message"`
	ErrorDetails            *string    `json:"error_details" db:"error_details"`
	RepositoryName          *string    `json:"repository_name,omitempty" db:"-"`
	RepositoryFullName      *string    `json:"repository_full_name,omitempty" db:"-"`
	CreatedAt               time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`
}

type TechnicalDebtIssue struct {
	ID                 uuid.UUID  `json:"id" db:"id"`
	UserID             uuid.UUID  `json:"user_id" db:"user_id"`
	RepositoryID       uuid.UUID  `json:"repository_id" db:"repository_id"`
	AnalysisRunID      uuid.UUID  `json:"analysis_run_id" db:"analysis_run_id"`
	FilePath           string     `json:"file_path" db:"file_path"`
	LineNumber         *int       `json:"line_number" db:"line_number"`
	ColumnNumber       *int       `json:"column_number" db:"column_number"`
	IssueType          string     `json:"issue_type" db:"issue_type"`
	Severity           string     `json:"severity" db:"severity"`
	Category           string     `json:"category" db:"category"`
	Message            string     `json:"message" db:"message"`
	Description        *string    `json:"description" db:"description"`
	ToolName           string     `json:"tool_name" db:"tool_name"`
	ToolRuleID         *string    `json:"tool_rule_id" db:"tool_rule_id"`
	ConfidenceScore    float64    `json:"confidence_score" db:"confidence_score"`
	TechnicalDebtHours float64    `json:"technical_debt_hours" db:"technical_debt_hours"`
	EffortMultiplier   float64    `json:"effort_multiplier" db:"effort_multiplier"`
	Status             string     `json:"status" db:"status"`
	ResolutionReason   *string    `json:"resolution_reason" db:"resolution_reason"`
	AssignedToUserID   *uuid.UUID `json:"assigned_to_user_id" db:"assigned_to_user_id"`
	ResolvedAt         *time.Time `json:"resolved_at" db:"resolved_at"`
	ResolvedByUserID   *uuid.UUID `json:"resolved_by_user_id" db:"resolved_by_user_id"`
	IgnoreUntil        *time.Time `json:"ignore_until" db:"ignore_until"`
	Comments           []string   `json:"comments" db:"comments"`
	CodeSnippet        *string    `json:"code_snippet" db:"code_snippet"`
	SurroundingContext *string    `json:"surrounding_context" db:"surrounding_context"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
	RepositoryName     string     `json:"repository_name,omitempty" db:"-"`
	RepositoryFullName string     `json:"repository_full_name,omitempty" db:"-"`
}

type IssueComment struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	IssueID   uuid.UUID  `json:"issue_id" db:"issue_id"`
	UserID    uuid.UUID  `json:"user_id" db:"user_id"`
	Text      string     `json:"text" db:"text"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	UserName  string     `json:"user_name,omitempty" db:"-"`
	UserEmail string     `json:"user_email,omitempty" db:"-"`
}

type IssueActivityLog struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	IssueID      uuid.UUID  `json:"issue_id" db:"issue_id"`
	UserID       *uuid.UUID `json:"user_id" db:"user_id"`
	ActivityType string     `json:"activity_type" db:"activity_type"`
	Details      string     `json:"details" db:"details"`
	Metadata     *string    `json:"metadata" db:"metadata"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UserName     string     `json:"user_name,omitempty" db:"-"`
	UserEmail    string     `json:"user_email,omitempty" db:"-"`
}

type IssueTrends struct {
	ID                     uuid.UUID `json:"id" db:"id"`
	RepositoryID           uuid.UUID `json:"repository_id" db:"repository_id"`
	IssueType              string    `json:"issue_type" db:"issue_type"`
	FilePath               *string   `json:"file_path" db:"file_path"`
	TotalOccurrences       int       `json:"total_occurrences" db:"total_occurrences"`
	OpenCount              int       `json:"open_count" db:"open_count"`
	ResolvedCount          int       `json:"resolved_count" db:"resolved_count"`
	ResolvedLast30Days     int       `json:"resolved_last_30_days" db:"resolved_last_30_days"`
	AvgResolutionTimeHours *float64  `json:"avg_resolution_time_hours" db:"avg_resolution_time_hours"`
	LastCalculatedAt       time.Time `json:"last_calculated_at" db:"last_calculated_at"`
	CreatedAt              time.Time `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time `json:"updated_at" db:"updated_at"`
}

type UserSession struct {
	ID             uuid.UUID `json:"id" db:"id"`
	UserID         uuid.UUID `json:"user_id" db:"user_id"`
	SessionToken   string    `json:"session_token" db:"session_token"`
	IPAddress      *string   `json:"ip_address" db:"ip_address"`
	UserAgent      *string   `json:"user_agent" db:"user_agent"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	ExpiresAt      time.Time `json:"expires_at" db:"expires_at"`
	LastActivityAt time.Time `json:"last_activity_at" db:"last_activity_at"`
	IsActive       bool      `json:"is_active" db:"is_active"`
	DeviceInfo     *string   `json:"device_info" db:"device_info"`
}

type AuditLog struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	UserID       *uuid.UUID `json:"user_id" db:"user_id"`
	Action       string     `json:"action" db:"action"`
	ResourceType *string    `json:"resource_type" db:"resource_type"`
	ResourceID   *uuid.UUID `json:"resource_id" db:"resource_id"`
	IPAddress    *string    `json:"ip_address" db:"ip_address"`
	UserAgent    *string    `json:"user_agent" db:"user_agent"`
	Metadata     *string    `json:"metadata" db:"metadata"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

type RepositorySummary struct {
	RepositoryID                  uuid.UUID  `json:"repository_id" db:"repository_id"`
	UserID                        uuid.UUID  `json:"user_id" db:"user_id"`
	Name                          string     `json:"name" db:"name"`
	FullName                      string     `json:"full_name" db:"full_name"`
	LastAnalysisAt                *time.Time `json:"last_analysis_at" db:"last_analysis_at"`
	LastAnalysisStatus            *string    `json:"last_analysis_status" db:"last_analysis_status"`
	LatestTotalTechnicalDebtHours float64    `json:"latest_total_technical_debt_hours" db:"latest_total_technical_debt_hours"`
	LatestCriticalIssuesCount     int        `json:"latest_critical_issues_count" db:"latest_critical_issues_count"`
	LatestHighIssuesCount         int        `json:"latest_high_issues_count" db:"latest_high_issues_count"`
	LatestTestCoveragePercentage  float64    `json:"latest_test_coverage_percentage" db:"latest_test_coverage_percentage"`
	LatestDuplicationPercentage   float64    `json:"latest_duplication_percentage" db:"latest_duplication_percentage"`
	AvgDebt30d                    *float64   `json:"avg_debt_30d" db:"avg_debt_30d"`
}
