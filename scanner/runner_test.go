package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/filepolicy"
	gitservice "github.com/endrilickollari/debtdrone-cli/v2/internal/git"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/scancore"
	"github.com/endrilickollari/debtdrone-cli/v2/scanner/repostructure"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testAnalyzer struct {
	id      string
	name    string
	result  AnalyzerResult
	err     error
	panicOn bool
}

type warningCoreAnalyzer struct{}

func (warningCoreAnalyzer) Name() string { return "Warning analyzer" }
func (warningCoreAnalyzer) Analyze(context.Context, *gitservice.Repository) (*scancore.Result, error) {
	return &scancore.Result{Warnings: []string{"structured warning"}}, nil
}

func (a testAnalyzer) ID() string   { return a.id }
func (a testAnalyzer) Name() string { return a.name }
func (a testAnalyzer) Analyze(context.Context) (AnalyzerResult, error) {
	if a.panicOn {
		panic("broken analyzer")
	}
	return a.result, a.err
}

func TestRunnerPreservesSuccessfulResultsAndReportsFailures(t *testing.T) {
	runner := Runner{MaxParallel: 2}
	report, err := runner.Run(context.Background(), []Analyzer{
		testAnalyzer{id: "z", name: "Z", result: AnalyzerResult{Findings: []Finding{{AnalyzerID: "z", Message: "later"}}}},
		testAnalyzer{id: "failed", name: "Failed", err: errors.New("scan failed")},
		testAnalyzer{id: "panic", name: "Panic", panicOn: true},
		testAnalyzer{id: "a", name: "A", result: AnalyzerResult{Findings: []Finding{{AnalyzerID: "a", Message: "first"}}}},
	})

	var partial *PartialFailureError
	require.ErrorAs(t, err, &partial)
	require.Len(t, report.Findings, 2)
	assert.Equal(t, []string{"a", "z"}, []string{report.Findings[0].AnalyzerID, report.Findings[1].AnalyzerID})
	require.Len(t, report.Failures, 2)
	assert.Equal(t, "failed", report.Failures[0].AnalyzerID)
	assert.Contains(t, report.Failures[1].Error, "panic: broken analyzer")
}

func TestLegacyAnalyzerConvertsStructuredWarnings(t *testing.T) {
	result, err := (legacyAnalyzer{id: "security", analyzer: warningCoreAnalyzer{}}).Analyze(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Warning{{AnalyzerID: "security", Message: "structured warning"}}, result.Warnings)
}

func TestRunnerReturnsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (Runner{}).Run(ctx, []Analyzer{testAnalyzer{id: "a", name: "A"}})
	require.ErrorIs(t, err, context.Canceled)
}

func TestRunnerPropagatesCancellationToRunningAnalyzer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := (Runner{}).Run(ctx, []Analyzer{cancelAwareAnalyzer{started: started}})
		done <- err
	}()

	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}

func TestRunnerProgressIsMonotonicAndCallbackPanicsAreWarnings(t *testing.T) {
	var mu sync.Mutex
	var started []int
	callbackCalls := 0
	runner := Runner{MaxParallel: 3, OnProgress: func(event ProgressEvent) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalls++
		if event.Phase == ProgressStarted {
			started = append(started, event.Index)
		}
		if callbackCalls == 1 {
			panic("broken progress handler")
		}
	}}

	report, err := runner.Run(context.Background(), []Analyzer{
		testAnalyzer{id: "a", name: "A"},
		testAnalyzer{id: "b", name: "B"},
		testAnalyzer{id: "c", name: "C"},
	})
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2}, started)
	require.Len(t, report.Warnings, 1)
	assert.Contains(t, report.Warnings[0].Message, "progress handler panic")
}

func TestRunnerRecoversAnalyzerMetadataPanic(t *testing.T) {
	report, err := (Runner{}).Run(context.Background(), []Analyzer{metadataPanicAnalyzer{}})
	var partial *PartialFailureError
	require.ErrorAs(t, err, &partial)
	require.Len(t, report.Failures, 1)
	assert.Contains(t, report.Failures[0].Error, "metadata panic")
}

