package memory

import (
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/endrilickollari/debtdrone-cli/internal/store"
	"github.com/google/uuid"
)

type InMemoryIssueStore struct {
	Issues []models.TechnicalDebtIssue
}

func NewInMemoryIssueStore() *InMemoryIssueStore {
	return &InMemoryIssueStore{Issues: []models.TechnicalDebtIssue{}}
}

func (s *InMemoryIssueStore) Create(issue *models.TechnicalDebtIssue) error {
	s.Issues = append(s.Issues, *issue)
	return nil
}

func (s *InMemoryIssueStore) BatchCreate(issues []models.TechnicalDebtIssue) error {
	s.Issues = append(s.Issues, issues...)
	return nil
}

func (s *InMemoryIssueStore) Get(id string) (*models.TechnicalDebtIssue, error) {
	for _, issue := range s.Issues {
		if issue.ID.String() == id {
			return &issue, nil
		}
	}
	return nil, nil
}

func (s *InMemoryIssueStore) List(limit, offset int) ([]models.TechnicalDebtIssue, error) {
	if offset >= len(s.Issues) {
		return []models.TechnicalDebtIssue{}, nil
	}
	end := offset + limit
	if end > len(s.Issues) {
		end = len(s.Issues)
	}
	return s.Issues[offset:end], nil
}

func (s *InMemoryIssueStore) ListWithFilters(filters store.IssueFilters, limit, offset int) ([]models.TechnicalDebtIssue, int, error) {
	issues, err := s.List(limit, offset)
	return issues, len(s.Issues), err
}

func (s *InMemoryIssueStore) Update(issue *models.TechnicalDebtIssue) error {
	for i, existing := range s.Issues {
		if existing.ID == issue.ID {
			s.Issues[i] = *issue
			return nil
		}
	}
	return nil
}

func (s *InMemoryIssueStore) IssueExists(repositoryID uuid.UUID, filePath string, lineNumber *int, issueType string, toolRuleID *string) (bool, error) {
	return false, nil
}
