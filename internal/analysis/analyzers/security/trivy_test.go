package security

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/scancore"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTrivyExecutor struct {
	runs int
	scan func(context.Context, string) ([]byte, error)
}

func (*fakeTrivyExecutor) LookPath() error { return nil }

func (executor *fakeTrivyExecutor) Scan(ctx context.Context, root string) ([]byte, error) {
	executor.runs++
	return executor.scan(ctx, root)
}

func TestAnalyzeStagesOnlySelectedSafeTargets(t *testing.T) {
	repositoryRoot := t.TempDir()
	require.NoError(t, writeSecurityTestFile(repositoryRoot, "changed.go", "package changed\n"))
	require.NoError(t, writeSecurityTestFile(repositoryRoot, "unchanged.go", "package unchanged\n"))
	require.NoError(t, writeSecurityTestFile(repositoryRoot, "node_modules/generated.js", "generated();\n"))

	executor := &fakeTrivyExecutor{scan: func(_ context.Context, scanRoot string) ([]byte, error) {
		assert.NotEqual(t, repositoryRoot, scanRoot)
		assert.FileExists(t, filepath.Join(scanRoot, "changed.go"))
		assert.NoFileExists(t, filepath.Join(scanRoot, "unchanged.go"))
		assert.NoFileExists(t, filepath.Join(scanRoot, "node_modules", "generated.js"))
		return []byte(`{"Results":[{"Target":"changed.go","Secrets":[{"RuleID":"test-secret","Category":"test","Severity":"HIGH","Title":"Selected secret","StartLine":1,"EndLine":1,"Match":"secret"}]},{"Target":"unchanged.go","Secrets":[{"RuleID":"must-be-filtered","Category":"test","Severity":"HIGH","Title":"Unchanged secret","StartLine":1,"EndLine":1,"Match":"secret"}]}]}`), nil
	}}
	repo := &git.Repository{FS: osfs.New(repositoryRoot), Path: repositoryRoot}
	ctx := securityTestContext()
	ctx = scancore.WithTargetFiles(ctx, []string{"/changed.go"}, true)

	result, err := (&TrivyAnalyzer{executor: executor}).Analyze(ctx, repo)
	require.NoError(t, err)
	require.Len(t, result.Issues, 1)
	assert.Equal(t, "changed.go", result.Issues[0].FilePath)
	assert.Equal(t, 1, executor.runs)
}

func TestAnalyzeDoesNotRunTrivyForEmptySelection(t *testing.T) {
	executor := &fakeTrivyExecutor{scan: func(context.Context, string) ([]byte, error) {
		t.Fatal("Trivy must not run for an empty filtered target set")
		return nil, nil
	}}
	repositoryRoot := t.TempDir()
	repo := &git.Repository{FS: osfs.New(repositoryRoot), Path: repositoryRoot}
	ctx := scancore.WithTargetFiles(securityTestContext(), []string{}, true)

	result, err := (&TrivyAnalyzer{executor: executor}).Analyze(ctx, repo)
	require.NoError(t, err)
	assert.Empty(t, result.Issues)
	assert.Equal(t, "no analyzable target files", result.Metrics["skip_reason"])
	assert.Zero(t, executor.runs)
}

func securityTestContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "analysisRunID", uuid.New())
	ctx = context.WithValue(ctx, "repositoryID", uuid.New())
	ctx = context.WithValue(ctx, "userID", uuid.New())
	return ctx
}

func writeSecurityTestFile(root, name, contents string) error {
	fullPath := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, []byte(contents), 0o644)
}
