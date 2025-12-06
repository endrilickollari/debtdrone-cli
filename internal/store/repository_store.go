package store

import (
	"database/sql"
	"log"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

type RepositoryStoreInterface interface {
	Create(repo *models.UserRepository) error
	Update(repo *models.UserRepository) error
	Delete(id string) error
	GetByID(id string) (*models.UserRepository, error)
	GetByFullName(userID, fullName string) (*models.UserRepository, error)
	ListByUserID(userID string) ([]*models.UserRepository, error)
	ListByConfigID(configID string) ([]*models.UserRepository, error)
	UpsertRepository(repo *models.UserRepository) error
	MarkAsInaccessible(id string) error
	GetUserRepositoriesPaginated(ctx interface{}, userID string, page, pageSize int) ([]*models.UserRepository, int64, error)
	GetRepositoryStats(ctx interface{}, userID string) (*RepositoryStats, error)
	UpdateMetrics(id string, debt float64, coverage float64, complexity float64, critical, high, medium, low int) error
}

type RepositoryStats struct {
	TotalRepositories int                    `json:"total_repositories"`
	PrivateRepos      int                    `json:"private_repos"`
	TotalSizeBytes    int64                  `json:"total_size_bytes"`
	ActiveToday       int                    `json:"active_today"`
	RecentRepos       []RecentRepository     `json:"recent_repos"`
	TopLanguages      []LanguageDistribution `json:"top_languages"`
	PlatformCount     map[string]int         `json:"platform_count"`
}

type RecentRepository struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	FullName        string    `json:"full_name"`
	URL             string    `json:"url"`
	PlatformType    string    `json:"platform_type"`
	PrimaryLanguage string    `json:"primary_language"`
	SizeBytes       int64     `json:"size_bytes"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type LanguageDistribution struct {
	Language string `json:"language"`
	Count    int    `json:"count"`
}

type DBRepositoryStore struct {
	db *sql.DB
}

func NewDBRepositoryStore(db *sql.DB) *DBRepositoryStore {
	log.Println("📦 Initializing database-backed repository store")
	return &DBRepositoryStore{db: db}
}

func (s *DBRepositoryStore) Create(repo *models.UserRepository) error {
	log.Printf("🔵 [RepoStore] Creating repository: %s", repo.FullName)

	repo.ID = uuid.New()
	repo.CreatedAt = time.Now()
	repo.UpdatedAt = time.Now()
	repo.AnalysisEnabled = true

	query := `
		INSERT INTO user_repositories (
			id, user_id, user_config_id, name, full_name, url, platform_type,
			primary_language, size_bytes, default_branch, last_commit_date,
			is_private, is_fork, language_breakdown, analysis_enabled,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
		)
	`

	_, err := s.db.Exec(query,
		repo.ID, repo.UserID, repo.UserConfigID, repo.Name, repo.FullName,
		repo.URL, repo.PlatformType, repo.PrimaryLanguage, repo.SizeBytes,
		repo.DefaultBranch, repo.LastCommitDate, repo.IsPrivate, repo.IsFork,
		repo.LanguageBreakdown, repo.AnalysisEnabled, repo.CreatedAt, repo.UpdatedAt,
	)

	if err != nil {
		log.Printf("❌ [RepoStore] Failed to create repository: %v", err)
		return err
	}

	log.Printf("✅ [RepoStore] Repository created: %s (id=%s)", repo.FullName, repo.ID)
	return nil
}

func (s *DBRepositoryStore) Update(repo *models.UserRepository) error {
	log.Printf("🔵 [RepoStore] Updating repository: %s", repo.FullName)

	repo.UpdatedAt = time.Now()

	query := `
		UPDATE user_repositories
		SET name = $2, full_name = $3, url = $4, primary_language = $5,
		    size_bytes = $6, default_branch = $7, last_commit_date = $8,
		    is_private = $9, is_fork = $10, language_breakdown = $11,
		    config_files = $12, updated_at = $13
		WHERE id = $1
	`

	result, err := s.db.Exec(query,
		repo.ID, repo.Name, repo.FullName, repo.URL, repo.PrimaryLanguage,
		repo.SizeBytes, repo.DefaultBranch, repo.LastCommitDate, repo.IsPrivate,
		repo.IsFork, repo.LanguageBreakdown, repo.ConfigFiles, repo.UpdatedAt,
	)

	if err != nil {
		log.Printf("❌ [RepoStore] Failed to update repository: %v", err)
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrUserNotFound
	}

	log.Printf("✅ [RepoStore] Repository updated: %s", repo.FullName)
	return nil
}

func (s *DBRepositoryStore) Delete(id string) error {
	log.Printf("🔵 [RepoStore] Deleting repository: %s", id)

	repoUUID, err := uuid.Parse(id)
	if err != nil {
		return err
	}

	query := `DELETE FROM user_repositories WHERE id = $1`

	result, err := s.db.Exec(query, repoUUID)
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to delete repository: %v", err)
		return err
	}

	rows, _ := result.RowsAffected()
	log.Printf("✅ [RepoStore] Repository deleted: %d rows affected", rows)
	return nil
}

func (s *DBRepositoryStore) UpdateMetrics(id string, debt float64, coverage float64, complexity float64, critical, high, medium, low int) error {
	repoUUID, err := uuid.Parse(id)
	if err != nil {
		return err
	}

	query := `
		UPDATE user_repositories
		SET latest_total_technical_debt_hours = $2,
			latest_test_coverage_percentage = $3,
			latest_complexity_score = $4,
			latest_critical_issues_count = $5,
			latest_high_issues_count = $6,
			latest_medium_issues_count = $7,
			latest_low_issues_count = $8,
			updated_at = NOW()
		WHERE id = $1
	`

	_, err = s.db.Exec(query, repoUUID, debt, coverage, complexity, critical, high, medium, low)
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to update metrics: %v", err)
		return err
	}

	return nil
}

func (s *DBRepositoryStore) GetByID(id string) (*models.UserRepository, error) {
	log.Printf("🔍 [RepoStore] Getting repository by ID: %s", id)

	repoUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, user_id, user_config_id, name, full_name, url, platform_type,
		       primary_language, size_bytes, default_branch, last_commit_date,
		       is_private, is_fork, language_breakdown, last_analysis_run_id,
		       analysis_enabled, last_analysis_status, last_analysis_at,
		       last_analysis_duration_seconds, created_at, updated_at
		FROM user_repositories
		WHERE id = $1
	`

	repo := &models.UserRepository{}
	err = s.db.QueryRow(query, repoUUID).Scan(
		&repo.ID, &repo.UserID, &repo.UserConfigID, &repo.Name, &repo.FullName,
		&repo.URL, &repo.PlatformType, &repo.PrimaryLanguage, &repo.SizeBytes,
		&repo.DefaultBranch, &repo.LastCommitDate, &repo.IsPrivate, &repo.IsFork,
		&repo.LanguageBreakdown, &repo.LastAnalysisRunID, &repo.AnalysisEnabled,
		&repo.LastAnalysisStatus, &repo.LastAnalysisAt, &repo.LastAnalysisDurationSeconds,
		&repo.CreatedAt, &repo.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}

	if err != nil {
		log.Printf("❌ [RepoStore] Failed to get repository: %v", err)
		return nil, err
	}

	return repo, nil
}

