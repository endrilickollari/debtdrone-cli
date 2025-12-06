package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

type IssueActivityStoreInterface interface {
	CreateComment(comment *models.IssueComment) error
	GetCommentsByIssueID(issueID string) ([]models.IssueComment, error)
	DeleteComment(id string) error
	CreateActivity(activity *models.IssueActivityLog) error
	GetActivityByIssueID(issueID string) ([]models.IssueActivityLog, error)
	GetIssueTrends(repositoryID, issueType string, filePath *string) (*models.IssueTrends, error)
	GetRelatedIssues(issueID string, limit int) ([]models.TechnicalDebtIssue, error)
	CalculateTrends(repositoryID string) error
}

type DBIssueActivityStore struct {
	db *sql.DB
}

func NewDBIssueActivityStore(db *sql.DB) *DBIssueActivityStore {
	return &DBIssueActivityStore{db: db}
}

func (s *DBIssueActivityStore) CreateComment(comment *models.IssueComment) error {
	query := `
		INSERT INTO issue_comments (id, issue_id, user_id, text, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	if comment.ID == uuid.Nil {
		comment.ID = uuid.New()
	}
	now := time.Now()
	comment.CreatedAt = now
	comment.UpdatedAt = now

	_, err := s.db.Exec(query, comment.ID, comment.IssueID, comment.UserID, comment.Text, comment.CreatedAt, comment.UpdatedAt)
	return err
}

func (s *DBIssueActivityStore) GetCommentsByIssueID(issueID string) ([]models.IssueComment, error) {
	query := `
		SELECT c.id, c.issue_id, c.user_id, c.text, c.created_at, c.updated_at, c.deleted_at,
		       COALESCE(u.full_name, '') as user_name, COALESCE(u.email, '') as user_email
		FROM issue_comments c
		LEFT JOIN users u ON c.user_id = u.id
		WHERE c.issue_id = $1 AND c.deleted_at IS NULL
		ORDER BY c.created_at ASC
	`

	rows, err := s.db.Query(query, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []models.IssueComment
	for rows.Next() {
		var comment models.IssueComment
		err := rows.Scan(
			&comment.ID, &comment.IssueID, &comment.UserID, &comment.Text,
			&comment.CreatedAt, &comment.UpdatedAt, &comment.DeletedAt,
			&comment.UserName, &comment.UserEmail,
		)
		if err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}

	return comments, nil
}

func (s *DBIssueActivityStore) DeleteComment(id string) error {
	query := `UPDATE issue_comments SET deleted_at = $1 WHERE id = $2`
	_, err := s.db.Exec(query, time.Now(), id)
	return err
}

func (s *DBIssueActivityStore) CreateActivity(activity *models.IssueActivityLog) error {
	query := `
		INSERT INTO issue_activity_log (id, issue_id, user_id, activity_type, details, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	if activity.ID == uuid.Nil {
		activity.ID = uuid.New()
	}
	activity.CreatedAt = time.Now()

	_, err := s.db.Exec(query,
		activity.ID, activity.IssueID, activity.UserID, activity.ActivityType,
		activity.Details, activity.Metadata, activity.CreatedAt,
	)
	return err
}

func (s *DBIssueActivityStore) GetActivityByIssueID(issueID string) ([]models.IssueActivityLog, error) {
	query := `
		SELECT a.id, a.issue_id, a.user_id, a.activity_type, a.details, a.metadata, a.created_at,
		       COALESCE(u.full_name, 'System') as user_name, COALESCE(u.email, '') as user_email
		FROM issue_activity_log a
		LEFT JOIN users u ON a.user_id = u.id
		WHERE a.issue_id = $1
		ORDER BY a.created_at ASC
	`

	rows, err := s.db.Query(query, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []models.IssueActivityLog
	for rows.Next() {
		var activity models.IssueActivityLog
		err := rows.Scan(
			&activity.ID, &activity.IssueID, &activity.UserID, &activity.ActivityType,
			&activity.Details, &activity.Metadata, &activity.CreatedAt,
			&activity.UserName, &activity.UserEmail,
		)
		if err != nil {
			return nil, err
		}
		activities = append(activities, activity)
	}

	return activities, nil
}

func (s *DBIssueActivityStore) GetIssueTrends(repositoryID, issueType string, filePath *string) (*models.IssueTrends, error) {
	query := `
		SELECT id, repository_id, issue_type, file_path, total_occurrences, open_count,
		       resolved_count, resolved_last_30_days, avg_resolution_time_hours,
		       last_calculated_at, created_at, updated_at
		FROM issue_trends_cache
		WHERE repository_id = $1 AND issue_type = $2
		  AND (file_path = $3 OR (file_path IS NULL AND $3 IS NULL))
		ORDER BY last_calculated_at DESC
		LIMIT 1
	`

	var trends models.IssueTrends
	err := s.db.QueryRow(query, repositoryID, issueType, filePath).Scan(
		&trends.ID, &trends.RepositoryID, &trends.IssueType, &trends.FilePath,
		&trends.TotalOccurrences, &trends.OpenCount, &trends.ResolvedCount,
		&trends.ResolvedLast30Days, &trends.AvgResolutionTimeHours,
		&trends.LastCalculatedAt, &trends.CreatedAt, &trends.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return &models.IssueTrends{
			RepositoryID:           uuid.MustParse(repositoryID),
			IssueType:              issueType,
			FilePath:               filePath,
			TotalOccurrences:       0,
			OpenCount:              0,
			ResolvedCount:          0,
			ResolvedLast30Days:     0,
			AvgResolutionTimeHours: nil,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return &trends, nil
}

func (s *DBIssueActivityStore) GetRelatedIssues(issueID string, limit int) ([]models.TechnicalDebtIssue, error) {
	query := `
		WITH current_issue AS (
			SELECT repository_id, file_path, issue_type
			FROM technical_debt_issues
			WHERE id = $1
		)
		SELECT i.id, i.user_id, i.repository_id, i.analysis_run_id, i.file_path,
		       i.line_number, i.column_number, i.issue_type, i.severity, i.category,
		       i.message, i.description, i.tool_name, i.tool_rule_id,
		       i.confidence_score, i.technical_debt_hours, i.effort_multiplier, i.status,
		       i.resolution_reason, i.assigned_to_user_id, i.resolved_at, i.resolved_by_user_id,
		       i.ignore_until, i.comments, i.code_snippet, i.surrounding_context,
		       i.created_at, i.updated_at,
		       COALESCE(r.name, '') as repository_name,
		       COALESCE(r.full_name, '') as repository_full_name,
		       CASE WHEN i.file_path = ci.file_path THEN 1 ELSE 2 END as same_file_priority,
		       CASE i.severity
		         WHEN 'critical' THEN 1
		         WHEN 'high' THEN 2
		         WHEN 'medium' THEN 3
		         WHEN 'low' THEN 4
		         ELSE 5
		       END as severity_priority
		FROM technical_debt_issues i
		CROSS JOIN current_issue ci
		LEFT JOIN user_repositories r ON i.repository_id = r.id
		WHERE i.id != $1
		  AND i.repository_id = ci.repository_id
		  AND (
		    i.file_path = ci.file_path
		    OR i.issue_type = ci.issue_type
		  )
		ORDER BY
		  same_file_priority,
		  severity_priority,
		  i.created_at DESC
		LIMIT $2
	`

	rows, err := s.db.Query(query, issueID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []models.TechnicalDebtIssue
	for rows.Next() {
		var issue models.TechnicalDebtIssue
		var assignedTo, resolvedBy sql.NullString
		var sameFilePriority, severityPriority int
		var comments interface{}
		err := rows.Scan(
			&issue.ID, &issue.UserID, &issue.RepositoryID, &issue.AnalysisRunID, &issue.FilePath,
			&issue.LineNumber, &issue.ColumnNumber, &issue.IssueType, &issue.Severity, &issue.Category,
			&issue.Message, &issue.Description, &issue.ToolName, &issue.ToolRuleID,
			&issue.ConfidenceScore, &issue.TechnicalDebtHours, &issue.EffortMultiplier, &issue.Status,
			&issue.ResolutionReason, &assignedTo, &issue.ResolvedAt, &resolvedBy,
			&issue.IgnoreUntil, &comments, &issue.CodeSnippet, &issue.SurroundingContext,
			&issue.CreatedAt, &issue.UpdatedAt,
			&issue.RepositoryName, &issue.RepositoryFullName,
			&sameFilePriority, &severityPriority,
		)
		if err != nil {
			return nil, err
		}
		issue.AssignedToUserID = scanNullableUUID(assignedTo)
		issue.ResolvedByUserID = scanNullableUUID(resolvedBy)

		if comments != nil {
			if commentsArray, ok := comments.([]interface{}); ok {
				issue.Comments = make([]string, len(commentsArray))
				for i, c := range commentsArray {
					if str, ok := c.(string); ok {
						issue.Comments[i] = str
					}
				}
			}
		} else {
			issue.Comments = []string{}
		}

		issues = append(issues, issue)
	}

	return issues, nil
}

func (s *DBIssueActivityStore) CalculateTrends(repositoryID string) error {
	query := `SELECT calculate_issue_trends($1::UUID)`
	_, err := s.db.Exec(query, repositoryID)
	return err
}

func (s *DBIssueActivityStore) LogIssueCreated(issueID uuid.UUID) error {
	activity := &models.IssueActivityLog{
		IssueID:      issueID,
		UserID:       nil,
		ActivityType: "created",
		Details:      "Issue detected during analysis",
		Metadata:     nil,
	}
	return s.CreateActivity(activity)
}

func (s *DBIssueActivityStore) LogStatusChange(issueID uuid.UUID, userID *uuid.UUID, oldStatus, newStatus string, reason *string) error {
	metadata := fmt.Sprintf(`{"old_status": "%s", "new_status": "%s"`, oldStatus, newStatus)
	if reason != nil && *reason != "" {
		metadata += fmt.Sprintf(`, "resolution_reason": "%s"`, *reason)
	}
	metadata += "}"

	activity := &models.IssueActivityLog{
		IssueID:      issueID,
		UserID:       userID,
		ActivityType: "status_changed",
		Details:      fmt.Sprintf("Status changed from %s to %s", oldStatus, newStatus),
		Metadata:     &metadata,
	}
	return s.CreateActivity(activity)
}

func (s *DBIssueActivityStore) LogAssignment(issueID uuid.UUID, assignedByUserID, assignedToUserID uuid.UUID) error {
	metadata := fmt.Sprintf(`{"assigned_to_user_id": "%s"}`, assignedToUserID.String())

	activity := &models.IssueActivityLog{
		IssueID:      issueID,
		UserID:       &assignedByUserID,
		ActivityType: "assigned",
		Details:      "Issue assigned to team member",
		Metadata:     &metadata,
	}
	return s.CreateActivity(activity)
}

func (s *DBIssueActivityStore) LogUnassignment(issueID uuid.UUID, userID *uuid.UUID, oldUserID uuid.UUID) error {
	metadata := fmt.Sprintf(`{"old_user_id": "%s"}`, oldUserID.String())

	activity := &models.IssueActivityLog{
		IssueID:      issueID,
		UserID:       userID,
		ActivityType: "unassigned",
		Details:      "Issue unassigned",
		Metadata:     &metadata,
	}
	return s.CreateActivity(activity)
}
