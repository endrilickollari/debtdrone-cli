package analysis_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/endrilickollari/debtdrone-cli/internal/analysis"
	"github.com/endrilickollari/debtdrone-cli/internal/analysis/analyzers"
	"github.com/endrilickollari/debtdrone-cli/internal/git"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_Golden(t *testing.T) {
	// 1. Setup Analyzers
	// Pass nil for store as we are only testing analysis logic, not persistence
	complexityAnalyzer := analyzers.NewComplexityAnalyzer(nil)
	lineCounter := analyzers.NewLineCounter()

	analyzersList := []analysis.Analyzer{
		complexityAnalyzer,
		lineCounter,
	}

	// 2. Define Test Cases
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

			// 3. Create Local Repository mock
			repo := &git.Repository{
				FS:   osfs.New(absPath),
				Path: absPath,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Mock context values required by Analyzers
			// These are needed because analyzers check for them
			// Parse UUIDs as the analyzers expect uuid.UUID type, not string
			runID, _ := uuid.Parse("00000000-0000-0000-0000-000000000000")
			repoID, _ := uuid.Parse("00000000-0000-0000-0000-000000000000")
			userID, _ := uuid.Parse("00000000-0000-0000-0000-000000000000")

			ctx = context.WithValue(ctx, "analysisRunID", runID)
			ctx = context.WithValue(ctx, "repositoryID", repoID)
			ctx = context.WithValue(ctx, "userID", userID)

			// 4. Run Analysis
			finalReport := map[string]interface{}{}
			issues := []interface{}{}

			for _, analyzer := range analyzersList {
				result, err := analyzer.Analyze(ctx, repo)
				require.NoError(t, err, "Analyzer %s failed", analyzer.Name())

				if result != nil {
					// Append issues
					for _, issue := range result.Issues {
						issues = append(issues, issue)
					}
					// Merge metrics
					for k, v := range result.Metrics {
						finalReport[k] = v
					}
				}
			}
			finalReport["issues"] = issues

			// 5. Sanitize Report
			// We iterate through the map to convert it to JSON and back to interface{} to normalize types
			// Then we walk through it to sanitize

			data, err := json.Marshal(finalReport)
			require.NoError(t, err)

			var parsedReport interface{}
			err = json.Unmarshal(data, &parsedReport)
			require.NoError(t, err)

			sanitized := sanitizeValues(parsedReport)

			// 6. Snapshot Comparison
			snapshotData, err := json.MarshalIndent(sanitized, "", "  ")
			require.NoError(t, err)

			goldenFile := filepath.Join("analyzers/testdata", tt.name+".golden.json")

			if os.Getenv("UPDATE_GOLDEN") == "true" {
				err = os.WriteFile(goldenFile, snapshotData, 0644)
				require.NoError(t, err)
			}

			// Read or Create if missing (for first run convenience)
			expectedData, err := os.ReadFile(goldenFile)
			if os.IsNotExist(err) {
				// Write it the first time to establish baseline.
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
			// Remove UUIDs and Timestamps
			if k == "id" || k == "analysis_run_id" || k == "repository_id" || k == "user_id" || k == "created_at" || k == "updated_at" {
				continue // Skip dynamic IDs
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

// mockPanicAnalyzer simulates a disastrous panic during code analysis
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
	// 1. Setup Analyzers
	safeAnalyzer := &mockPanicAnalyzer{name: "safe_analyzer", panics: false}
	panicAnalyzer := &mockPanicAnalyzer{name: "panic_analyzer", panics: true}

	analyzersList := []analysis.Analyzer{
		safeAnalyzer,
		panicAnalyzer,
	}

	// 2. Mock repository
	absPath, err := filepath.Abs("analyzers/testdata/go/clean")
	require.NoError(t, err)

	repo := &git.Repository{
		FS:   osfs.New(absPath),
		Path: absPath,
	}

	ctx := context.Background()

	// 3. Run Analysis loop mirroring Engine's parallel loop
	var mu sync.Mutex
	var issues []interface{}
	var analyzerErrors []error

	var wg sync.WaitGroup

	for _, analyzer := range analyzersList {
		analyzer := analyzer // capture
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			// Simulate the Engine's exact wrapper call mapping
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

	// 4. Assertions
	// The main test loop hasn't crashed, proving Engine continues successfully.
	assert.Len(t, analyzerErrors, 1, "Expected exactly one error from the panicking analyzer")
	assert.Contains(t, analyzerErrors[0].Error(), "panic in analyzer panic_analyzer")
	assert.Contains(t, analyzerErrors[0].Error(), "simulated disaster code")

	// Other non-panicking analyzers still ran perfectly fine.
	assert.Len(t, issues, 1, "Expected the safe analyzer to still produce its issue")
}
