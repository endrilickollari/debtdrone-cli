package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

type ComplexityStoreInterface interface {
	BatchCreate(ctx context.Context, metrics []models.ComplexityMetric) error
	GetByAnalysisRun(ctx context.Context, analysisRunID uuid.UUID) ([]models.ComplexityMetric, error)
	GetByRepository(ctx context.Context, repositoryID uuid.UUID, filters ComplexityFilters) ([]models.ComplexityMetric, error)
	GetFileSummary(ctx context.Context, analysisRunID uuid.UUID, filePath string) (*models.FileComplexitySummary, error)
	GetRepositorySummary(ctx context.Context, analysisRunID uuid.UUID) (*models.RepositoryComplexitySummary, error)
}

type ComplexityStore struct {
	db *sql.DB
}

func NewComplexityStore(db *sql.DB) *ComplexityStore {
	return &ComplexityStore{db: db}
}

func (s *ComplexityStore) Create(ctx context.Context, metric *models.ComplexityMetric) error {
	suggestionsJSON, err := json.Marshal(metric.RefactoringSuggestions)
	if err != nil {
		return fmt.Errorf("failed to marshal refactoring suggestions: %w", err)
	}

	query := `
		INSERT INTO complexity_metrics (
			id, user_id, repository_id, analysis_run_id,
			file_path, function_name, start_line, end_line, start_column, end_column,
			cyclomatic_complexity, cognitive_complexity, nesting_depth, parameter_count, lines_of_code,
			halstead_volume, halstead_difficulty, halstead_effort, halstead_time, halstead_bugs,
			severity, complexity_category, technical_debt_minutes,
			code_snippet, refactoring_suggestions, language, metadata
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20,
			$21, $22, $23,
			$24, $25, $26, $27
		)
	`

	_, err = s.db.ExecContext(ctx, query,
		metric.ID, metric.UserID, metric.RepositoryID, metric.AnalysisRunID,
		metric.FilePath, metric.FunctionName, metric.StartLine, metric.EndLine, metric.StartColumn, metric.EndColumn,
		metric.CyclomaticComplexity, metric.CognitiveComplexity, metric.NestingDepth, metric.ParameterCount, metric.LinesOfCode,
		metric.HalsteadVolume, metric.HalsteadDifficulty, metric.HalsteadEffort, metric.HalsteadTime, metric.HalsteadBugs,
		metric.Severity, metric.ComplexityCategory, metric.TechnicalDebtMinutes,
		metric.CodeSnippet, suggestionsJSON, metric.Language, metric.Metadata,
	)

	return err
}

