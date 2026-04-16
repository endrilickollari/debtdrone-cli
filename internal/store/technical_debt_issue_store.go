package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func scanNullableUUID(ns sql.NullString) *uuid.UUID {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	id, err := uuid.Parse(ns.String)
	if err != nil {
		return nil
	}
	return &id
}

type IssueFilters struct {
	Severity     *string
	Status       *string
	IssueType    *string
	RepositoryID *string
	UserID       *string
}

// OpenIssueSummary holds the aggregated counts of open issues by severity
type OpenIssueSummary struct {
	CriticalCount  int     `json:"critical_count"`
	HighCount      int     `json:"high_count"`
	MediumCount    int     `json:"medium_count"`
	LowCount       int     `json:"low_count"`
	TotalDebtHours float64 `json:"total_debt_hours"`
}

type TechnicalDebtIssueStoreInterface interface {
	Create(issue *models.TechnicalDebtIssue) error
	BatchCreate(issues []models.TechnicalDebtIssue) error
	Get(id string) (*models.TechnicalDebtIssue, error)
	List(limit, offset int) ([]models.TechnicalDebtIssue, error)
	ListWithFilters(filters IssueFilters, limit, offset int) ([]models.TechnicalDebtIssue, int, error)
	Update(issue *models.TechnicalDebtIssue) error
	IssueExists(repositoryID uuid.UUID, filePath string, lineNumber *int, issueType string, toolRuleID *string) (bool, error)
	TouchExistingIssue(repositoryID uuid.UUID, analysisRunID uuid.UUID, filePath string, lineNumber *int, issueType string, toolRuleID *string) error
	ResolveMissingIssues(repositoryID uuid.UUID, currentAnalysisRunID uuid.UUID, resolutionReason string) ([]uuid.UUID, error)
	GetOpenIssueSummary(repositoryID uuid.UUID) (*OpenIssueSummary, error)
	// ReconcileIssuesForAnalyzer atomically replaces all issues for a specific analyzer in a repository.
	// This implements the "Clear-and-Replace" strategy to ensure idempotent scans.
	ReconcileIssuesForAnalyzer(repositoryID uuid.UUID, analyzerName string, newIssues []models.TechnicalDebtIssue) (int, int, error)
	// UpdateExternalLink persists the external ticket link after successful creation in Jira/Trello.
	// This enables the "Link & Skip" deduplication logic on subsequent scans.
	UpdateExternalLink(issueID uuid.UUID, externalID, platform, externalURL string) error
	// GetByExternalLink retrieves an issue by its external platform link (for sync operations).
	GetByExternalLink(platform, externalID string) (*models.TechnicalDebtIssue, error)
	// GetPendingSyncs retrieves issues with a 'pending' sync status for a specific platform.
	GetPendingSyncs(repoID uuid.UUID, platform string, severities []string) ([]models.TechnicalDebtIssue, error)
	// ResolveStaleIssuesInFiles resolves issues in specific files that are not in the current found list (for delta scans)
	ResolveStaleIssuesInFiles(ctx context.Context, repoID uuid.UUID, filePaths []string, foundIssueIDs []uuid.UUID) error

	// GetTopNewIssuesForRun retrieves the most severe issues introduced in a specific analysis run, optionally filtered by target files.
	GetTopNewIssuesForRun(runID uuid.UUID, limit int, targetFiles []string) ([]models.TechnicalDebtIssue, error)
}

type DBTechnicalDebtIssueStore struct {
	db *sql.DB
}

func NewDBTechnicalDebtIssueStore(db *sql.DB) *DBTechnicalDebtIssueStore {
	return &DBTechnicalDebtIssueStore{db: db}
}

