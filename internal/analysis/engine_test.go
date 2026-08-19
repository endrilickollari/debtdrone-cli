package analysis_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/analysis"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/analysis/analyzers"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_Golden(t *testing.T) {
	complexityAnalyzer := analyzers.NewComplexityAnalyzer()
	lineCounter := analyzers.NewLineCounter()

	analyzersList := []analysis.Analyzer{
		complexityAnalyzer,
		lineCounter,
	}

	tests := []struct {
		name     string
		repoPath string
	}{
		{
			name:     "go_clean",
			repoPath: "analyzers/testdata/go/clean",
		},
		{
			name:     "go_dirty",
			repoPath: "analyzers/testdata/go/dirty",
		},
		{
			name:     "ts_clean",
			repoPath: "analyzers/testdata/ts/clean",
		},
		{
			name:     "ts_dirty",
			repoPath: "analyzers/testdata/ts/dirty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			absPath, err := filepath.Abs(tt.repoPath)
			require.NoError(t, err)

			repo := &git.Repository{
				FS:   osfs.New(absPath),
				Path: absPath,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Analyzers require UUID-typed run, repository, and user context values.
			runID, _ := uuid.Parse("00000000-0000-0000-0000-000000000000")
			repoID, _ := uuid.Parse("00000000-0000-0000-0000-000000000000")
			userID, _ := uuid.Parse("00000000-0000-0000-0000-000000000000")

			ctx = context.WithValue(ctx, "analysisRunID", runID)
			ctx = context.WithValue(ctx, "repositoryID", repoID)
			ctx = context.WithValue(ctx, "userID", userID)

			finalReport := map[string]interface{}{}
			issues := []interface{}{}

			for _, analyzer := range analyzersList {
				result, err := analyzer.Analyze(ctx, repo)
				require.NoError(t, err, "Analyzer %s failed", analyzer.Name())

				if result != nil {
					for _, issue := range result.Issues {
						issues = append(issues, issue)
					}
					for k, v := range result.Metrics {
						finalReport[k] = v
					}
				}
			}
			finalReport["issues"] = issues

			data, err := json.Marshal(finalReport)
			require.NoError(t, err)

			var parsedReport interface{}
			err = json.Unmarshal(data, &parsedReport)
			require.NoError(t, err)

			sanitized := sanitizeValues(parsedReport)

			snapshotData, err := json.MarshalIndent(sanitized, "", "  ")
			require.NoError(t, err)

			goldenFile := filepath.Join("analyzers/testdata", tt.name+".golden.json")

			if os.Getenv("UPDATE_GOLDEN") == "true" {
				err = os.WriteFile(goldenFile, snapshotData, 0644)
				require.NoError(t, err)
			}

			expectedData, err := os.ReadFile(goldenFile)
			if os.IsNotExist(err) {
				err = os.WriteFile(goldenFile, snapshotData, 0644)
				require.NoError(t, err)
				expectedData = snapshotData
			}
			require.NoError(t, err)

			assert.JSONEq(t, string(expectedData), string(snapshotData), "Snapshot mismatch! Run with UPDATE_GOLDEN=true to update.")
		})
	}
}

// sanitizeValues recursively walks the JSON structure and replaces non-deterministic values
func sanitizeValues(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		m := make(map[string]interface{})
		for k, val := range x {
			if k == "id" || k == "analysis_run_id" || k == "repository_id" || k == "user_id" || k == "created_at" || k == "updated_at" {
				continue
			}
			m[k] = sanitizeValues(val)
		}
		return m
	case []interface{}:
		s := make([]interface{}, len(x))
		for i, val := range x {
			s[i] = sanitizeValues(val)
		}
		return s
	default:
		return x
	}
}

type mockPanicAnalyzer struct {
	name   string
	panics bool
}

func (m *mockPanicAnalyzer) Name() string { return m.name }

func (m *mockPanicAnalyzer) Analyze(ctx context.Context, repo *git.Repository) (*analysis.Result, error) {
	if m.panics {
		panic("simulated disaster code")
	}
	return &analysis.Result{
		Issues: []models.TechnicalDebtIssue{
			{Message: "dummy_issue", Status: "open"},
		},
		Metrics: map[string]interface{}{"dummy_metric": 1},
	}, nil
}

func TestEngine_PanicRecovery(t *testing.T) {
	safeAnalyzer := &mockPanicAnalyzer{name: "safe_analyzer", panics: false}
	panicAnalyzer := &mockPanicAnalyzer{name: "panic_analyzer", panics: true}

	analyzersList := []analysis.Analyzer{
		safeAnalyzer,
		panicAnalyzer,
	}

	absPath, err := filepath.Abs("analyzers/testdata/go/clean")
	require.NoError(t, err)

	repo := &git.Repository{
		FS:   osfs.New(absPath),
		Path: absPath,
	}

	ctx := context.Background()

	var mu sync.Mutex
	var issues []interface{}
	var analyzerErrors []error

	var wg sync.WaitGroup

	for _, analyzer := range analyzersList {
		analyzer := analyzer
		wg.Add(1)
		go func() {
			defer wg.Done()

			result, err := analysis.ExecuteAnalyzerSafeTest(ctx, analyzer, repo)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				analyzerErrors = append(analyzerErrors, err)
			} else if result != nil {
				for _, issue := range result.Issues {
					issues = append(issues, issue)
				}
			}
		}()
	}

	wg.Wait()

	assert.Len(t, analyzerErrors, 1, "Expected exactly one error from the panicking analyzer")
	assert.Contains(t, analyzerErrors[0].Error(), "panic in analyzer panic_analyzer")
	assert.Contains(t, analyzerErrors[0].Error(), "simulated disaster code")

	assert.Len(t, issues, 1, "Expected the safe analyzer to still produce its issue")
}
