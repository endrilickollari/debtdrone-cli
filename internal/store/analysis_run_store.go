package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)




type AnalysisRunStoreInterface interface {
	Create(run *models.AnalysisRun) error
	Get(id string) (*models.AnalysisRun, error)
	List(userID string, status string, limit, offset int) ([]models.AnalysisRun, error)
	Update(run *models.AnalysisRun) error
	UpdateStatus(ctx context.Context, runID uuid.UUID, status string, results map[string]interface{}) error
	GetStatus(ctx context.Context, id string) (string, error)
	GetBillableScanCount(orgID string, startOfMonth time.Time) (int64, error)
}

type DBAnalysisRunStore struct {
	db *sql.DB
}

func NewDBAnalysisRunStore(db *sql.DB) *DBAnalysisRunStore {
	return &DBAnalysisRunStore{db: db}
}

func (s *DBAnalysisRunStore) Create(run *models.AnalysisRun) error {
	query := `
		INSERT INTO analysis_runs (
			id, user_id, repository_id, user_config_id, run_type, trigger_source,
			started_at, status, analysis_config, commit_hash, branch, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	now := time.Now()
	run.CreatedAt = now
	run.UpdatedAt = now

	_, err := s.db.Exec(query,
		run.ID, run.UserID, run.RepositoryID, run.UserConfigID, run.RunType, run.TriggerSource,
		run.StartedAt, run.Status, run.AnalysisConfig, run.CommitHash, run.Branch, run.CreatedAt, run.UpdatedAt,
	)
	return err
}

func (s *DBAnalysisRunStore) Get(id string) (*models.AnalysisRun, error) {
	query := `
		SELECT id, user_id, repository_id, user_config_id, run_type, trigger_source,
		       started_at, completed_at, duration_seconds, status, analysis_config,
		       total_issues_found, critical_issues_count, high_issues_count, medium_issues_count, low_issues_count,
		       total_technical_debt_hours, test_coverage_percentage, duplication_percentage,
		       error_message, commit_hash, branch, created_at, updated_at
		FROM analysis_runs
		WHERE id = $1
	`

	var run models.AnalysisRun
	err := s.db.QueryRow(query, id).Scan(
		&run.ID, &run.UserID, &run.RepositoryID, &run.UserConfigID, &run.RunType, &run.TriggerSource,
		&run.StartedAt, &run.CompletedAt, &run.DurationSeconds, &run.Status, &run.AnalysisConfig,
		&run.TotalIssuesFound, &run.CriticalIssuesCount, &run.HighIssuesCount, &run.MediumIssuesCount, &run.LowIssuesCount,
		&run.TotalTechnicalDebtHours, &run.TestCoveragePercentage, &run.DuplicationPercentage,
		&run.ErrorMessage, &run.CommitHash, &run.Branch, &run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Calculate Delta
	prevRun, err := s.getPreviousRun(run.RepositoryID.String(), run.ID.String(), run.StartedAt)
	if err == nil && prevRun != nil {
		run.Delta = map[string]interface{}{
			"debt_hours_change":     run.TotalTechnicalDebtHours - prevRun.TotalTechnicalDebtHours,
			"critical_count_change": run.CriticalIssuesCount - prevRun.CriticalIssuesCount,
			"new_issues":            0, // Placeholder: requires issue diffing, expensive for lists.
			"fixed_issues":          0,
		}
	}

	return &run, nil
}

func (s *DBAnalysisRunStore) List(userID string, status string, limit, offset int) ([]models.AnalysisRun, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}

	baseQuery := `
		SELECT
			ar.id, ar.user_id, ar.repository_id, ar.user_config_id, ar.run_type, ar.trigger_source,
			ar.started_at, ar.completed_at, ar.duration_seconds, ar.status, ar.analysis_config,
			ar.total_issues_found, ar.critical_issues_count, ar.high_issues_count, ar.medium_issues_count, ar.low_issues_count,
			ar.total_technical_debt_hours, ar.test_coverage_percentage, ar.duplication_percentage,
			ar.error_message, ar.commit_hash, ar.branch, ar.created_at, ar.updated_at,
			r.name as repository_name, r.full_name as repository_full_name
		FROM analysis_runs ar
		LEFT JOIN user_repositories r ON ar.repository_id = r.id
		WHERE EXISTS (
			SELECT 1 FROM organization_members om
			JOIN user_repositories ur ON ur.organization_id = om.organization_id
			WHERE om.user_id = $1 AND ur.id = ar.repository_id
		)
	`
	args := []interface{}{uid}
	paramCount := 1

	if status != "" {
		paramCount++
		baseQuery += fmt.Sprintf(" AND ar.status = $%d", paramCount)
		args = append(args, status)
	}

	paramCount++
	baseQuery += fmt.Sprintf(" ORDER BY ar.created_at DESC LIMIT $%d", paramCount)
	args = append(args, limit)

	paramCount++
	baseQuery += fmt.Sprintf(" OFFSET $%d", paramCount)
	args = append(args, offset)

	rows, err := s.db.Query(baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []models.AnalysisRun
	for rows.Next() {
		var run models.AnalysisRun
		var repoName, repoFullName sql.NullString

		err := rows.Scan(
			&run.ID, &run.UserID, &run.RepositoryID, &run.UserConfigID, &run.RunType, &run.TriggerSource,
			&run.StartedAt, &run.CompletedAt, &run.DurationSeconds, &run.Status, &run.AnalysisConfig,
			&run.TotalIssuesFound, &run.CriticalIssuesCount, &run.HighIssuesCount, &run.MediumIssuesCount, &run.LowIssuesCount,
			&run.TotalTechnicalDebtHours, &run.TestCoveragePercentage, &run.DuplicationPercentage,
			&run.ErrorMessage, &run.CommitHash, &run.Branch, &run.CreatedAt, &run.UpdatedAt,
			&repoName, &repoFullName,
		)
		if err != nil {
			return nil, err
		}
		if repoName.Valid {
			run.RepositoryName = &repoName.String
		}
		if repoFullName.Valid {
			run.RepositoryFullName = &repoFullName.String
		}
		runs = append(runs, run)
	}

	return runs, nil
}


func (s *DBAnalysisRunStore) getPreviousRun(repoID, _ string, startedAt time.Time) (*models.AnalysisRun, error) {
	query := `
		SELECT total_technical_debt_hours, critical_issues_count
		FROM analysis_runs
		WHERE repository_id = $1
		  AND status = 'completed'
		  AND completed_at < $2
		ORDER BY completed_at DESC
		LIMIT 1
	`
	var run models.AnalysisRun
	// We use startedAt of current run as the cutoff, assuming previous run completed before this one started.
	// Or we can use the ID excluding itself if timestamps are close.
	// Directive says "look up the previous scan".
	err := s.db.QueryRow(query, repoID, startedAt).Scan(&run.TotalTechnicalDebtHours, &run.CriticalIssuesCount)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *DBAnalysisRunStore) Update(run *models.AnalysisRun) error {
	query := `
		UPDATE analysis_runs
		SET completed_at = $1, duration_seconds = $2, status = $3,
		    total_issues_found = $4, critical_issues_count = $5, high_issues_count = $6,
		    medium_issues_count = $7, low_issues_count = $8, total_technical_debt_hours = $9,
		    test_coverage_percentage = $10, duplication_percentage = $11, error_message = $12,
		    updated_at = $13
		WHERE id = $14
	`

	run.UpdatedAt = time.Now()

	_, err := s.db.Exec(query,
		run.CompletedAt, run.DurationSeconds, run.Status,
		run.TotalIssuesFound, run.CriticalIssuesCount, run.HighIssuesCount,
		run.MediumIssuesCount, run.LowIssuesCount, run.TotalTechnicalDebtHours,
		run.TestCoveragePercentage, run.DuplicationPercentage, run.ErrorMessage,
		run.UpdatedAt, run.ID,
	)
	return err
}

func (s *DBAnalysisRunStore) UpdateStatus(ctx context.Context, runID uuid.UUID, status string, results map[string]interface{}) error {
	now := time.Now()

	// Extract commit hash and branch if present in results
	var commitHash *string
	var branch *string

	if results != nil {
		if v, ok := results["commit_hash"].(string); ok {
			commitHash = &v
		}
		if v, ok := results["branch"].(string); ok {
			branch = &v
		}
	}

	if status == "completed" || status == "failed" {
		var startedAt time.Time
		err := s.db.QueryRowContext(ctx, "SELECT started_at FROM analysis_runs WHERE id = $1", runID).Scan(&startedAt)
		if err != nil {
			return err
		}

		durationSeconds := int(now.Sub(startedAt).Seconds())

		totalIssues := 0
		criticalCount := 0
		highCount := 0
		mediumCount := 0
		lowCount := 0
		totalDebtHours := 0.0
		coveragePercent := 0.0
		duplicationPercent := 0.0
		var errorMsg *string

		if results != nil {
			log.Printf("📊 Analysis metrics: %+v", results)

			// total_issues_found
			if v, ok := results["total_issues_found"].(int); ok {
				totalIssues = v
			} else if v, ok := results["total_issues_found"].(float64); ok {
				totalIssues = int(v)
			}

			// critical_count
			if v, ok := results["critical_count"].(int); ok {
				criticalCount = v
			} else if v, ok := results["critical_count"].(float64); ok {
				criticalCount = int(v)
			}

			// high_count
			if v, ok := results["high_count"].(int); ok {
				highCount = v
			} else if v, ok := results["high_count"].(float64); ok {
				highCount = int(v)
			}

			// medium_count
			if v, ok := results["medium_count"].(int); ok {
				mediumCount = v
			} else if v, ok := results["medium_count"].(float64); ok {
				mediumCount = int(v)
			}

			// low_count
			if v, ok := results["low_count"].(int); ok {
				lowCount = v
			} else if v, ok := results["low_count"].(float64); ok {
				lowCount = int(v)
			}

			// total_debt_hours
			if v, ok := results["total_debt_hours"].(float64); ok {
				totalDebtHours = v
			} else if v, ok := results["total_debt_hours"].(int); ok {
				totalDebtHours = float64(v)
			}

			// test_coverage_percentage
			if v, ok := results["test_coverage_percentage"].(float64); ok {
				coveragePercent = v
			} else if v, ok := results["test_coverage_percentage"].(int); ok {
				coveragePercent = float64(v)
			}

			// duplication_percentage
			if v, ok := results["duplication_percentage"].(float64); ok {
				duplicationPercent = v
			} else if v, ok := results["duplication_percentage"].(int); ok {
				duplicationPercent = float64(v)
			}

			// error message
			if v, ok := results["error"].(string); ok {
				errorMsg = &v
			}

			log.Printf("💾 Saving metrics - Issues: %d (C:%d H:%d M:%d L:%d), Debt: %.1fh, Commit: %v",
				totalIssues, criticalCount, highCount, mediumCount, lowCount, totalDebtHours, commitHash)
		}

		query := `
			UPDATE analysis_runs
			SET status = $1, completed_at = $2, duration_seconds = $3,
				total_issues_found = $4, critical_issues_count = $5, high_issues_count = $6,
				medium_issues_count = $7, low_issues_count = $8, total_technical_debt_hours = $9,
				test_coverage_percentage = $10, duplication_percentage = $11, error_message = $12,
				commit_hash = COALESCE($13, commit_hash), branch = COALESCE($14, branch),
				updated_at = $15
			WHERE id = $16
		`

		_, err = s.db.ExecContext(ctx, query,
			status, now, durationSeconds,
			totalIssues, criticalCount, highCount, mediumCount, lowCount, totalDebtHours,
			coveragePercent, duplicationPercent, errorMsg,
			commitHash, branch,
			now, runID,
		)
		return err
	}

	query := `
		UPDATE analysis_runs
		SET status = $1, 
		    commit_hash = COALESCE($2, commit_hash), 
		    branch = COALESCE($3, branch),
		    updated_at = $4
		WHERE id = $5
	`

	_, err := s.db.ExecContext(ctx, query, status, commitHash, branch, now, runID)
	return err
}

func (s *DBAnalysisRunStore) GetBillableScanCount(orgID string, startOfMonth time.Time) (int64, error) {
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		return 0, err
	}

	query := `
		SELECT COUNT(ar.id)
		FROM analysis_runs ar
		JOIN user_repositories ur ON ar.repository_id = ur.id
		WHERE ur.organization_id = $1
		  AND ar.created_at >= $2
		  AND ar.status != 'failed'
		  AND ar.run_type = 'manual'
	`

	var count int64
	err = s.db.QueryRow(query, orgUUID, startOfMonth).Scan(&count)
	if err != nil {
		log.Printf("❌ [AnalysisStore] Failed to count billable scans: %v", err)
		return 0, err
	}

	return count, nil
}

func (s *DBAnalysisRunStore) GetStatus(ctx context.Context, id string) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, "SELECT status FROM analysis_runs WHERE id = $1", id).Scan(&status)
	return status, err
}