func TestScanNoChangesDoesNotOpenRepository(t *testing.T) {
	report, err := Scan(context.Background(), "/path/that/does/not/exist", Options{Scope: NoChanges()})
	require.NoError(t, err)
	assert.Empty(t, report.Findings)
	assert.Empty(t, report.Failures)
}

func TestNoChangesSkipsUnavailableOptionalCapabilities(t *testing.T) {
	report, err := Scan(context.Background(), "/path/that/does/not/exist", Options{Scope: NoChanges(), Coverage: CoverageOptions{Enabled: true}})
	require.NoError(t, err)
	assert.Empty(t, report.Findings)
	assert.Empty(t, report.Metrics)
}

func TestScanRejectsUnavailableCoverageCapability(t *testing.T) {
	_, err := Scan(context.Background(), ".", Options{Coverage: CoverageOptions{Enabled: true}})
	require.EqualError(t, err, "coverage scanning is not available until Phase 2")
}

func TestScanReturnsNeutralMetrics(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, writeTestFile(root, "main.go", "package main\n\nfunc main() {}\n"))

	options := DefaultOptions()
	report, err := Scan(context.Background(), root, options)
	require.NoError(t, err)
	require.Contains(t, report.Metrics, "line_counter")
	assert.EqualValues(t, 3, metricValue(t, report.Metrics["line_counter"], "loc"))
}

func TestScanReturnsRepositoryStructureMetrics(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, writeTestFile(root, "go.mod", "module example.com/test\n\ngo 1.26\n"))
	require.NoError(t, writeTestFile(root, "cmd/server/main.go", "package main\n\nfunc main() {}\n"))

	report, err := Scan(context.Background(), root, DefaultOptions())
	require.NoError(t, err)
	require.Contains(t, report.Metrics, "repository_structure")
	roots, ok := metricValue(t, report.Metrics["repository_structure"], "build_roots").([]repostructure.BuildRoot)
	require.True(t, ok)
	require.Len(t, roots, 1)
	assert.Equal(t, ".", roots[0].Dir)
	assert.Equal(t, "go", roots[0].Tool)
	assert.Equal(t, "golang:1.26-alpine", roots[0].DockerImage)
	assert.Equal(t, []string{"cmd/server/main.go"}, metricValue(t, report.Metrics["repository_structure"], "entry_points"))
}

func TestRepositoryStructureMetricsAreStableAcrossCheckoutPaths(t *testing.T) {
	metrics := make([][]Metric, 0, 2)
	for range 2 {
		root := t.TempDir()
		require.NoError(t, writeTestFile(root, "go.mod", "module example.com/test\n\ngo 1.26\n"))
		require.NoError(t, writeTestFile(root, "cmd/server/main.go", "package main\n\nfunc main() {}\n"))

		report, err := Scan(context.Background(), root, DefaultOptions())
		require.NoError(t, err)
		metrics = append(metrics, report.Metrics["repository_structure"])
	}

	assert.Equal(t, metrics[0], metrics[1])
}

func TestScanReturnsPartialRepositoryStructureWarnings(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, writeTestFile(root, "package.json", "{"))
	require.NoError(t, writeTestFile(root, "index.js", "export const value = true;\n"))

	report, err := Scan(context.Background(), root, DefaultOptions())
	require.NoError(t, err)
	require.Contains(t, report.Metrics, "repository_structure")
	require.Len(t, report.Warnings, 1)
	assert.Equal(t, "repository_structure", report.Warnings[0].AnalyzerID)
	assert.Contains(t, report.Warnings[0].Message, "package.json")
}

