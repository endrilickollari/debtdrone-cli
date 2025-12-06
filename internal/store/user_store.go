package store

import (
	"errors"
	"sync"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrAccountLocked     = errors.New("account is temporarily locked")
)

type UserStoreInterface interface {
	Create(user *models.User) error
	GetByEmail(email string) (*models.User, error)
	GetByID(id string) (*models.User, error)
	GetByProviderID(provider, providerID string) (*models.User, error)
	Update(user *models.User) error
	IncrementLoginAttempts(email string) error
	ResetLoginAttempts(email string) error
	LockAccount(email string, duration time.Duration) error
	IsAccountLocked(email string) (bool, error)
	UpdateLastLogin(userID string) error
}

type UserStore struct {
	mu    sync.RWMutex
	users map[uuid.UUID]*models.User
	email map[string]uuid.UUID
}

func NewUserStore() *UserStore {
	return &UserStore{
		users: make(map[uuid.UUID]*models.User),
		email: make(map[string]uuid.UUID),
	}
}

func (s *UserStore) Create(user *models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.email[user.Email]; exists {
		return ErrUserAlreadyExists
	}

	user.ID = uuid.New()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	user.IsActive = true

	s.users[user.ID] = user
	s.email[user.Email] = user.ID

	return nil
}

func (s *UserStore) GetByEmail(email string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userID, exists := s.email[email]
	if !exists {
		return nil, ErrUserNotFound
	}

	user := s.users[userID]
	return user, nil
}

func (s *UserStore) GetByID(id string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userID, err := uuid.Parse(id)
	if err != nil {
		return nil, ErrUserNotFound
	}

	user, exists := s.users[userID]
	if !exists {
		return nil, ErrUserNotFound
	}

	return user, nil
}

func (s *UserStore) GetByProviderID(provider, providerID string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if user.ProviderName != nil && *user.ProviderName == provider &&
			user.ProviderID != nil && *user.ProviderID == providerID {
			return user, nil
		}
	}

	return nil, ErrUserNotFound
}

func (s *UserStore) Update(user *models.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.ID]; !exists {
		return ErrUserNotFound
	}

	user.UpdatedAt = time.Now()
	s.users[user.ID] = user

	return nil
}

func (s *UserStore) IncrementLoginAttempts(email string) error {
	user, err := s.GetByEmail(email)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user.FailedLoginAttempts++
	user.UpdatedAt = time.Now()

	return nil
}

func (s *UserStore) ResetLoginAttempts(email string) error {
	user, err := s.GetByEmail(email)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	user.UpdatedAt = time.Now()

	return nil
}

func (s *UserStore) LockAccount(email string, duration time.Duration) error {
	user, err := s.GetByEmail(email)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	lockUntil := time.Now().Add(duration)
	user.LockedUntil = &lockUntil
	user.UpdatedAt = time.Now()

	return nil
}

func (s *UserStore) IsAccountLocked(email string) (bool, error) {
	user, err := s.GetByEmail(email)
	if err != nil {
		return false, err
	}

	if user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
		return true, nil
	}

	return false, nil
}

func (s *UserStore) UpdateLastLogin(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	uuid, err := uuid.Parse(userID)
	if err != nil {
		return ErrUserNotFound
	}

	user, exists := s.users[uuid]
	if !exists {
		return ErrUserNotFound
	}

	now := time.Now()
	user.LastLoginAt = &now
	user.UpdatedAt = now

	return nil
}
