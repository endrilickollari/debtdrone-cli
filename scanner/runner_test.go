package scanner

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

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