func (s *DBRepositoryStore) GetByFullName(userID, fullName string) (*models.UserRepository, error) {
	log.Printf("🔍 [RepoStore] Getting repository by full name: %s", fullName)

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, user_id, user_config_id, name, full_name, url, platform_type,
		       primary_language, size_bytes, default_branch, last_commit_date,
		       is_private, is_fork, language_breakdown, last_analysis_run_id,
		       analysis_enabled, last_analysis_status, last_analysis_at,
		       last_analysis_duration_seconds, created_at, updated_at
		FROM user_repositories
		WHERE user_id = $1 AND full_name = $2
	`

	repo := &models.UserRepository{}
	err = s.db.QueryRow(query, userUUID, fullName).Scan(
		&repo.ID, &repo.UserID, &repo.UserConfigID, &repo.Name, &repo.FullName,
		&repo.URL, &repo.PlatformType, &repo.PrimaryLanguage, &repo.SizeBytes,
		&repo.DefaultBranch, &repo.LastCommitDate, &repo.IsPrivate, &repo.IsFork,
		&repo.LanguageBreakdown, &repo.LastAnalysisRunID, &repo.AnalysisEnabled,
		&repo.LastAnalysisStatus, &repo.LastAnalysisAt, &repo.LastAnalysisDurationSeconds,
		&repo.CreatedAt, &repo.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}

	if err != nil {
		log.Printf("❌ [RepoStore] Failed to get repository: %v", err)
		return nil, err
	}

	return repo, nil
}

func (s *DBRepositoryStore) ListByUserID(userID string) ([]*models.UserRepository, error) {
	log.Printf("🔍 [RepoStore] Listing repositories for user: %s", userID)

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, user_id, user_config_id, name, full_name, url, platform_type,
		       primary_language, size_bytes, default_branch, last_commit_date,
		       is_private, is_fork, language_breakdown, last_analysis_run_id,
		       analysis_enabled, last_analysis_status, last_analysis_at,
		       last_analysis_duration_seconds, created_at, updated_at
		FROM user_repositories
		WHERE user_id = $1
		ORDER BY full_name ASC
	`

	rows, err := s.db.Query(query, userUUID)
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to list repositories: %v", err)
		return nil, err
	}
	defer rows.Close()

	var repos []*models.UserRepository
	for rows.Next() {
		repo := &models.UserRepository{}
		err := rows.Scan(
			&repo.ID, &repo.UserID, &repo.UserConfigID, &repo.Name, &repo.FullName,
			&repo.URL, &repo.PlatformType, &repo.PrimaryLanguage, &repo.SizeBytes,
			&repo.DefaultBranch, &repo.LastCommitDate, &repo.IsPrivate, &repo.IsFork,
			&repo.LanguageBreakdown, &repo.LastAnalysisRunID, &repo.AnalysisEnabled,
			&repo.LastAnalysisStatus, &repo.LastAnalysisAt, &repo.LastAnalysisDurationSeconds,
			&repo.CreatedAt, &repo.UpdatedAt,
		)
		if err != nil {
			log.Printf("⚠️  [RepoStore] Error scanning repository: %v", err)
			continue
		}
		repos = append(repos, repo)
	}

	log.Printf("✅ [RepoStore] Found %d repositories", len(repos))
	return repos, nil
}

