package store

import (
	"time"

	"github.com/google/uuid"
)

type PendingOAuthConfig struct {
	UserID           uuid.UUID
	OrganizationName string
	PlatformType     string
	CreatedAt        time.Time
}
