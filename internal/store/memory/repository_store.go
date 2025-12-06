package memory

import (
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/endrilickollari/debtdrone-cli/internal/store"
)

type InMemoryRepositoryStore struct {
	Repos []models.UserRepository
}

func NewInMemoryRepositoryStore() *InMemoryRepositoryStore {
	return &InMemoryRepositoryStore{Repos: []models.UserRepository{}}
}

func (s *InMemoryRepositoryStore) Create(repo *models.UserRepository) error {
	s.Repos = append(s.Repos, *repo)
	return nil
}

func (s *InMemoryRepositoryStore) Update(repo *models.UserRepository) error {
	for i, existing := range s.Repos {
		if existing.ID == repo.ID {
			s.Repos[i] = *repo
			return nil
		}
	}
	return nil
}

func (s *InMemoryRepositoryStore) Delete(id string) error {
	return nil
}

func (s *InMemoryRepositoryStore) GetByID(id string) (*models.UserRepository, error) {
	for _, repo := range s.Repos {
		if repo.ID.String() == id {
			return &repo, nil
		}
	}
	return nil, nil
}

func (s *InMemoryRepositoryStore) GetByFullName(userID, fullName string) (*models.UserRepository, error) {
	return nil, nil
}

func (s *InMemoryRepositoryStore) ListByUserID(userID string) ([]*models.UserRepository, error) {
	return []*models.UserRepository{}, nil
}

func (s *InMemoryRepositoryStore) ListByConfigID(configID string) ([]*models.UserRepository, error) {
	return []*models.UserRepository{}, nil
}

func (s *InMemoryRepositoryStore) UpsertRepository(repo *models.UserRepository) error {
	return s.Create(repo)
}

func (s *InMemoryRepositoryStore) MarkAsInaccessible(id string) error {
	return nil
}

func (s *InMemoryRepositoryStore) GetUserRepositoriesPaginated(ctx interface{}, userID string, page, pageSize int) ([]*models.UserRepository, int64, error) {
	return []*models.UserRepository{}, 0, nil
}

func (s *InMemoryRepositoryStore) GetRepositoryStats(ctx interface{}, userID string) (*store.RepositoryStats, error) {
	return &store.RepositoryStats{}, nil
}