func (s *DBRepositoryStore) ListByConfigID(configID string) ([]*models.UserRepository, error) {
	log.Printf("🔍 [RepoStore] Listing repositories for config: %s", configID)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, user_id, user_config_id, name, full_name, url, platform_type,
		       primary_language, size_bytes, default_branch, last_commit_date,
		       is_private, is_fork, language_breakdown, last_analysis_run_id,
		       analysis_enabled, last_analysis_status, last_analysis_at,
		       last_analysis_duration_seconds, created_at, updated_at
		FROM user_repositories
		WHERE user_config_id = $1
		ORDER BY full_name ASC
	`

	rows, err := s.db.Query(query, configUUID)
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to list repositories: %v", err)
		return nil, err
	}
	defer rows.Close()

	var repos []*models.UserRepository
	for rows.Next() {
		repo := &models.UserRepository{}
		err := rows.Scan(
			&repo.ID, &repo.UserID, &repo.UserConfigID, &repo.Name, &repo.FullName,
			&repo.URL, &repo.PlatformType, &repo.PrimaryLanguage, &repo.SizeBytes,
			&repo.DefaultBranch, &repo.LastCommitDate, &repo.IsPrivate, &repo.IsFork,
			&repo.LanguageBreakdown, &repo.LastAnalysisRunID, &repo.AnalysisEnabled,
			&repo.LastAnalysisStatus, &repo.LastAnalysisAt, &repo.LastAnalysisDurationSeconds,
			&repo.CreatedAt, &repo.UpdatedAt,
		)
		if err != nil {
			log.Printf("⚠️  [RepoStore] Error scanning repository: %v", err)
			continue
		}
		repos = append(repos, repo)
	}

	log.Printf("✅ [RepoStore] Found %d repositories", len(repos))
	return repos, nil
}

func (s *DBRepositoryStore) UpsertRepository(repo *models.UserRepository) error {
	existing, err := s.GetByFullName(repo.UserID.String(), repo.FullName)

	if err == ErrUserNotFound {
		return s.Create(repo)
	} else if err != nil {
		return err
	}

	repo.ID = existing.ID
	repo.CreatedAt = existing.CreatedAt
	return s.Update(repo)
}

func (s *DBRepositoryStore) MarkAsInaccessible(id string) error {
	log.Printf("⚠️  [RepoStore] Marking repository as inaccessible: %s", id)

	repoUUID, err := uuid.Parse(id)
	if err != nil {
		return err
	}

	query := `
		UPDATE user_repositories
		SET analysis_enabled = FALSE,
		    last_analysis_status = 'inaccessible',
		    updated_at = $2
		WHERE id = $1
	`

	_, err = s.db.Exec(query, repoUUID, time.Now())
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to mark repository as inaccessible: %v", err)
		return err
	}

	log.Printf("✅ [RepoStore] Repository marked as inaccessible")
	return nil
}

func (s *DBRepositoryStore) GetUserRepositoriesPaginated(ctx interface{}, userID string, page, pageSize int) ([]*models.UserRepository, int64, error) {
	log.Printf("🔵 [RepoStore] Fetching repositories for user %s (page %d, size %d)", userID, page, pageSize)

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	countQuery := `SELECT COUNT(*) FROM user_repositories WHERE user_id = $1`
	err = s.db.QueryRow(countQuery, userUUID).Scan(&total)
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to count repositories: %v", err)
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	query := `
		SELECT 
			id, user_id, user_config_id, name, full_name, url, platform_type,
			primary_language, size_bytes, default_branch, last_commit_date,
			is_private, is_fork, language_breakdown, analysis_enabled,
			created_at, updated_at
		FROM user_repositories
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.Query(query, userUUID, pageSize, offset)
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to fetch repositories: %v", err)
		return nil, 0, err
	}
	defer rows.Close()

	var repos []*models.UserRepository
	for rows.Next() {
		repo := &models.UserRepository{}
		err := rows.Scan(
			&repo.ID,
			&repo.UserID,
			&repo.UserConfigID,
			&repo.Name,
			&repo.FullName,
			&repo.URL,
			&repo.PlatformType,
			&repo.PrimaryLanguage,
			&repo.SizeBytes,
			&repo.DefaultBranch,
			&repo.LastCommitDate,
			&repo.IsPrivate,
			&repo.IsFork,
			&repo.LanguageBreakdown,
			&repo.AnalysisEnabled,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		)
		if err != nil {
			log.Printf("⚠️  [RepoStore] Error scanning repository: %v", err)
			continue
		}
		repos = append(repos, repo)
	}

	log.Printf("✅ [RepoStore] Fetched %d repositories (total: %d)", len(repos), total)
	return repos, total, nil
}

