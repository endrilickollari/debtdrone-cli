package memory

import (
	"context"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/google/uuid"
)

type InMemoryRunStore struct {
	Runs []models.AnalysisRun
}

func NewInMemoryRunStore() *InMemoryRunStore {
	return &InMemoryRunStore{Runs: []models.AnalysisRun{}}
}

func (s *InMemoryRunStore) Create(run *models.AnalysisRun) error {
	s.Runs = append(s.Runs, *run)
	return nil
}

func (s *InMemoryRunStore) Get(id string) (*models.AnalysisRun, error) {
	for _, run := range s.Runs {
		if run.ID.String() == id {
			return &run, nil
		}
	}
	return nil, nil
}

func (s *InMemoryRunStore) List(limit, offset int) ([]models.AnalysisRun, error) {
	if offset >= len(s.Runs) {
		return []models.AnalysisRun{}, nil
	}
	end := offset + limit
	if end > len(s.Runs) {
		end = len(s.Runs)
	}
	return s.Runs[offset:end], nil
}

func (s *InMemoryRunStore) Update(run *models.AnalysisRun) error {
	for i, existing := range s.Runs {
		if existing.ID == run.ID {
			s.Runs[i] = *run
			return nil
		}
	}
	return nil
}

func (s *InMemoryRunStore) UpdateStatus(ctx context.Context, runID uuid.UUID, status string, results map[string]interface{}) error {
	for i, run := range s.Runs {
		if run.ID == runID {
			s.Runs[i].Status = status
			return nil
		}
	}
	return nil
}