func (s *ComplexityStore) BatchCreate(ctx context.Context, metrics []models.ComplexityMetric) error {
	if len(metrics) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO complexity_metrics (
			id, user_id, repository_id, analysis_run_id,
			file_path, function_name, start_line, end_line, start_column, end_column,
			cyclomatic_complexity, cognitive_complexity, nesting_depth, parameter_count, lines_of_code,
			halstead_volume, halstead_difficulty, halstead_effort, halstead_time, halstead_bugs,
			severity, complexity_category, technical_debt_minutes,
			code_snippet, refactoring_suggestions, language, metadata
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20,
			$21, $22, $23,
			$24, $25, $26, $27
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, metric := range metrics {
		suggestionsJSON, err := json.Marshal(metric.RefactoringSuggestions)
		if err != nil {
			return fmt.Errorf("failed to marshal refactoring suggestions: %w", err)
		}

		_, err = stmt.ExecContext(ctx,
			metric.ID, metric.UserID, metric.RepositoryID, metric.AnalysisRunID,
			metric.FilePath, metric.FunctionName, metric.StartLine, metric.EndLine, metric.StartColumn, metric.EndColumn,
			metric.CyclomaticComplexity, metric.CognitiveComplexity, metric.NestingDepth, metric.ParameterCount, metric.LinesOfCode,
			metric.HalsteadVolume, metric.HalsteadDifficulty, metric.HalsteadEffort, metric.HalsteadTime, metric.HalsteadBugs,
			metric.Severity, metric.ComplexityCategory, metric.TechnicalDebtMinutes,
			metric.CodeSnippet, suggestionsJSON, metric.Language, metric.Metadata,
		)
		if err != nil {
			return fmt.Errorf("failed to insert metric: %w", err)
		}
	}

	return tx.Commit()
}

func (s *ComplexityStore) GetByAnalysisRun(ctx context.Context, analysisRunID uuid.UUID) ([]models.ComplexityMetric, error) {
	query := `
		SELECT 
			id, user_id, repository_id, analysis_run_id,
			file_path, function_name, start_line, end_line, start_column, end_column,
			cyclomatic_complexity, cognitive_complexity, nesting_depth, parameter_count, lines_of_code,
			halstead_volume, halstead_difficulty, halstead_effort, halstead_time, halstead_bugs,
			severity, complexity_category, technical_debt_minutes,
			code_snippet, refactoring_suggestions, language, metadata,
			created_at, updated_at
		FROM complexity_metrics
		WHERE analysis_run_id = $1
		ORDER BY cyclomatic_complexity DESC, file_path, start_line
	`

	rows, err := s.db.QueryContext(ctx, query, analysisRunID)
	if err != nil {
		return nil, fmt.Errorf("failed to query complexity metrics: %w", err)
	}
	defer rows.Close()

	var metrics []models.ComplexityMetric
	for rows.Next() {
		var metric models.ComplexityMetric
		var suggestionsJSON []byte

		err := rows.Scan(
			&metric.ID, &metric.UserID, &metric.RepositoryID, &metric.AnalysisRunID,
			&metric.FilePath, &metric.FunctionName, &metric.StartLine, &metric.EndLine, &metric.StartColumn, &metric.EndColumn,
			&metric.CyclomaticComplexity, &metric.CognitiveComplexity, &metric.NestingDepth, &metric.ParameterCount, &metric.LinesOfCode,
			&metric.HalsteadVolume, &metric.HalsteadDifficulty, &metric.HalsteadEffort, &metric.HalsteadTime, &metric.HalsteadBugs,
			&metric.Severity, &metric.ComplexityCategory, &metric.TechnicalDebtMinutes,
			&metric.CodeSnippet, &suggestionsJSON, &metric.Language, &metric.Metadata,
			&metric.CreatedAt, &metric.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric: %w", err)
		}

		if len(suggestionsJSON) > 0 {
			var suggestions []models.RefactoringSuggestion
			if err := json.Unmarshal(suggestionsJSON, &suggestions); err == nil {
				metric.RefactoringSuggestions = suggestions
			}
		}

		metrics = append(metrics, metric)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics: %w", err)
	}

	return metrics, nil
}

func (s *ComplexityStore) GetByRepository(ctx context.Context, repositoryID uuid.UUID, filters ComplexityFilters) ([]models.ComplexityMetric, error) {
	query := `
		SELECT DISTINCT ON (cm.file_path, cm.function_name)
			cm.id, cm.user_id, cm.repository_id, cm.analysis_run_id,
			cm.file_path, cm.function_name, cm.start_line, cm.end_line, cm.start_column, cm.end_column,
			cm.cyclomatic_complexity, cm.cognitive_complexity, cm.nesting_depth, cm.parameter_count, cm.lines_of_code,
			cm.halstead_volume, cm.halstead_difficulty, cm.halstead_effort, cm.halstead_time, cm.halstead_bugs,
			cm.severity, cm.complexity_category, cm.technical_debt_minutes,
			cm.code_snippet, cm.refactoring_suggestions, cm.language, cm.metadata,
			cm.created_at, cm.updated_at
		FROM complexity_metrics cm
		INNER JOIN analysis_runs ar ON cm.analysis_run_id = ar.id
		WHERE cm.repository_id = $1
	`

	args := []interface{}{repositoryID}
	argNum := 2

	if filters.Severity != "" {
		query += fmt.Sprintf(" AND cm.severity = $%d", argNum)
		args = append(args, filters.Severity)
		argNum++
	}

	if filters.MinComplexity > 0 {
		query += fmt.Sprintf(" AND cm.cyclomatic_complexity >= $%d", argNum)
		args = append(args, filters.MinComplexity)
		argNum++
	}

	if filters.FilePath != "" {
		query += fmt.Sprintf(" AND cm.file_path = $%d", argNum)
		args = append(args, filters.FilePath)
		argNum++
	}

	query += ` ORDER BY cm.file_path, cm.function_name, ar.started_at DESC`

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, filters.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query complexity metrics: %w", err)
	}
	defer rows.Close()

	var metrics []models.ComplexityMetric
	for rows.Next() {
		var metric models.ComplexityMetric
		var suggestionsJSON []byte

		err := rows.Scan(
			&metric.ID, &metric.UserID, &metric.RepositoryID, &metric.AnalysisRunID,
			&metric.FilePath, &metric.FunctionName, &metric.StartLine, &metric.EndLine, &metric.StartColumn, &metric.EndColumn,
			&metric.CyclomaticComplexity, &metric.CognitiveComplexity, &metric.NestingDepth, &metric.ParameterCount, &metric.LinesOfCode,
			&metric.HalsteadVolume, &metric.HalsteadDifficulty, &metric.HalsteadEffort, &metric.HalsteadTime, &metric.HalsteadBugs,
			&metric.Severity, &metric.ComplexityCategory, &metric.TechnicalDebtMinutes,
			&metric.CodeSnippet, &suggestionsJSON, &metric.Language, &metric.Metadata,
			&metric.CreatedAt, &metric.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric: %w", err)
		}

		if len(suggestionsJSON) > 0 {
			var suggestions []models.RefactoringSuggestion
			if err := json.Unmarshal(suggestionsJSON, &suggestions); err == nil {
				metric.RefactoringSuggestions = suggestions
			}
		}

		metrics = append(metrics, metric)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics: %w", err)
	}

	return metrics, nil
}

