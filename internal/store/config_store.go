package store

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type ConfigStoreInterface interface {
	Create(config *models.UserConfiguration) error
	Update(config *models.UserConfiguration) error
	GetByID(id string) (*models.UserConfiguration, error)
	ListByUserID(userID string) ([]*models.UserConfiguration, error)
	UpdateLastSync(id string, lastSync, nextSync time.Time) error
	Delete(id string) error
}

type DBConfigStore struct {
	db *sql.DB
}

func NewDBConfigStore(db *sql.DB) *DBConfigStore {
	log.Println("📦 Initializing database-backed config store")
	return &DBConfigStore{db: db}
}

func (s *DBConfigStore) Create(config *models.UserConfiguration) error {
	log.Printf("🔵 [ConfigStore] Creating configuration: %s/%s", config.OrganizationName, config.PlatformType)

	config.ID = uuid.New()
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	query := `
		INSERT INTO user_configurations (
			id, user_id, organization_name, organization_url, platform_type,
			access_token_encrypted, auto_sync_enabled, sync_frequency_minutes,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`

	_, err := s.db.Exec(query,
		config.ID, config.UserID, config.OrganizationName, config.OrganizationURL,
		config.PlatformType, config.AccessTokenEncrypted, config.AutoSyncEnabled,
		config.SyncFrequencyMinutes, config.CreatedAt, config.UpdatedAt,
	)

	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			log.Printf("⚠️  [ConfigStore] Configuration already exists")
			return ErrUserAlreadyExists
		}
		log.Printf("❌ [ConfigStore] Failed to create configuration: %v", err)
		return err
	}

	log.Printf("✅ [ConfigStore] Configuration created: id=%s", config.ID)
	return nil
}

func (s *DBConfigStore) Update(config *models.UserConfiguration) error {
	log.Printf("🔵 [ConfigStore] Updating configuration: %s", config.ID)

	config.UpdatedAt = time.Now()

	query := `
		UPDATE user_configurations
		SET organization_name = $2, organization_url = $3, access_token_encrypted = $4,
		    auto_sync_enabled = $5, sync_frequency_minutes = $6, updated_at = $7
		WHERE id = $1
	`

	result, err := s.db.Exec(query,
		config.ID, config.OrganizationName, config.OrganizationURL,
		config.AccessTokenEncrypted, config.AutoSyncEnabled,
		config.SyncFrequencyMinutes, config.UpdatedAt,
	)

	if err != nil {
		log.Printf("❌ [ConfigStore] Failed to update configuration: %v", err)
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrUserNotFound
	}

	log.Printf("✅ [ConfigStore] Configuration updated")
	return nil
}

func (s *DBConfigStore) GetByID(id string) (*models.UserConfiguration, error) {
	log.Printf("🔍 [ConfigStore] Getting configuration by ID: %s", id)

	configUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, user_id, organization_name, organization_url, platform_type,
		       access_token_encrypted, auto_sync_enabled, sync_frequency_minutes,
		       last_sync_at, next_sync_at, created_at, updated_at
		FROM user_configurations
		WHERE id = $1
	`

	config := &models.UserConfiguration{}
	err = s.db.QueryRow(query, configUUID).Scan(
		&config.ID, &config.UserID, &config.OrganizationName, &config.OrganizationURL,
		&config.PlatformType, &config.AccessTokenEncrypted, &config.AutoSyncEnabled,
		&config.SyncFrequencyMinutes, &config.LastSyncAt, &config.NextSyncAt,
		&config.CreatedAt, &config.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}

	if err != nil {
		log.Printf("❌ [ConfigStore] Failed to get configuration: %v", err)
		return nil, err
	}

	return config, nil
}

func (s *DBConfigStore) ListByUserID(userID string) ([]*models.UserConfiguration, error) {
	log.Printf("🔍 [ConfigStore] Listing configurations for user: %s", userID)

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, user_id, organization_name, organization_url, platform_type,
		       access_token_encrypted, auto_sync_enabled, sync_frequency_minutes,
		       last_sync_at, next_sync_at, created_at, updated_at
		FROM user_configurations
		WHERE user_id = $1
		ORDER BY organization_name ASC
	`

	rows, err := s.db.Query(query, userUUID)
	if err != nil {
		log.Printf("❌ [ConfigStore] Failed to list configurations: %v", err)
		return nil, err
	}
	defer rows.Close()

	var configs []*models.UserConfiguration
	for rows.Next() {
		config := &models.UserConfiguration{}
		err := rows.Scan(
			&config.ID, &config.UserID, &config.OrganizationName, &config.OrganizationURL,
			&config.PlatformType, &config.AccessTokenEncrypted, &config.AutoSyncEnabled,
			&config.SyncFrequencyMinutes, &config.LastSyncAt, &config.NextSyncAt,
			&config.CreatedAt, &config.UpdatedAt,
		)
		if err != nil {
			log.Printf("⚠️  [ConfigStore] Error scanning configuration: %v", err)
			continue
		}
		configs = append(configs, config)
	}

	log.Printf("✅ [ConfigStore] Found %d configurations", len(configs))
	return configs, nil
}

func (s *DBConfigStore) UpdateLastSync(id string, lastSync, nextSync time.Time) error {
	log.Printf("🔵 [ConfigStore] Updating sync timestamps for config: %s", id)

	configUUID, err := uuid.Parse(id)
	if err != nil {
		return err
	}

	query := `
		UPDATE user_configurations
		SET last_sync_at = $2, next_sync_at = $3, updated_at = $4
		WHERE id = $1
	`

	_, err = s.db.Exec(query, configUUID, lastSync, nextSync, time.Now())
	if err != nil {
		log.Printf("❌ [ConfigStore] Failed to update sync timestamps: %v", err)
		return err
	}

	log.Printf("✅ [ConfigStore] Sync timestamps updated")
	return nil
}

func (s *DBConfigStore) Delete(id string) error {
	log.Printf("🗑️  [ConfigStore] Deleting configuration: %s", id)

	configUUID, err := uuid.Parse(id)
	if err != nil {
		return err
	}

	query := `DELETE FROM user_configurations WHERE id = $1`

	result, err := s.db.Exec(query, configUUID)
	if err != nil {
		log.Printf("❌ [ConfigStore] Failed to delete configuration: %v", err)
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("⚠️  [ConfigStore] Configuration not found: %s", id)
		return fmt.Errorf("configuration not found")
	}

	log.Printf("✅ [ConfigStore] Configuration deleted: %s", id)
	return nil
}
