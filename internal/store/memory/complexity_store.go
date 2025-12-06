package memory

import (
	"context"

	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/endrilickollari/debtdrone-cli/internal/store"
	"github.com/google/uuid"
)

type InMemoryComplexityStore struct {
	Metrics []models.ComplexityMetric
}

func NewInMemoryComplexityStore() *InMemoryComplexityStore {
	return &InMemoryComplexityStore{
		Metrics: []models.ComplexityMetric{},
	}
}

func (s *InMemoryComplexityStore) BatchCreate(ctx context.Context, metrics []models.ComplexityMetric) error {
	s.Metrics = append(s.Metrics, metrics...)
	return nil
}
func (s *InMemoryComplexityStore) GetByAnalysisRun(ctx context.Context, analysisRunID uuid.UUID) ([]models.ComplexityMetric, error) {
	var results []models.ComplexityMetric
	for _, m := range s.Metrics {
		if m.AnalysisRunID == analysisRunID {
			results = append(results, m)
		}
	}
	return results, nil
}

func (s *InMemoryComplexityStore) GetByRepository(ctx context.Context, repositoryID uuid.UUID, filters store.ComplexityFilters) ([]models.ComplexityMetric, error) {
	var results []models.ComplexityMetric
	for _, m := range s.Metrics {
		if m.RepositoryID == repositoryID {
			results = append(results, m)
		}
	}
	return results, nil
}

func (s *InMemoryComplexityStore) GetFileSummary(ctx context.Context, analysisRunID uuid.UUID, filePath string) (*models.FileComplexitySummary, error) {
	return nil, nil
}
func (s *InMemoryComplexityStore) GetRepositorySummary(ctx context.Context, analysisRunID uuid.UUID) (*models.RepositoryComplexitySummary, error) {
	return nil, nil
}