func (s *ComplexityStore) GetFileSummary(ctx context.Context, analysisRunID uuid.UUID, filePath string) (*models.FileComplexitySummary, error) {
	query := `
		SELECT 
			repository_id, analysis_run_id, file_path, language,
			function_count, avg_cyclomatic_complexity, max_cyclomatic_complexity,
			avg_cognitive_complexity, max_cognitive_complexity, avg_nesting_depth, max_nesting_depth,
			total_lines_of_code, total_technical_debt_minutes,
			critical_functions, high_complexity_functions, medium_complexity_functions, low_complexity_functions
		FROM file_complexity_summary
		WHERE analysis_run_id = $1 AND file_path = $2
	`

	var summary models.FileComplexitySummary
	err := s.db.QueryRowContext(ctx, query, analysisRunID, filePath).Scan(
		&summary.RepositoryID, &summary.AnalysisRunID, &summary.FilePath, &summary.Language,
		&summary.FunctionCount, &summary.AvgCyclomaticComplexity, &summary.MaxCyclomaticComplexity,
		&summary.AvgCognitiveComplexity, &summary.MaxCognitiveComplexity, &summary.AvgNestingDepth, &summary.MaxNestingDepth,
		&summary.TotalLinesOfCode, &summary.TotalTechnicalDebtMinutes,
		&summary.CriticalFunctions, &summary.HighComplexityFunctions, &summary.MediumComplexityFunctions, &summary.LowComplexityFunctions,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file complexity summary: %w", err)
	}

	return &summary, nil
}

func (s *ComplexityStore) GetRepositorySummary(ctx context.Context, analysisRunID uuid.UUID) (*models.RepositoryComplexitySummary, error) {
	query := `
		SELECT 
			repository_id, analysis_run_id, analyzed_files_count, total_functions,
			avg_cyclomatic_complexity, max_cyclomatic_complexity,
			avg_cognitive_complexity, max_cognitive_complexity,
			total_complexity_debt_hours, critical_complexity_count, high_complexity_count,
			deep_nesting_count, long_parameter_list_count,
			critical_issues, high_issues, medium_issues, low_issues
		FROM repository_complexity_summary
		WHERE analysis_run_id = $1
	`

	var summary models.RepositoryComplexitySummary
	err := s.db.QueryRowContext(ctx, query, analysisRunID).Scan(
		&summary.RepositoryID, &summary.AnalysisRunID, &summary.AnalyzedFilesCount, &summary.TotalFunctions,
		&summary.AvgCyclomaticComplexity, &summary.MaxCyclomaticComplexity,
		&summary.AvgCognitiveComplexity, &summary.MaxCognitiveComplexity,
		&summary.TotalComplexityDebtHours, &summary.CriticalComplexityCount, &summary.HighComplexityCount,
		&summary.DeepNestingCount, &summary.LongParameterListCount,
		&summary.CriticalIssues, &summary.HighIssues, &summary.MediumIssues, &summary.LowIssues,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get repository complexity summary: %w", err)
	}

	return &summary, nil
}

type ComplexityFilters struct {
	Severity      string
	MinComplexity int
	FilePath      string
	Limit         int
}