func (s *DBTechnicalDebtIssueStore) IssueExists(repositoryID uuid.UUID, filePath string, lineNumber *int, issueType string, toolRuleID *string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM technical_debt_issues
			WHERE repository_id = $1
			  AND file_path = $2
			  AND (line_number = $3 OR (line_number IS NULL AND $3 IS NULL))
			  AND issue_type = $4
			  AND (tool_rule_id = $5 OR (tool_rule_id IS NULL AND $5 IS NULL))
			  AND status IN ('open', 'ignored')
		)
	`

	var exists bool
	err := s.db.QueryRow(query, repositoryID, filePath, lineNumber, issueType, toolRuleID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if issue exists: %w", err)
	}

	return exists, nil
}

func (s *DBTechnicalDebtIssueStore) Create(issue *models.TechnicalDebtIssue) error {
	exists, err := s.IssueExists(issue.RepositoryID, issue.FilePath, issue.LineNumber, issue.IssueType, issue.ToolRuleID)
	if err != nil {
		return fmt.Errorf("failed to check for duplicate issue: %w", err)
	}
	if exists {
		return nil
	}

	query := `
		INSERT INTO technical_debt_issues (
			id, user_id, repository_id, analysis_run_id, file_path, line_number, column_number,
			issue_type, severity, category, message, description, tool_name, tool_rule_id,
			confidence_score, technical_debt_hours, effort_multiplier, status, code_snippet,
			fingerprint_hash, jira_sync_status, trello_sync_status,
			external_id, external_platform, external_url,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27)
	`

	if issue.ID == uuid.Nil {
		issue.ID = uuid.New()
	}
	now := time.Now()
	issue.CreatedAt = now
	issue.UpdatedAt = now

	_, execErr := s.db.Exec(query,
		issue.ID, issue.UserID, issue.RepositoryID, issue.AnalysisRunID, issue.FilePath,
		issue.LineNumber, issue.ColumnNumber, issue.IssueType, issue.Severity, issue.Category,
		issue.Message, issue.Description, issue.ToolName, issue.ToolRuleID, issue.ConfidenceScore,
		issue.TechnicalDebtHours, issue.EffortMultiplier, issue.Status, issue.CodeSnippet,
		issue.FingerprintHash, issue.JiraSyncStatus, issue.TrelloSyncStatus,
		issue.ExternalID, issue.ExternalPlatform, issue.ExternalURL,
		issue.CreatedAt, issue.UpdatedAt,
	)
	return execErr
}

func (s *DBTechnicalDebtIssueStore) BatchCreate(issues []models.TechnicalDebtIssue) error {
	if len(issues) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO technical_debt_issues (
			id, user_id, repository_id, analysis_run_id, file_path, line_number, column_number,
			issue_type, severity, category, message, description, tool_name, tool_rule_id,
			confidence_score, technical_debt_hours, effort_multiplier, status, code_snippet,
			fingerprint_hash, jira_sync_status, trello_sync_status,
			external_id, external_platform, external_url,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()

	for i := range issues {
		issue := &issues[i]

		exists, err := s.IssueExists(issue.RepositoryID, issue.FilePath, issue.LineNumber, issue.IssueType, issue.ToolRuleID)
		if err != nil {
			return fmt.Errorf("failed to check for duplicate issue %d: %w", i, err)
		}
		if exists {
			// Touch the existing issue to update its analysis_run_id
			// This prevents it from being marked as 'resolved' by ResolveMissingIssues
			if err := s.TouchExistingIssue(issue.RepositoryID, issue.AnalysisRunID, issue.FilePath, issue.LineNumber, issue.IssueType, issue.ToolRuleID); err != nil {
				return fmt.Errorf("failed to touch existing issue %d: %w", i, err)
			}
			continue
		}

		if issue.ID == uuid.Nil {
			issue.ID = uuid.New()
		}
		issue.CreatedAt = now
		issue.UpdatedAt = now

		var execErr error
		_, execErr = stmt.Exec(
			issue.ID, issue.UserID, issue.RepositoryID, issue.AnalysisRunID, issue.FilePath,
			issue.LineNumber, issue.ColumnNumber, issue.IssueType, issue.Severity, issue.Category,
			issue.Message, issue.Description, issue.ToolName, issue.ToolRuleID, issue.ConfidenceScore,
			issue.TechnicalDebtHours, issue.EffortMultiplier, issue.Status, issue.CodeSnippet,
			issue.FingerprintHash, issue.JiraSyncStatus, issue.TrelloSyncStatus,
			issue.ExternalID, issue.ExternalPlatform, issue.ExternalURL,
			issue.CreatedAt, issue.UpdatedAt,
		)
		if execErr != nil {
			return fmt.Errorf("failed to insert issue %d: %w", i, execErr)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (s *DBTechnicalDebtIssueStore) Get(id string) (*models.TechnicalDebtIssue, error) {
	query := `
		SELECT i.id, i.user_id, i.repository_id, i.analysis_run_id, i.file_path, i.line_number, i.column_number,
		       i.issue_type, i.severity, i.category, i.message, i.description, i.tool_name, i.tool_rule_id,
		       i.confidence_score, i.technical_debt_hours, i.effort_multiplier, i.status,
		       i.resolution_reason, i.assigned_to_user_id, i.resolved_at, i.resolved_by_user_id,
		       i.ignore_until, i.comments, i.code_snippet, i.surrounding_context,
		       i.fingerprint_hash, i.jira_sync_status, i.trello_sync_status,
		       i.external_id, i.external_platform, i.external_url,
		       i.created_at, i.updated_at,
		       COALESCE(r.name, '') as repository_name,
		       COALESCE(r.full_name, '') as repository_full_name
		FROM technical_debt_issues i
		LEFT JOIN user_repositories r ON i.repository_id = r.id
		WHERE i.id = $1
	`

	var issue models.TechnicalDebtIssue
	var assignedTo, resolvedBy sql.NullString
	var externalIDNull, externalPlatformNull, externalURLNull, fingerprintHashNull sql.NullString
	err := s.db.QueryRow(query, id).Scan(
		&issue.ID, &issue.UserID, &issue.RepositoryID, &issue.AnalysisRunID, &issue.FilePath,
		&issue.LineNumber, &issue.ColumnNumber, &issue.IssueType, &issue.Severity, &issue.Category,
		&issue.Message, &issue.Description, &issue.ToolName, &issue.ToolRuleID,
		&issue.ConfidenceScore, &issue.TechnicalDebtHours, &issue.EffortMultiplier, &issue.Status,
		&issue.ResolutionReason, &assignedTo, &issue.ResolvedAt, &resolvedBy,
		&issue.IgnoreUntil, pq.Array(&issue.Comments), &issue.CodeSnippet, &issue.SurroundingContext,
		&fingerprintHashNull, &issue.JiraSyncStatus, &issue.TrelloSyncStatus,
		&externalIDNull, &externalPlatformNull, &externalURLNull,
		&issue.CreatedAt, &issue.UpdatedAt,
		&issue.RepositoryName, &issue.RepositoryFullName,
	)
	if err != nil {
		return nil, err
	}
	issue.AssignedToUserID = scanNullableUUID(assignedTo)
	issue.ResolvedByUserID = scanNullableUUID(resolvedBy)
	if externalIDNull.Valid {
		issue.ExternalID = &externalIDNull.String
	}
	if externalPlatformNull.Valid {
		issue.ExternalPlatform = &externalPlatformNull.String
	}
	if externalURLNull.Valid {
		issue.ExternalURL = &externalURLNull.String
	}
	if fingerprintHashNull.Valid {
		issue.FingerprintHash = fingerprintHashNull.String
	}
	return &issue, nil
}

func (s *DBTechnicalDebtIssueStore) List(limit, offset int) ([]models.TechnicalDebtIssue, error) {
	query := `
		SELECT id, user_id, repository_id, analysis_run_id, file_path, line_number, column_number,
		       issue_type, severity, category, message, description, tool_name, tool_rule_id,
		       confidence_score, technical_debt_hours, effort_multiplier, status,
		       resolution_reason, assigned_to_user_id, resolved_at, resolved_by_user_id,
		       ignore_until, comments, code_snippet, surrounding_context,
		       fingerprint_hash, jira_sync_status, trello_sync_status,
		       external_id, external_platform, external_url,
		       created_at, updated_at
		FROM technical_debt_issues
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []models.TechnicalDebtIssue
	for rows.Next() {
		var issue models.TechnicalDebtIssue
		var assignedTo, resolvedBy sql.NullString
		var externalIDNull, externalPlatformNull, externalURLNull, fingerprintHashNull sql.NullString
		err := rows.Scan(
			&issue.ID, &issue.UserID, &issue.RepositoryID, &issue.AnalysisRunID, &issue.FilePath,
			&issue.LineNumber, &issue.ColumnNumber, &issue.IssueType, &issue.Severity, &issue.Category,
			&issue.Message, &issue.Description, &issue.ToolName, &issue.ToolRuleID,
			&issue.ConfidenceScore, &issue.TechnicalDebtHours, &issue.EffortMultiplier, &issue.Status,
			&issue.ResolutionReason, &assignedTo, &issue.ResolvedAt, &resolvedBy,
			&issue.IgnoreUntil, pq.Array(&issue.Comments), &issue.CodeSnippet, &issue.SurroundingContext,
			&fingerprintHashNull, &issue.JiraSyncStatus, &issue.TrelloSyncStatus,
			&externalIDNull, &externalPlatformNull, &externalURLNull,
			&issue.CreatedAt, &issue.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		issue.AssignedToUserID = scanNullableUUID(assignedTo)
		issue.ResolvedByUserID = scanNullableUUID(resolvedBy)
		if externalIDNull.Valid {
			issue.ExternalID = &externalIDNull.String
		}
		if externalPlatformNull.Valid {
			issue.ExternalPlatform = &externalPlatformNull.String
		}
		if externalURLNull.Valid {
			issue.ExternalURL = &externalURLNull.String
		}
		if fingerprintHashNull.Valid {
			issue.FingerprintHash = fingerprintHashNull.String
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

func (s *DBTechnicalDebtIssueStore) ListWithFilters(filters IssueFilters, limit, offset int) ([]models.TechnicalDebtIssue, int, error) {
	whereClauses := []string{}
	args := []interface{}{}
	argCount := 1

	if filters.UserID != nil && *filters.UserID != "" {
		userUUID, err := uuid.Parse(*filters.UserID)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid user ID: %w", err)
		}
		whereClauses = append(whereClauses, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM user_repositories ur
				JOIN organization_members om ON ur.organization_id = om.organization_id
				WHERE ur.id = i.repository_id AND om.user_id = $%d
			)
		`, argCount))
		args = append(args, userUUID)
		argCount++
	}

	if filters.Severity != nil && *filters.Severity != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("i.severity = $%d", argCount))
		args = append(args, *filters.Severity)
		argCount++
	}

	if filters.Status != nil && *filters.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("i.status = $%d", argCount))
		args = append(args, *filters.Status)
		argCount++
	}

	if filters.IssueType != nil && *filters.IssueType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("i.issue_type = $%d", argCount))
		args = append(args, *filters.IssueType)
		argCount++
	}

	if filters.RepositoryID != nil && *filters.RepositoryID != "" {
		repoUUID, err := uuid.Parse(*filters.RepositoryID)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid repository ID: %w", err)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("i.repository_id = $%d", argCount))
		args = append(args, repoUUID)
		argCount++
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + whereClauses[0]
		for _, clause := range whereClauses[1:] {
			whereClause += " AND " + clause
		}
	}

	countQuery := "SELECT COUNT(*) FROM technical_debt_issues i " + whereClause
	var total int
	err := s.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`SELECT
			i.id, i.user_id, i.repository_id, i.analysis_run_id, i.file_path, i.line_number, i.column_number,
			i.issue_type, i.severity, i.category, i.message, i.description, i.tool_name, i.tool_rule_id,
			i.confidence_score, i.technical_debt_hours, i.effort_multiplier, i.status,
			i.resolution_reason, i.assigned_to_user_id, i.resolved_at, i.resolved_by_user_id,
			i.ignore_until, i.comments, i.code_snippet, i.surrounding_context,
			i.fingerprint_hash, i.jira_sync_status, i.trello_sync_status,
			i.external_id, i.external_platform, i.external_url,
			i.created_at, i.updated_at,
			COALESCE(r.name, '') as repository_name,
			COALESCE(r.full_name, '') as repository_full_name
		FROM technical_debt_issues i
		LEFT JOIN user_repositories r ON i.repository_id = r.id
		%s
		ORDER BY
			CASE i.severity
				WHEN 'critical' THEN 1
				WHEN 'high' THEN 2
				WHEN 'medium' THEN 3
				WHEN 'low' THEN 4
				ELSE 5
			END,
			i.created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argCount, argCount+1)

	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var issues []models.TechnicalDebtIssue
	for rows.Next() {
		var issue models.TechnicalDebtIssue
		var assignedTo, resolvedBy sql.NullString
		var externalIDNull, externalPlatformNull, externalURLNull, fingerprintHashNull sql.NullString
		err := rows.Scan(
			&issue.ID, &issue.UserID, &issue.RepositoryID, &issue.AnalysisRunID, &issue.FilePath,
			&issue.LineNumber, &issue.ColumnNumber, &issue.IssueType, &issue.Severity, &issue.Category,
			&issue.Message, &issue.Description, &issue.ToolName, &issue.ToolRuleID,
			&issue.ConfidenceScore, &issue.TechnicalDebtHours, &issue.EffortMultiplier, &issue.Status,
			&issue.ResolutionReason, &assignedTo, &issue.ResolvedAt, &resolvedBy,
			&issue.IgnoreUntil, pq.Array(&issue.Comments), &issue.CodeSnippet, &issue.SurroundingContext,
			&fingerprintHashNull, &issue.JiraSyncStatus, &issue.TrelloSyncStatus,
			&externalIDNull, &externalPlatformNull, &externalURLNull,
			&issue.CreatedAt, &issue.UpdatedAt,
			&issue.RepositoryName, &issue.RepositoryFullName,
		)
		if err != nil {
			return nil, 0, err
		}
		issue.AssignedToUserID = scanNullableUUID(assignedTo)
		issue.ResolvedByUserID = scanNullableUUID(resolvedBy)
		if externalIDNull.Valid {
			issue.ExternalID = &externalIDNull.String
		}
		if externalPlatformNull.Valid {
			issue.ExternalPlatform = &externalPlatformNull.String
		}
		if externalURLNull.Valid {
			issue.ExternalURL = &externalURLNull.String
		}
		if fingerprintHashNull.Valid {
			issue.FingerprintHash = fingerprintHashNull.String
		}
		issues = append(issues, issue)
	}

	return issues, total, nil
}

// TouchExistingIssue updates the analysis_run_id of an existing issue to mark it as still present in the codebase.
// This prevents the issue from being auto-resolved by ResolveMissingIssues.
func (s *DBTechnicalDebtIssueStore) TouchExistingIssue(repositoryID uuid.UUID, analysisRunID uuid.UUID, filePath string, lineNumber *int, issueType string, toolRuleID *string) error {
	query := `
		UPDATE technical_debt_issues
		SET analysis_run_id = $1, updated_at = NOW()
		WHERE repository_id = $2
		  AND file_path = $3
		  AND (line_number = $4 OR (line_number IS NULL AND $4 IS NULL))
		  AND issue_type = $5
		  AND (tool_rule_id = $6 OR (tool_rule_id IS NULL AND $6 IS NULL))
		  AND status IN ('open', 'ignored')
	`

	_, err := s.db.Exec(query, analysisRunID, repositoryID, filePath, lineNumber, issueType, toolRuleID)
	if err != nil {
		return fmt.Errorf("failed to touch existing issue: %w", err)
	}

	return nil
}

func (s *DBTechnicalDebtIssueStore) Update(issue *models.TechnicalDebtIssue) error {
	query := `
		UPDATE technical_debt_issues
		SET status = $1, resolution_reason = $2, assigned_to_user_id = $3,
		    resolved_at = $4, resolved_by_user_id = $5, ignore_until = $6,
		    updated_at = $7
		WHERE id = $8
	`

	issue.UpdatedAt = time.Now()

	_, err := s.db.Exec(query,
		issue.Status, issue.ResolutionReason, issue.AssignedToUserID,
		issue.ResolvedAt, issue.ResolvedByUserID, issue.IgnoreUntil,
		issue.UpdatedAt, issue.ID,
	)
	return err
}

// GetTopNewIssuesForRun retrieves the most severe issues introduced in a specific analysis run.
func (s *DBTechnicalDebtIssueStore) GetTopNewIssuesForRun(runID uuid.UUID, limit int, targetFiles []string) ([]models.TechnicalDebtIssue, error) {
	query := `
		SELECT
			t.id, t.file_path, t.line_number, t.severity, t.description, t.message
		FROM technical_debt_issues t
		JOIN analysis_runs r ON r.id = t.analysis_run_id
		WHERE t.analysis_run_id = $1 AND t.status = 'open'
		AND t.created_at >= r.started_at
	`

	args := []interface{}{runID}
	argCount := 2

	if len(targetFiles) > 0 {
		query += fmt.Sprintf(" AND t.file_path = ANY($%d::text[])", argCount)
		args = append(args, pq.Array(targetFiles))
		argCount++
	}

	query += fmt.Sprintf(`
		ORDER BY
			CASE severity
				WHEN 'critical' THEN 1
				WHEN 'high' THEN 2
				WHEN 'medium' THEN 3
				WHEN 'low' THEN 4
				ELSE 5
			END,
			created_at DESC
		LIMIT $%d
	`, argCount)

	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get top new issues for run: %w", err)
	}
	defer rows.Close()

	var issues []models.TechnicalDebtIssue
	for rows.Next() {
		var issue models.TechnicalDebtIssue
		err := rows.Scan(
			&issue.ID, &issue.FilePath, &issue.LineNumber, &issue.Severity, &issue.Description, &issue.Message,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan top new issue: %w", err)
		}
		issues = append(issues, issue)
	}

	return issues, nil
}

// ResolveMissingIssues marks all open issues that were not detected in the latest scan as 'resolved'.
// This implements the "Sync-to-Truth" pattern for issue reconciliation.
// ResolveMissingIssues marks all open issues that were not detected in the latest scan as 'resolved'.
// This implements the "Sync-to-Truth" pattern for issue reconciliation.
func (s *DBTechnicalDebtIssueStore) ResolveMissingIssues(repositoryID uuid.UUID, currentAnalysisRunID uuid.UUID, resolutionReason string) ([]uuid.UUID, error) {
	query := `
		UPDATE technical_debt_issues
		SET
			status = 'resolved',
			resolved_at = NOW(),
			updated_at = NOW(),
			resolution_reason = $3
		WHERE repository_id = $1
		  AND status = 'open'
		  AND analysis_run_id != $2
		RETURNING id
	`

	rows, err := s.db.Query(query, repositoryID, currentAnalysisRunID, resolutionReason)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve missing issues: %w", err)
	}
	defer rows.Close()

	var resolvedIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan return id: %w", err)
		}
		resolvedIDs = append(resolvedIDs, id)
	}

	return resolvedIDs, nil
}