func (s *DBRepositoryStore) GetRepositoryStats(ctx interface{}, userID string) (*RepositoryStats, error) {
	log.Printf("🔵 [RepoStore] Fetching repository statistics for user %s", userID)

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}

	stats := &RepositoryStats{
		PlatformCount: make(map[string]int),
	}

	query := `
		SELECT 
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN is_private = true THEN 1 ELSE 0 END), 0) as private_count,
			COALESCE(SUM(size_bytes), 0) as total_size
		FROM user_repositories
		WHERE user_id = $1
	`
	err = s.db.QueryRow(query, userUUID).Scan(&stats.TotalRepositories, &stats.PrivateRepos, &stats.TotalSizeBytes)
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to get repository counts: %v", err)
		return nil, err
	}

	today := time.Now().Format("2006-01-02")
	activeQuery := `
		SELECT COUNT(*)
		FROM user_repositories
		WHERE user_id = $1 AND DATE(last_commit_date) >= $2
	`
	err = s.db.QueryRow(activeQuery, userUUID, today).Scan(&stats.ActiveToday)
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to get active today count: %v", err)
		return nil, err
	}

	recentQuery := `
		SELECT id, name, full_name, url, platform_type, primary_language, size_bytes, updated_at
		FROM user_repositories
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT 4
	`
	rows, err := s.db.Query(recentQuery, userUUID)
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to get recent repositories: %v", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var repo RecentRepository
		err := rows.Scan(&repo.ID, &repo.Name, &repo.FullName, &repo.URL, &repo.PlatformType, &repo.PrimaryLanguage, &repo.SizeBytes, &repo.UpdatedAt)
		if err != nil {
			log.Printf("⚠️  [RepoStore] Error scanning recent repository: %v", err)
			continue
		}
		stats.RecentRepos = append(stats.RecentRepos, repo)
	}

	langQuery := `
		SELECT primary_language, COUNT(*) as count
		FROM user_repositories
		WHERE user_id = $1 AND primary_language IS NOT NULL AND primary_language != ''
		GROUP BY primary_language
		ORDER BY count DESC
		LIMIT 5
	`
	rows, err = s.db.Query(langQuery, userUUID)
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to get language distribution: %v", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var lang LanguageDistribution
		err := rows.Scan(&lang.Language, &lang.Count)
		if err != nil {
			log.Printf("⚠️  [RepoStore] Error scanning language: %v", err)
			continue
		}
		stats.TopLanguages = append(stats.TopLanguages, lang)
	}

	platformQuery := `
		SELECT platform_type, COUNT(*) as count
		FROM user_repositories
		WHERE user_id = $1
		GROUP BY platform_type
	`
	rows, err = s.db.Query(platformQuery, userUUID)
	if err != nil {
		log.Printf("❌ [RepoStore] Failed to get platform distribution: %v", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var platform string
		var count int
		err := rows.Scan(&platform, &count)
		if err != nil {
			log.Printf("⚠️  [RepoStore] Error scanning platform: %v", err)
			continue
		}
		stats.PlatformCount[platform] = count
	}

	log.Printf("✅ [RepoStore] Repository statistics fetched successfully")
	return stats, nil
}
