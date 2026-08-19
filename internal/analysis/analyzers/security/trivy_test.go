package security

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/scancore"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTrivyExecutor struct {
	runs        int
	lookPathErr error
	scan        func(context.Context, string) ([]byte, error)
}

func (executor *fakeTrivyExecutor) LookPath() error { return executor.lookPathErr }

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

func TestAnalyzeMissingTrivyReturnsStructuredWarningAndStableMetrics(t *testing.T) {
	executor := &fakeTrivyExecutor{
		lookPathErr: errors.New("executable not found"),
		scan: func(context.Context, string) ([]byte, error) {
			t.Fatal("Trivy must not run when it is unavailable")
			return nil, nil
		},
	}
	root := t.TempDir()
	repo := &git.Repository{FS: osfs.New(root), Path: root}

	result, err := (&TrivyAnalyzer{executor: executor}).Analyze(securityTestContext(), repo)
	require.NoError(t, err)
	assert.Empty(t, result.Issues)
	assert.Equal(t, []string{missingTrivyWarning}, result.Warnings)
	assert.Equal(t, false, result.Metrics["trivy_available"])
	assert.Equal(t, "trivy not installed", result.Metrics["skip_reason"])
	assertStableSecurityMetrics(t, result.Metrics, true)
	assert.Zero(t, executor.runs)
}

func TestAnalyzeRetainsValidPartialOutputAndReturnsWarning(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, writeSecurityTestFile(root, "changed.go", "package changed\n"))
	executor := &fakeTrivyExecutor{scan: func(context.Context, string) ([]byte, error) {
		return []byte(`{"Results":[{"Target":"changed.go","Vulnerabilities":[{"VulnerabilityID":"CVE-1","PkgName":"example","Title":"Example vulnerability","Severity":"HIGH"}]}]}`), errors.New("exit status 1")
	}}
	repo := &git.Repository{FS: osfs.New(root), Path: root}
	ctx := scancore.WithTargetFiles(securityTestContext(), []string{"/changed.go"}, true)

	result, err := (&TrivyAnalyzer{executor: executor}).Analyze(ctx, repo)
	require.NoError(t, err)
	require.Len(t, result.Issues, 1)
	assert.Equal(t, "CVE-1", *result.Issues[0].ToolRuleID)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "valid partial results were retained")
	assert.Equal(t, 1, result.Metrics["security_high_count"])
	assertStableSecurityMetrics(t, result.Metrics, false)
}

func TestExecuteTrivyCommandKeepsStderrOutOfJSONOutput(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestTrivyCommandHelperProcess")
	cmd.Env = append(os.Environ(), "DEBTDRONE_TRIVY_HELPER_PROCESS=1")

	output, err := executeTrivyCommand(cmd)
	require.Error(t, err)
	assert.ErrorContains(t, err, "database warning")
	assert.Equal(t, `{"Results":[]}`, string(output))

	var result TrivyOutput
	require.NoError(t, json.Unmarshal(output, &result))
}

func TestTrivyCommandHelperProcess(t *testing.T) {
	if os.Getenv("DEBTDRONE_TRIVY_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, `{"Results":[]}`)
	fmt.Fprint(os.Stderr, "database warning")
	os.Exit(1)
}

func TestAnalyzePropagatesCancellationInsteadOfRetainingPartialOutput(t *testing.T) {
	root := t.TempDir()
	executor := &fakeTrivyExecutor{scan: func(context.Context, string) ([]byte, error) {
		return []byte(`{"Results":[]}`), context.Canceled
	}}
	repo := &git.Repository{FS: osfs.New(root), Path: root}
	ctx, cancel := context.WithCancel(securityTestContext())
	cancel()

	result, err := (&TrivyAnalyzer{executor: executor}).Analyze(ctx, repo)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
}

func assertStableSecurityMetrics(t *testing.T, metrics map[string]interface{}, hasSkipReason bool) {
	t.Helper()
	keys := make([]string, 0, len(metrics))
	for key := range metrics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	expected := []string{
		"secrets_count",
		"security_critical_count",
		"security_high_count",
		"security_issues_count",
		"security_low_count",
		"security_medium_count",
		"trivy_available",
		"vulnerabilities_count",
	}
	if hasSkipReason {
		expected = append(expected, "skip_reason")
		sort.Strings(expected)
	}
	assert.Equal(t, expected, keys)
	assert.NotContains(t, metrics, "critical_issues_count")
	assert.NotContains(t, metrics, "high_issues_count")
	assert.NotContains(t, metrics, "medium_issues_count")
	assert.NotContains(t, metrics, "low_issues_count")
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
