package store

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

type SessionStoreInterface interface {
	Create(session *models.UserSession) error
	GetByToken(token string) (*models.UserSession, error)
	GetActiveSessionsByUserID(userID string) ([]*models.UserSession, error)
	UpdateLastActivity(sessionID string) error
	Revoke(sessionID string) error
	RevokeByToken(token string) error
	RevokeAllUserSessions(userID string) error
	DeleteExpired() error
}

type DBSessionStore struct {
	db *sql.DB
}

func NewDBSessionStore(db *sql.DB) *DBSessionStore {
	log.Println("📦 Initializing database-backed session store")
	return &DBSessionStore{db: db}
}

func (s *DBSessionStore) Create(session *models.UserSession) error {
	log.Printf("🔵 [SessionStore] Creating session: user_id=%s", session.UserID)

	session.ID = uuid.New()
	session.CreatedAt = time.Now()
	session.LastActivityAt = time.Now()
	session.IsActive = true

	query := `
		INSERT INTO user_sessions (
			id, user_id, session_token, ip_address, user_agent,
			created_at, expires_at, last_activity_at, is_active, device_info
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`

	_, err := s.db.Exec(query,
		session.ID, session.UserID, session.SessionToken, session.IPAddress,
		session.UserAgent, session.CreatedAt, session.ExpiresAt,
		session.LastActivityAt, session.IsActive, session.DeviceInfo,
	)

	if err != nil {
		log.Printf("❌ [SessionStore] Failed to create session: %v", err)
		return err
	}

	log.Printf("✅ [SessionStore] Session created: id=%s", session.ID)
	return nil
}

func (s *DBSessionStore) GetByToken(token string) (*models.UserSession, error) {
	log.Printf("🔍 [SessionStore] Looking up session by token")

	query := `
		SELECT id, user_id, session_token, ip_address, user_agent,
		       created_at, expires_at, last_activity_at, is_active, device_info
		FROM user_sessions
		WHERE session_token = $1 AND is_active = TRUE
	`

	session := &models.UserSession{}
	err := s.db.QueryRow(query, token).Scan(
		&session.ID, &session.UserID, &session.SessionToken, &session.IPAddress,
		&session.UserAgent, &session.CreatedAt, &session.ExpiresAt,
		&session.LastActivityAt, &session.IsActive, &session.DeviceInfo,
	)

	if err == sql.ErrNoRows {
		log.Printf("⚠️  [SessionStore] Session not found or inactive")
		return nil, ErrUserNotFound
	}

	if err != nil {
		log.Printf("❌ [SessionStore] Failed to get session: %v", err)
		return nil, err
	}

	log.Printf("✅ [SessionStore] Session found: id=%s", session.ID)
	return session, nil
}

func (s *DBSessionStore) GetActiveSessionsByUserID(userID string) ([]*models.UserSession, error) {
	log.Printf("🔍 [SessionStore] Getting active sessions for user: %s", userID)

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, user_id, session_token, ip_address, user_agent,
		       created_at, expires_at, last_activity_at, is_active, device_info
		FROM user_sessions
		WHERE user_id = $1 AND is_active = TRUE AND expires_at > NOW()
		ORDER BY last_activity_at DESC
	`

	rows, err := s.db.Query(query, userUUID)
	if err != nil {
		log.Printf("❌ [SessionStore] Failed to get sessions: %v", err)
		return nil, err
	}
	defer rows.Close()

	var sessions []*models.UserSession
	for rows.Next() {
		session := &models.UserSession{}
		err := rows.Scan(
			&session.ID, &session.UserID, &session.SessionToken, &session.IPAddress,
			&session.UserAgent, &session.CreatedAt, &session.ExpiresAt,
			&session.LastActivityAt, &session.IsActive, &session.DeviceInfo,
		)
		if err != nil {
			log.Printf("⚠️  [SessionStore] Error scanning session: %v", err)
			continue
		}
		sessions = append(sessions, session)
	}

	log.Printf("✅ [SessionStore] Found %d active sessions", len(sessions))
	return sessions, nil
}

func (s *DBSessionStore) UpdateLastActivity(sessionID string) error {
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return err
	}

	query := `
		UPDATE user_sessions
		SET last_activity_at = $1
		WHERE id = $2 AND is_active = TRUE
	`

	_, err = s.db.Exec(query, time.Now(), sessionUUID)
	if err != nil {
		log.Printf("❌ [SessionStore] Failed to update last activity: %v", err)
		return err
	}

	return nil
}

func (s *DBSessionStore) Revoke(sessionID string) error {
	log.Printf("🔵 [SessionStore] Revoking session: %s", sessionID)

	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return err
	}

	query := `
		UPDATE user_sessions
		SET is_active = FALSE
		WHERE id = $1
	`

	result, err := s.db.Exec(query, sessionUUID)
	if err != nil {
		log.Printf("❌ [SessionStore] Failed to revoke session: %v", err)
		return err
	}

	rows, _ := result.RowsAffected()
	log.Printf("✅ [SessionStore] Session revoked: %d rows affected", rows)
	return nil
}

func (s *DBSessionStore) RevokeByToken(token string) error {
	log.Printf("🔵 [SessionStore] Revoking session by token")

	query := `
		UPDATE user_sessions
		SET is_active = FALSE
		WHERE session_token = $1
	`

	result, err := s.db.Exec(query, token)
	if err != nil {
		log.Printf("❌ [SessionStore] Failed to revoke session: %v", err)
		return err
	}

	rows, _ := result.RowsAffected()
	log.Printf("✅ [SessionStore] Session revoked: %d rows affected", rows)
	return nil
}

func (s *DBSessionStore) RevokeAllUserSessions(userID string) error {
	log.Printf("🔵 [SessionStore] Revoking all sessions for user: %s", userID)

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return err
	}

	query := `
		UPDATE user_sessions
		SET is_active = FALSE
		WHERE user_id = $1 AND is_active = TRUE
	`

	result, err := s.db.Exec(query, userUUID)
	if err != nil {
		log.Printf("❌ [SessionStore] Failed to revoke sessions: %v", err)
		return err
	}

	rows, _ := result.RowsAffected()
	log.Printf("✅ [SessionStore] %d sessions revoked for user", rows)
	return nil
}

func (s *DBSessionStore) DeleteExpired() error {
	log.Println("🔵 [SessionStore] Deleting expired sessions")

	query := `
		DELETE FROM user_sessions
		WHERE expires_at < NOW()
	`

	result, err := s.db.Exec(query)
	if err != nil {
		log.Printf("❌ [SessionStore] Failed to delete expired sessions: %v", err)
		return err
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		log.Printf("✅ [SessionStore] Deleted %d expired sessions", rows)
	}
	return nil
}

func ParseDeviceInfo(userAgent string) *string {
	deviceInfo := map[string]string{
		"user_agent": userAgent,
	}

	jsonData, err := json.Marshal(deviceInfo)
	if err != nil {
		return nil
	}

	result := string(jsonData)
	return &result
}