// ResolveStaleIssuesInFiles resolves issues in specific files that are not in the current found list (for delta scans)
func (s *DBTechnicalDebtIssueStore) ResolveStaleIssuesInFiles(ctx context.Context, repoID uuid.UUID, filePaths []string, foundIssueIDs []uuid.UUID) error {
	query := `
		UPDATE technical_debt_issues 
		SET status = 'resolved', 
			resolved_at = now(),
			updated_at = now(),
			resolution_reason = 'Automatically resolved during incremental scan'
		WHERE repository_id = $1 
		AND status = 'open' 
		AND file_path = ANY($2::text[])
		AND NOT (id = ANY($3::uuid[]));
	`

	// Convert filePaths to pq.StringArray for Postgres array compatibility
	// Convert foundIssueIDs to pq.Array (or manual string array if needed, but uuid array should work with lib/pq)

	_, err := s.db.ExecContext(ctx, query, repoID, pq.Array(filePaths), pq.Array(foundIssueIDs))
	if err != nil {
		return fmt.Errorf("failed to resolve stale issues in files: %w", err)
	}

	return nil
}

// GetOpenIssueSummary returns the aggregated counts of open issues by severity for a repository.
// This is used to calculate accurate metrics after issue reconciliation.
func (s *DBTechnicalDebtIssueStore) GetOpenIssueSummary(repositoryID uuid.UUID) (*OpenIssueSummary, error) {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE severity = 'critical') AS critical_count,
			COUNT(*) FILTER (WHERE severity = 'high') AS high_count,
			COUNT(*) FILTER (WHERE severity = 'medium') AS medium_count,
			COUNT(*) FILTER (WHERE severity = 'low') AS low_count,
			COALESCE(SUM(technical_debt_hours), 0) AS total_debt_hours
		FROM technical_debt_issues
		WHERE repository_id = $1 AND status = 'open'
	`

	var summary OpenIssueSummary
	err := s.db.QueryRow(query, repositoryID).Scan(
		&summary.CriticalCount,
		&summary.HighCount,
		&summary.MediumCount,
		&summary.LowCount,
		&summary.TotalDebtHours,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get open issue summary: %w", err)
	}

	return &summary, nil
}

// ReconcileIssuesForAnalyzer performs an atomic "Sync-to-Truth" upsert.
// It ensures that stable issues (matching fingerprint) are updated, not recreated.
// Issues present in the DB but missing from the current analysis are marked as 'resolved'.
func (s *DBTechnicalDebtIssueStore) ReconcileIssuesForAnalyzer(repositoryID uuid.UUID, analyzerName string, newIssues []models.TechnicalDebtIssue) (int, int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Step 1: Upsert new issues
	// We use the fingerprint_hash to match existing open issues.
	// If a match is found, we update the analysis_run_id and timestamp, effectively "touching" it.
	// If no match, we insert as new.
	query := `
		INSERT INTO technical_debt_issues (
			id, user_id, repository_id, analysis_run_id, file_path, line_number, column_number,
			issue_type, severity, category, message, description, tool_name, tool_rule_id,
			confidence_score, technical_debt_hours, effort_multiplier, status, code_snippet, surrounding_context,
			fingerprint_hash, jira_sync_status, trello_sync_status,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, 'open', $18, $19,
			$20, $21, $22,
			$23, $24
		)
		ON CONFLICT (repository_id, fingerprint_hash) WHERE status = 'open'
		DO UPDATE SET
			analysis_run_id = EXCLUDED.analysis_run_id,
			updated_at = EXCLUDED.updated_at,
			-- Update volatile fields that might change (e.g. line numbers due to shifts)
			line_number = EXCLUDED.line_number,
			column_number = EXCLUDED.column_number,
			code_snippet = EXCLUDED.code_snippet,
			surrounding_context = EXCLUDED.surrounding_context
	`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to prepare upsert statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	insertedCount := 0

	for i := range newIssues {
		issue := &newIssues[i]
		if issue.ID == uuid.Nil {
			issue.ID = uuid.New()
		}

		// Ensure sync status defaults are set if empty (though model/DB should handle this)
		if issue.JiraSyncStatus == "" {
			issue.JiraSyncStatus = "pending"
		}
		if issue.TrelloSyncStatus == "" {
			issue.TrelloSyncStatus = "pending"
		}

		_, err := stmt.Exec(
			issue.ID, issue.UserID, issue.RepositoryID, issue.AnalysisRunID, issue.FilePath, issue.LineNumber, issue.ColumnNumber,
			issue.IssueType, issue.Severity, issue.Category, issue.Message, issue.Description, issue.ToolName, issue.ToolRuleID,
			issue.ConfidenceScore, issue.TechnicalDebtHours, issue.EffortMultiplier, issue.CodeSnippet, issue.SurroundingContext,
			issue.FingerprintHash, issue.JiraSyncStatus, issue.TrelloSyncStatus,
			now, now,
		)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to upsert issue %s (fingerprint: %s): %w", issue.FilePath, issue.FingerprintHash, err)
		}
		insertedCount++
	}

	// Step 2: "Sweep" - Resolve missing issues
	// Any open issue for this analyzer & repo that wasn't updated in this run (analysis_run_id != current) is now resolved.
	// NOTE: We assume all issues for this analyzer were processed in the batch above.
	// If newIssues is empty, ALL open issues for this analyzer are resolved.

	// Get the Current Analysis Run ID from the first issue, or fail if empty?
	// If newIssues is empty, we need the runID passed separately or derived.
	// But wait, if newIssues is empty, we just mark all open issues for this analyzer as resolved.
	// We need the AnalysisRunID to log properly, but technically any issue NOT touched is resolved.

	var currentRunID uuid.UUID
	if len(newIssues) > 0 {
		currentRunID = newIssues[0].AnalysisRunID
	} else if len(newIssues) == 0 {
		// Edge case: Analyzer found 0 issues. Everything currently open for this analyzer should be resolved.
		// logic below handles it because NO issues have the new runID.
	}

	resolveQuery := `
		UPDATE technical_debt_issues
		SET status = 'resolved',
			resolution_reason = 'fixed_in_code',
			resolved_at = $1,
			updated_at = $1
		WHERE repository_id = $2
		  AND tool_name = $3
		  AND status = 'open'
		  AND (analysis_run_id != $4 OR analysis_run_id IS NULL)
	`
	// If currentRunID is nil (empty batch), the Condition `analysis_run_id != $4` will be true for any existing ID?
	// Make sure uuid.Nil works in postgres comparison. UUID(00..) != existing-uuid is True.

	res, err := tx.Exec(resolveQuery, now, repositoryID, analyzerName, currentRunID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to mark old issues as resolved: %w", err)
	}

	resolvedCount, _ := res.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// We return resolvedCount as "deducted" (conceptually) or separately?
	// The signature is (deleted, inserted, error). We'll map resolved->deleted for backward compat in logging.
	return int(resolvedCount), insertedCount, nil
}

// UpdateExternalLink persists the external ticket link after successful creation in Jira/Trello.
// This enables the "Link & Skip" deduplication logic on subsequent scans.
func (s *DBTechnicalDebtIssueStore) UpdateExternalLink(issueID uuid.UUID, externalID, platform, externalURL string) error {
	query := `
		UPDATE technical_debt_issues
		SET external_id = $2,
		    external_platform = $3,
		    external_url = $4,
		    updated_at = NOW()
		WHERE id = $1
	`

	result, err := s.db.Exec(query, issueID, externalID, platform, externalURL)
	if err != nil {
		return fmt.Errorf("failed to update external link for issue %s: %w", issueID, err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("issue %s not found", issueID)
	}

	return nil
}

// GetByExternalLink retrieves an issue by its external platform link (for sync operations).
func (s *DBTechnicalDebtIssueStore) GetByExternalLink(platform, externalID string) (*models.TechnicalDebtIssue, error) {
	query := `
		SELECT i.id, i.user_id, i.repository_id, i.analysis_run_id, i.file_path, i.line_number, i.column_number,
		       i.issue_type, i.severity, i.category, i.message, i.description, i.tool_name, i.tool_rule_id,
		       i.confidence_score, i.technical_debt_hours, i.effort_multiplier, i.status,
		       i.resolution_reason, i.assigned_to_user_id, i.resolved_at, i.resolved_by_user_id,
		       i.ignore_until, i.comments, i.code_snippet, i.surrounding_context,
		       i.external_id, i.external_platform, i.external_url,
		       i.created_at, i.updated_at
		FROM technical_debt_issues i
		WHERE i.external_platform = $1 AND i.external_id = $2
	`

	var issue models.TechnicalDebtIssue
	var comments pq.StringArray
	var resolutionReason, description, toolRuleID, codeSnippet, surroundingContext sql.NullString
	var assignedToUserID, resolvedByUserID sql.NullString
	var resolvedAt, ignoreUntil sql.NullTime
	var lineNumber, columnNumber sql.NullInt64
	var externalIDNull, externalPlatformNull, externalURLNull sql.NullString

	err := s.db.QueryRow(query, platform, externalID).Scan(
		&issue.ID, &issue.UserID, &issue.RepositoryID, &issue.AnalysisRunID,
		&issue.FilePath, &lineNumber, &columnNumber,
		&issue.IssueType, &issue.Severity, &issue.Category, &issue.Message, &description,
		&issue.ToolName, &toolRuleID, &issue.ConfidenceScore, &issue.TechnicalDebtHours,
		&issue.EffortMultiplier, &issue.Status, &resolutionReason, &assignedToUserID,
		&resolvedAt, &resolvedByUserID, &ignoreUntil, &comments, &codeSnippet, &surroundingContext,
		&externalIDNull, &externalPlatformNull, &externalURLNull,
		&issue.CreatedAt, &issue.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get issue by external link: %w", err)
	}

	// Handle nullable fields
	if lineNumber.Valid {
		ln := int(lineNumber.Int64)
		issue.LineNumber = &ln
	}
	if columnNumber.Valid {
		cn := int(columnNumber.Int64)
		issue.ColumnNumber = &cn
	}
	if description.Valid {
		issue.Description = &description.String
	}
	if toolRuleID.Valid {
		issue.ToolRuleID = &toolRuleID.String
	}
	if codeSnippet.Valid {
		issue.CodeSnippet = &codeSnippet.String
	}
	if surroundingContext.Valid {
		issue.SurroundingContext = &surroundingContext.String
	}
	if resolutionReason.Valid {
		issue.ResolutionReason = &resolutionReason.String
	}
	if assignedToUserID.Valid {
		id := uuid.MustParse(assignedToUserID.String)
		issue.AssignedToUserID = &id
	}
	if resolvedByUserID.Valid {
		id := uuid.MustParse(resolvedByUserID.String)
		issue.ResolvedByUserID = &id
	}
	if resolvedAt.Valid {
		issue.ResolvedAt = &resolvedAt.Time
	}
	if ignoreUntil.Valid {
		issue.IgnoreUntil = &ignoreUntil.Time
	}
	if externalIDNull.Valid {
		issue.ExternalID = &externalIDNull.String
	}
	if externalPlatformNull.Valid {
		issue.ExternalPlatform = &externalPlatformNull.String
	}
	if externalURLNull.Valid {
		issue.ExternalURL = &externalURLNull.String
	}
	issue.Comments = comments

	return &issue, nil
}

// GetPendingSyncs retrieves issues with a 'pending' sync status for a specific platform.
func (s *DBTechnicalDebtIssueStore) GetPendingSyncs(repoID uuid.UUID, platform string, severities []string) ([]models.TechnicalDebtIssue, error) {
	if len(severities) == 0 {
		return []models.TechnicalDebtIssue{}, nil
	}

	statusColumn := "jira_sync_status"
	if platform == "trello" {
		statusColumn = "trello_sync_status"
	} else if platform != "jira" {
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}

	query := fmt.Sprintf(`