func TestScanReturnsCancellationDuringTargetPreparation(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, writeTestFile(root, "main.go", "package main\n"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Scan(ctx, root, Options{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestPrepareTargetFilesSurfacesTraversalErrors(t *testing.T) {
	fs := memfs.New()
	file, err := fs.Create("broken.go")
	require.NoError(t, err)
	require.NoError(t, file.Close())
	walkErr := errors.New("permission denied")
	repo := &gitservice.Repository{FS: failingLstatFilesystem{Filesystem: fs, target: "/broken.go", err: walkErr}}

	_, _, err = prepareTargetFiles(context.Background(), repo, FullScan())
	require.ErrorIs(t, err, walkErr)
	require.ErrorContains(t, err, "discover repository files")
}

func TestIncrementalScanScopesBuiltInAnalyzers(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.go")
	require.NoError(t, writeTestFile(root, "first.go", "package first\n\nfunc First() {}\n"))
	require.NoError(t, writeTestFile(root, "second.py", "def ignored():\n    if True:\n        if True:\n            if True:\n                if True:\n                    if True:\n                        if True:\n                            if True:\n                                if True:\n                                    if True:\n                                        if True:\n                                            pass\n"))

	report, err := Scan(context.Background(), root, Options{Scope: IncrementalScan([]string{first}), Complexity: ComplexityOptions{Enabled: true}})
	require.NoError(t, err)
	assert.EqualValues(t, 3, metricValue(t, report.Metrics["line_counter"], "loc"))
	assert.EqualValues(t, 1, metricValue(t, report.Metrics["line_counter"], "file_count"))
	assert.Empty(t, report.Findings, "non-target complexity findings must be excluded")
}

func TestIncrementalScanRejectsFilesOutsideRepository(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.go")
	_, err := Scan(context.Background(), root, Options{Scope: IncrementalScan([]string{outside})})
	require.ErrorContains(t, err, "outside repository root")
}

func TestIncrementalScanRejectsPortableTraversal(t *testing.T) {
	root := t.TempDir()
	_, err := Scan(context.Background(), root, Options{Scope: IncrementalScan([]string{`..\outside.go`})})
	require.ErrorContains(t, err, "outside repository root")
}

func TestIncrementalScanRejectsWindowsAbsolutePathOnOtherPlatforms(t *testing.T) {
	root := t.TempDir()
	_, err := Scan(context.Background(), root, Options{Scope: IncrementalScan([]string{`C:\outside.go`})})
	if runtime.GOOS == "windows" {
		require.ErrorContains(t, err, "outside repository root")
		return
	}
	require.ErrorContains(t, err, "absolute path for a different platform")
}

func TestIncrementalScanRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.go")
	require.NoError(t, os.WriteFile(outside, []byte("package outside\n"), 0o644))
	link := filepath.Join(root, "linked.go")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := Scan(context.Background(), root, Options{Scope: IncrementalScan([]string{"linked.go"})})
	require.ErrorContains(t, err, "resolves outside repository root")
}

func TestIncrementalScanNormalizesWindowsSeparators(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, writeTestFile(root, "nested/main.go", "package nested\n"))

	report, err := Scan(context.Background(), root, Options{Scope: IncrementalScan([]string{`nested\main.go`})})
	require.NoError(t, err)
	assert.EqualValues(t, 1, metricValue(t, report.Metrics["line_counter"], "file_count"))
}

func TestIncrementalScanWithOnlyFilteredFilesStaysEmpty(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, writeTestFile(root, "main.go", "package main\n\nfunc main() {}\n"))
	require.NoError(t, writeTestFile(root, "assets/logo.png", "not an image\n"))

	report, err := Scan(context.Background(), root, Options{
		Scope:      IncrementalScan([]string{"assets/logo.png"}),
		Complexity: ComplexityOptions{Enabled: true},
	})
	require.NoError(t, err)
	assert.EqualValues(t, 0, metricValue(t, report.Metrics["line_counter"], "file_count"))
	assert.EqualValues(t, 0, metricValue(t, report.Metrics["line_counter"], "loc"))
	assert.Empty(t, report.Findings, "an empty safe target set must not fall back to a full scan")
}

func TestIncrementalScanExcludesExplicitGeneratedPath(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, writeTestFile(root, "main.go", "package main\n"))
	require.NoError(t, writeTestFile(root, "node_modules/generated.js", "generated();\n"))

	report, err := Scan(context.Background(), root, Options{Scope: IncrementalScan([]string{"node_modules/generated.js"})})
	require.NoError(t, err)
	assert.EqualValues(t, 0, metricValue(t, report.Metrics["line_counter"], "file_count"))
	assert.Empty(t, report.Warnings)
}

