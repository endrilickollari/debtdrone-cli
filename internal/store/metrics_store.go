package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

type MetricsStoreInterface interface {
	CreateMetricsSnapshot(ctx context.Context, userID, repositoryID uuid.UUID) error
	CreateSnapshot(repositoryID string) error
	GetMetricsSnapshots(ctx context.Context, repositoryID uuid.UUID, startDate, endDate time.Time) ([]models.RepositoryMetricsSnapshot, error)
	GetLatestSnapshot(ctx context.Context, repositoryID uuid.UUID) (*models.RepositoryMetricsSnapshot, error)
	RecordUserActivity(ctx context.Context, activity models.UserActivity) error
	GetActiveUsersCount(ctx context.Context, startDate, endDate time.Time) (int, error)
	GetResolvedIssuesCount(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (int, error)
	SetMetricsCache(ctx context.Context, cache models.DashboardMetricsCache) error
	GetMetricsCache(ctx context.Context, userID uuid.UUID, metricType string) (*models.DashboardMetricsCache, error)
	CleanupExpiredCache(ctx context.Context) error
	GetDashboardStats(ctx context.Context, userID uuid.UUID) (*models.DashboardStats, error)
}

type MetricsStore struct {
	db *sql.DB
}

func NewMetricsStore(db *sql.DB) MetricsStoreInterface {
	return &MetricsStore{db: db}
}

func (s *MetricsStore) CreateMetricsSnapshot(ctx context.Context, userID, repositoryID uuid.UUID) error {
	query := `SELECT create_metrics_snapshot($1, $2)`
	_, err := s.db.ExecContext(ctx, query, userID, repositoryID)
	return err
}

func (s *MetricsStore) CreateSnapshot(repositoryID string) error {
	repoUUID, err := uuid.Parse(repositoryID)
	if err != nil {
		return fmt.Errorf("invalid repository ID: %w", err)
	}

	var userID uuid.UUID
	query := `SELECT user_id FROM user_repositories WHERE id = $1`
	err = s.db.QueryRow(query, repoUUID).Scan(&userID)
	if err != nil {
		return fmt.Errorf("failed to get user ID for repository: %w", err)
	}

	return s.CreateMetricsSnapshot(context.Background(), userID, repoUUID)
}

func (s *MetricsStore) GetMetricsSnapshots(ctx context.Context, repositoryID uuid.UUID, startDate, endDate time.Time) ([]models.RepositoryMetricsSnapshot, error) {
	query := `
		SELECT id, user_id, repository_id, snapshot_date, total_issues_count,
		       critical_issues_count, high_issues_count, medium_issues_count, low_issues_count,
		       technical_debt_hours, test_coverage_percentage, duplication_percentage,
		       complexity_score, created_at
		FROM repository_metrics_snapshots
		WHERE repository_id = $1 AND snapshot_date BETWEEN $2 AND $3
		ORDER BY snapshot_date ASC
	`

	rows, err := s.db.QueryContext(ctx, query, repositoryID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []models.RepositoryMetricsSnapshot
	for rows.Next() {
		var snapshot models.RepositoryMetricsSnapshot
		err := rows.Scan(
			&snapshot.ID,
			&snapshot.UserID,
			&snapshot.RepositoryID,
			&snapshot.SnapshotDate,
			&snapshot.TotalIssuesCount,
			&snapshot.CriticalIssuesCount,
			&snapshot.HighIssuesCount,
			&snapshot.MediumIssuesCount,
			&snapshot.LowIssuesCount,
			&snapshot.TechnicalDebtHours,
			&snapshot.TestCoveragePercentage,
			&snapshot.DuplicationPercentage,
			&snapshot.ComplexityScore,
			&snapshot.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}

	return snapshots, nil
}

func (s *MetricsStore) GetLatestSnapshot(ctx context.Context, repositoryID uuid.UUID) (*models.RepositoryMetricsSnapshot, error) {
	query := `
		SELECT id, user_id, repository_id, snapshot_date, total_issues_count,
		       critical_issues_count, high_issues_count, medium_issues_count, low_issues_count,
		       technical_debt_hours, test_coverage_percentage, duplication_percentage,
		       complexity_score, created_at
		FROM repository_metrics_snapshots
		WHERE repository_id = $1
		ORDER BY snapshot_date DESC, created_at DESC
		LIMIT 1
	`

	var snapshot models.RepositoryMetricsSnapshot
	err := s.db.QueryRowContext(ctx, query, repositoryID).Scan(
		&snapshot.ID,
		&snapshot.UserID,
		&snapshot.RepositoryID,
		&snapshot.SnapshotDate,
		&snapshot.TotalIssuesCount,
		&snapshot.CriticalIssuesCount,
		&snapshot.HighIssuesCount,
		&snapshot.MediumIssuesCount,
		&snapshot.LowIssuesCount,
		&snapshot.TechnicalDebtHours,
		&snapshot.TestCoveragePercentage,
		&snapshot.DuplicationPercentage,
		&snapshot.ComplexityScore,
		&snapshot.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &snapshot, nil
}

func (s *MetricsStore) RecordUserActivity(ctx context.Context, activity models.UserActivity) error {
	var metadataJSON []byte
	var err error
	if activity.Metadata != nil {
		metadataJSON, err = json.Marshal(activity.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO user_activities (user_id, activity_type, resource_type, resource_id, activity_date, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`

	err = s.db.QueryRowContext(
		ctx,
		query,
		activity.UserID,
		activity.ActivityType,
		activity.ResourceType,
		activity.ResourceID,
		time.Now(),
		metadataJSON,
	).Scan(&activity.ID, &activity.CreatedAt)

	return err
}

func (s *MetricsStore) GetActiveUsersCount(ctx context.Context, startDate, endDate time.Time) (int, error) {
	query := `
		SELECT COUNT(DISTINCT user_id)
		FROM user_activities
		WHERE activity_date BETWEEN $1 AND $2
	`

	var count int
	err := s.db.QueryRowContext(ctx, query, startDate, endDate).Scan(&count)
	return count, err
}

func (s *MetricsStore) GetResolvedIssuesCount(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM technical_debt_issues
		WHERE user_id = $1
		  AND status = 'resolved'
		  AND resolved_at BETWEEN $2 AND $3
	`

	var count int
	err := s.db.QueryRowContext(ctx, query, userID, startDate, endDate).Scan(&count)
	return count, err
}

func (s *MetricsStore) SetMetricsCache(ctx context.Context, cache models.DashboardMetricsCache) error {
	valueJSON, err := json.Marshal(cache.MetricValue)
	if err != nil {
		return fmt.Errorf("failed to marshal metric value: %w", err)
	}

	query := `
		INSERT INTO dashboard_metrics_cache (user_id, metric_type, metric_value, calculated_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, metric_type)
		DO UPDATE SET
			metric_value = EXCLUDED.metric_value,
			calculated_at = EXCLUDED.calculated_at,
			expires_at = EXCLUDED.expires_at
		RETURNING id
	`

	return s.db.QueryRowContext(
		ctx,
		query,
		cache.UserID,
		cache.MetricType,
		valueJSON,
		cache.CalculatedAt,
		cache.ExpiresAt,
	).Scan(&cache.ID)
}

func (s *MetricsStore) GetMetricsCache(ctx context.Context, userID uuid.UUID, metricType string) (*models.DashboardMetricsCache, error) {
	query := `
		SELECT id, user_id, metric_type, metric_value, calculated_at, expires_at
		FROM dashboard_metrics_cache
		WHERE user_id = $1 AND metric_type = $2 AND expires_at > NOW()
	`

	var cache models.DashboardMetricsCache
	var valueJSON []byte

	err := s.db.QueryRowContext(ctx, query, userID, metricType).Scan(
		&cache.ID,
		&cache.UserID,
		&cache.MetricType,
		&valueJSON,
		&cache.CalculatedAt,
		&cache.ExpiresAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(valueJSON, &cache.MetricValue); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metric value: %w", err)
	}

	return &cache, nil
}

func (s *MetricsStore) CleanupExpiredCache(ctx context.Context) error {
	query := `SELECT cleanup_expired_metrics_cache()`
	_, err := s.db.ExecContext(ctx, query)
	return err
}

func CalculateTrend(current, previous float64) models.MetricsTrend {
	trend := models.MetricsTrend{
		CurrentValue:  current,
		PreviousValue: previous,
	}

	if previous == 0 {
		if current > 0 {
			trend.Direction = "up"
			trend.Change = current
			trend.ChangePercent = 100
		} else {
			trend.Direction = "stable"
		}
		return trend
	}

	trend.Change = current - previous
	trend.ChangePercent = (trend.Change / previous) * 100

	if trend.Change > 0 {
		trend.Direction = "up"
	} else if trend.Change < 0 {
		trend.Direction = "down"
	} else {
		trend.Direction = "stable"
	}

	return trend
}

func (s *MetricsStore) GetDashboardStats(ctx context.Context, userID uuid.UUID) (*models.DashboardStats, error) {
	cached, err := s.GetMetricsCache(ctx, userID, models.MetricTypeDashboardStats)
	if err == nil && cached != nil {
		var stats models.DashboardStats
		statsJSON, _ := json.Marshal(cached.MetricValue)
		if err := json.Unmarshal(statsJSON, &stats); err == nil {
			return &stats, nil
		}
	}

	stats := &models.DashboardStats{}

	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM user_repositories WHERE user_id = $1
	`, userID).Scan(&stats.TotalRepositories)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM technical_debt_issues WHERE user_id = $1 AND status = 'open'
	`, userID).Scan(&stats.TotalIssues)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(latest_test_coverage_percentage), 0)
		FROM user_repositories
		WHERE user_id = $1 AND latest_test_coverage_percentage > 0
	`, userID).Scan(&stats.AvgCodeCoverage)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(latest_complexity_score), 0)
		FROM user_repositories
		WHERE user_id = $1 AND latest_complexity_score > 0
	`, userID).Scan(&stats.AvgComplexity)
	if err != nil {
		return nil, err
	}

	stats.ActiveUsersCount, _ = s.GetActiveUsersCount(ctx, time.Now().AddDate(0, 0, -30), time.Now())

	weekStart := time.Now().AddDate(0, 0, -7)
	stats.ResolvedThisWeek, _ = s.GetResolvedIssuesCount(ctx, userID, weekStart, time.Now())
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM analysis_runs WHERE user_id = $1 AND status = 'completed'
	`, userID).Scan(&stats.ReportsGenerated)
	if err != nil {
		return nil, err
	}

	lastMonth := time.Now().AddDate(0, -1, 0)

	var prevRepoCount int
	s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM user_repositories
		WHERE user_id = $1 AND created_at < $2
	`, userID, lastMonth).Scan(&prevRepoCount)
	stats.RepositoriesTrend = CalculateTrend(float64(stats.TotalRepositories), float64(prevRepoCount))

	var prevIssueCount int
	s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM technical_debt_issues
		WHERE user_id = $1 AND status = 'open' AND created_at < $2
	`, userID, lastMonth).Scan(&prevIssueCount)
	stats.IssuesTrend = CalculateTrend(float64(stats.TotalIssues), float64(prevIssueCount))

	cacheData := models.DashboardMetricsCache{
		UserID:       userID,
		MetricType:   models.MetricTypeDashboardStats,
		CalculatedAt: time.Now(),
		ExpiresAt:    time.Now().Add(5 * time.Minute),
	}
	statsJSON, _ := json.Marshal(stats)
	json.Unmarshal(statsJSON, &cacheData.MetricValue)
	s.SetMetricsCache(ctx, cacheData)

	return stats, nil
}
