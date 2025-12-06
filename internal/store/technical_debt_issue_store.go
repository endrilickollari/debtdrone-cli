package store

import (
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

type TechnicalDebtIssueStoreInterface interface {
	Create(issue *models.TechnicalDebtIssue) error
	BatchCreate(issues []models.TechnicalDebtIssue) error
	Get(id string) (*models.TechnicalDebtIssue, error)
	List(limit, offset int) ([]models.TechnicalDebtIssue, error)
	ListWithFilters(filters IssueFilters, limit, offset int) ([]models.TechnicalDebtIssue, int, error)
	Update(issue *models.TechnicalDebtIssue) error
	IssueExists(repositoryID uuid.UUID, filePath string, lineNumber *int, issueType string, toolRuleID *string) (bool, error)
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
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
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
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
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
		       i.ignore_until, i.comments, i.code_snippet, i.surrounding_context, i.created_at, i.updated_at,
		       COALESCE(r.name, '') as repository_name,
		       COALESCE(r.full_name, '') as repository_full_name
		FROM technical_debt_issues i
		LEFT JOIN user_repositories r ON i.repository_id = r.id
		WHERE i.id = $1
	`

	var issue models.TechnicalDebtIssue
	var assignedTo, resolvedBy sql.NullString
	err := s.db.QueryRow(query, id).Scan(
		&issue.ID, &issue.UserID, &issue.RepositoryID, &issue.AnalysisRunID, &issue.FilePath,
		&issue.LineNumber, &issue.ColumnNumber, &issue.IssueType, &issue.Severity, &issue.Category,
		&issue.Message, &issue.Description, &issue.ToolName, &issue.ToolRuleID,
		&issue.ConfidenceScore, &issue.TechnicalDebtHours, &issue.EffortMultiplier, &issue.Status,
		&issue.ResolutionReason, &assignedTo, &issue.ResolvedAt, &resolvedBy,
		&issue.IgnoreUntil, pq.Array(&issue.Comments), &issue.CodeSnippet, &issue.SurroundingContext, &issue.CreatedAt, &issue.UpdatedAt,
		&issue.RepositoryName, &issue.RepositoryFullName,
	)
	if err != nil {
		return nil, err
	}
	issue.AssignedToUserID = scanNullableUUID(assignedTo)
	issue.ResolvedByUserID = scanNullableUUID(resolvedBy)
	return &issue, nil
}

func (s *DBTechnicalDebtIssueStore) List(limit, offset int) ([]models.TechnicalDebtIssue, error) {
	query := `
		SELECT id, user_id, repository_id, analysis_run_id, file_path, line_number, column_number,
		       issue_type, severity, category, message, description, tool_name, tool_rule_id,
		       confidence_score, technical_debt_hours, effort_multiplier, status,
		       resolution_reason, assigned_to_user_id, resolved_at, resolved_by_user_id,
		       ignore_until, comments, code_snippet, surrounding_context, created_at, updated_at
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
		err := rows.Scan(
			&issue.ID, &issue.UserID, &issue.RepositoryID, &issue.AnalysisRunID, &issue.FilePath,
			&issue.LineNumber, &issue.ColumnNumber, &issue.IssueType, &issue.Severity, &issue.Category,
			&issue.Message, &issue.Description, &issue.ToolName, &issue.ToolRuleID,
			&issue.ConfidenceScore, &issue.TechnicalDebtHours, &issue.EffortMultiplier, &issue.Status,
			&issue.ResolutionReason, &assignedTo, &issue.ResolvedAt, &resolvedBy,
			&issue.IgnoreUntil, pq.Array(&issue.Comments), &issue.CodeSnippet, &issue.SurroundingContext, &issue.CreatedAt, &issue.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		issue.AssignedToUserID = scanNullableUUID(assignedTo)
		issue.ResolvedByUserID = scanNullableUUID(resolvedBy)
		issues = append(issues, issue)
	}
	return issues, nil
}

func (s *DBTechnicalDebtIssueStore) ListWithFilters(filters IssueFilters, limit, offset int) ([]models.TechnicalDebtIssue, int, error) {
	whereClauses := []string{}
	args := []interface{}{}
	argCount := 1

	if filters.UserID != nil && *filters.UserID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("i.user_id = $%d", argCount))
		args = append(args, *filters.UserID)
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
		whereClauses = append(whereClauses, fmt.Sprintf("i.repository_id = $%d", argCount))
		args = append(args, *filters.RepositoryID)
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

	query := fmt.Sprintf(`
		SELECT
			i.id, i.user_id, i.repository_id, i.analysis_run_id, i.file_path, i.line_number, i.column_number,
			i.issue_type, i.severity, i.category, i.message, i.description, i.tool_name, i.tool_rule_id,
			i.confidence_score, i.technical_debt_hours, i.effort_multiplier, i.status,
			i.resolution_reason, i.assigned_to_user_id, i.resolved_at, i.resolved_by_user_id,
			i.ignore_until, i.comments, i.code_snippet, i.surrounding_context, i.created_at, i.updated_at,
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
		err := rows.Scan(
			&issue.ID, &issue.UserID, &issue.RepositoryID, &issue.AnalysisRunID, &issue.FilePath,
			&issue.LineNumber, &issue.ColumnNumber, &issue.IssueType, &issue.Severity, &issue.Category,
			&issue.Message, &issue.Description, &issue.ToolName, &issue.ToolRuleID,
			&issue.ConfidenceScore, &issue.TechnicalDebtHours, &issue.EffortMultiplier, &issue.Status,
			&issue.ResolutionReason, &assignedTo, &issue.ResolvedAt, &resolvedBy,
			&issue.IgnoreUntil, pq.Array(&issue.Comments), &issue.CodeSnippet, &issue.SurroundingContext, &issue.CreatedAt, &issue.UpdatedAt,
			&issue.RepositoryName, &issue.RepositoryFullName,
		)
		if err != nil {
			return nil, 0, err
		}
		issue.AssignedToUserID = scanNullableUUID(assignedTo)
		issue.ResolvedByUserID = scanNullableUUID(resolvedBy)
		issues = append(issues, issue)
	}

	return issues, total, nil
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