func TestFullScanFiltersGeneratedAssetsAndUnsafeFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, writeTestFile(root, "src/main.go", "package main\n\nfunc main() {}\n"))
	require.NoError(t, writeTestFile(root, "node_modules/generated.js", "generated();\n"))
	require.NoError(t, writeTestFile(root, "dist/bundle.js", "bundled();\n"))
	require.NoError(t, writeTestFile(root, "assets/logo.png", "not an image\n"))

	oversized := filepath.Join(root, "oversized.go")
	file, err := os.Create(oversized)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(filepolicy.MaxFileSize+1))
	require.NoError(t, file.Close())

	report, err := Scan(context.Background(), root, Options{Complexity: ComplexityOptions{Enabled: true}})
	require.NoError(t, err)
	assert.EqualValues(t, 1, metricValue(t, report.Metrics["line_counter"], "file_count"))
	require.Len(t, report.Warnings, 1)
	assert.Equal(t, "scanner", report.Warnings[0].AnalyzerID)
	assert.Contains(t, report.Warnings[0].Message, "/oversized.go skipped: file exceeds maximum size limit")
}

func TestFullScanDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, writeTestFile(root, "main.go", "package main\n"))
	outside := filepath.Join(t.TempDir(), "outside.go")
	require.NoError(t, os.WriteFile(outside, []byte("package outside\n"), 0o644))
	if err := os.Symlink(outside, filepath.Join(root, "linked.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	report, err := Scan(context.Background(), root, Options{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, metricValue(t, report.Metrics["line_counter"], "file_count"))
}

func TestFingerprintIgnoresVolatileLineMessageAndWhitespace(t *testing.T) {
	rule := "complexity"
	lineOne, lineTwo := 10, 200
	snippetOne := "func Work() {\n  doThing()\n}"
	snippetTwo := " func   Work() { doThing() } "
	first := Finding{AnalyzerID: "complexity_analyzer", RuleID: &rule, Type: "complexity", Message: "old wording", Location: Location{Path: "/src/work.go", Line: &lineOne}, CodeSnippet: &snippetOne}
	second := Finding{AnalyzerID: "renamed_adapter", RuleID: &rule, Type: "complexity", Message: "new wording", Location: Location{Path: "/src/work.go", Line: &lineTwo}, CodeSnippet: &snippetTwo}

	assert.Equal(t, fingerprint(first), fingerprint(second))
	assert.Equal(t, "362fa643800ba8b7d45d03dff69403723a294a80a900f0f35949381aac7aaf71", fingerprint(first))
}

type metadataPanicAnalyzer struct{}

func (metadataPanicAnalyzer) ID() string   { panic("broken metadata") }
func (metadataPanicAnalyzer) Name() string { return "never reached" }
func (metadataPanicAnalyzer) Analyze(context.Context) (AnalyzerResult, error) {
	return AnalyzerResult{}, nil
}

type cancelAwareAnalyzer struct {
	started chan<- struct{}
}

type failingLstatFilesystem struct {
	billy.Filesystem
	target string
	err    error
}

func (fs failingLstatFilesystem) Lstat(path string) (os.FileInfo, error) {
	if filepath.ToSlash(path) == fs.target {
		return nil, fs.err
	}
	return fs.Filesystem.Lstat(path)
}

func (cancelAwareAnalyzer) ID() string   { return "cancel_aware" }
func (cancelAwareAnalyzer) Name() string { return "Cancel Aware" }
func (a cancelAwareAnalyzer) Analyze(ctx context.Context) (AnalyzerResult, error) {
	close(a.started)
	<-ctx.Done()
	return AnalyzerResult{}, ctx.Err()
}

func metricValue(t *testing.T, metrics []Metric, name string) any {
	t.Helper()
	for _, metric := range metrics {
		if metric.Name == name {
			return metric.Value
		}
	}
	t.Fatalf("metric %q not found", name)
	return nil
}