SELECT
i.id, i.user_id, i.repository_id, i.analysis_run_id, i.file_path, i.line_number, i.column_number,
i.issue_type, i.severity, i.category, i.message, i.description, i.tool_name, i.tool_rule_id,
i.confidence_score, i.technical_debt_hours, i.effort_multiplier, i.status,
i.resolution_reason, i.assigned_to_user_id, i.resolved_at, i.resolved_by_user_id,
i.ignore_until, i.comments, i.code_snippet, i.surrounding_context,
i.external_id, i.external_platform, i.external_url,
i.fingerprint_hash, i.jira_sync_status, i.trello_sync_status,
i.created_at, i.updated_at,
COALESCE(r.name, '') as repository_name,
COALESCE(r.full_name, '') as repository_full_name
FROM technical_debt_issues i
LEFT JOIN user_repositories r ON i.repository_id = r.id
WHERE i.repository_id = $1
  AND i.status = 'open'
  AND i.%s = 'pending'
  AND i.severity = ANY($2)
`, statusColumn)

	rows, err := s.db.Query(query, repoID, pq.Array(severities))
	if err != nil {
		return nil, fmt.Errorf("failed to get pending syncs: %w", err)
	}
	defer rows.Close()

	var issues []models.TechnicalDebtIssue
	for rows.Next() {
		var issue models.TechnicalDebtIssue
		var comments pq.StringArray
		var resolutionReason, description, toolRuleID, codeSnippet, surroundingContext sql.NullString
		var assignedToUserID, resolvedByUserID sql.NullString
		var resolvedAt, ignoreUntil sql.NullTime
		var lineNumber, columnNumber sql.NullInt64
		var externalIDNull, externalPlatformNull, externalURLNull sql.NullString
		var fingerprintHash, jiraSync, trelloSync sql.NullString

		err := rows.Scan(
			&issue.ID, &issue.UserID, &issue.RepositoryID, &issue.AnalysisRunID,
			&issue.FilePath, &lineNumber, &columnNumber,
			&issue.IssueType, &issue.Severity, &issue.Category, &issue.Message, &description,
			&issue.ToolName, &toolRuleID, &issue.ConfidenceScore, &issue.TechnicalDebtHours,
			&issue.EffortMultiplier, &issue.Status, &resolutionReason, &assignedToUserID,
			&resolvedAt, &resolvedByUserID, &ignoreUntil, &comments, &codeSnippet, &surroundingContext,
			&externalIDNull, &externalPlatformNull, &externalURLNull,
			&fingerprintHash, &jiraSync, &trelloSync,
			&issue.CreatedAt, &issue.UpdatedAt,
			&issue.RepositoryName, &issue.RepositoryFullName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pending sync issue: %w", err)
		}

		if lineNumber.Valid {
			ln := int(lineNumber.Int64)
			issue.LineNumber = &ln
		}
		if columnNumber.Valid {
			cn := int(columnNumber.Int64)
			issue.ColumnNumber = &cn
		}
		if description.Valid {
			issue.Description = &description.String
		}
		if toolRuleID.Valid {
			issue.ToolRuleID = &toolRuleID.String
		}
		if codeSnippet.Valid {
			issue.CodeSnippet = &codeSnippet.String
		}
		if surroundingContext.Valid {
			issue.SurroundingContext = &surroundingContext.String
		}
		if resolutionReason.Valid {
			issue.ResolutionReason = &resolutionReason.String
		}
		if assignedToUserID.Valid {
			id := uuid.MustParse(assignedToUserID.String)
			issue.AssignedToUserID = &id
		}
		if resolvedByUserID.Valid {
			id := uuid.MustParse(resolvedByUserID.String)
			issue.ResolvedByUserID = &id
		}
		if resolvedAt.Valid {
			issue.ResolvedAt = &resolvedAt.Time
		}
		if ignoreUntil.Valid {
			issue.IgnoreUntil = &ignoreUntil.Time
		}
		if externalIDNull.Valid {
			issue.ExternalID = &externalIDNull.String
		}
		if externalPlatformNull.Valid {
			issue.ExternalPlatform = &externalPlatformNull.String
		}
		if externalURLNull.Valid {
			issue.ExternalURL = &externalURLNull.String
		}
		if fingerprintHash.Valid {
			issue.FingerprintHash = fingerprintHash.String
		}
		if jiraSync.Valid {
			issue.JiraSyncStatus = jiraSync.String
		}
		if trelloSync.Valid {
			issue.TrelloSyncStatus = trelloSync.String
		}
		issue.Comments = comments
		issues = append(issues, issue)
	}

	return issues, nil
}
