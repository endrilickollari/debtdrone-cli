package scanner

import (
	"context"
	"fmt"
	"strings"
)

type ScopeMode string

const (
	ScopeFull        ScopeMode = "full"
	ScopeIncremental ScopeMode = "incremental"
	ScopeNoChanges   ScopeMode = "no_changes"
)

type Scope struct {
	Mode  ScopeMode
	Files []string
}

// FullScan discovers source files below the repository root, excluding known
// generated directories and files that fail the bounded safety policy.
func FullScan() Scope { return Scope{Mode: ScopeFull} }

// IncrementalScan restricts file-based analyzers to the supplied repository
// paths. An empty list is invalid; a list containing only filtered files is a
// valid empty scan and never falls back to FullScan.
func IncrementalScan(files []string) Scope {
	return Scope{Mode: ScopeIncremental, Files: append([]string(nil), files...)}
}

// NoChanges explicitly represents an empty delta and returns without opening
// the repository or invoking analyzers.
func NoChanges() Scope { return Scope{Mode: ScopeNoChanges} }

type ComplexityOptions struct {
	Enabled       bool
	MaxCyclomatic int
}

type SecurityOptions struct {
	Enabled bool
}

// CoverageArtifact is an in-memory report supplied by a scanner consumer.
type CoverageArtifact struct {
	Name string
	// Root identifies the repository-relative build root that produced the
	// artifact. It is required to disambiguate uploads in monorepositories.
	Root    string
	Content []byte
}

// CoverageOptions controls artifact parsing and optional local test execution.
// Coverage is disabled by default. RunLocalTests must be enabled separately
// because it executes code from the scanned repository.
type CoverageOptions struct {
	Enabled       bool
	Artifacts     []CoverageArtifact
	RunLocalTests bool
}

type Options struct {
	Scope       Scope
	Complexity  ComplexityOptions
	Security    SecurityOptions
	Coverage    CoverageOptions
	MaxParallel int
	OnProgress  func(ProgressEvent)
}

func DefaultOptions() Options {
	return Options{
		Scope:      FullScan(),
		Complexity: ComplexityOptions{Enabled: true, MaxCyclomatic: 10},
	}
}

type Location struct {
	Path   string
	Line   *int
	Column *int
}

type Finding struct {
	Fingerprint        string
	AnalyzerID         string
	RuleID             *string
	Type               string
	Category           string
	Severity           string
	Message            string
	Description        *string
	Location           Location
	Confidence         float64
	EstimatedDebtHours float64
	EffortMultiplier   float64
	CodeSnippet        *string
}

type Warning struct {
	AnalyzerID string
	Message    string
}

type AnalyzerFailure struct {
	AnalyzerID   string
	AnalyzerName string
	Error        string
}

// Metric is a named analyzer measurement. Value is intentionally open so
// analyzers can report counts, ratios, booleans, and structured summaries.
type Metric struct {
	Name  string
	Value any
}

type AnalyzerResult struct {
	Findings []Finding
	Metrics  []Metric
	Warnings []Warning
}

type Report struct {
	Findings []Finding
	Metrics  map[string][]Metric
	Warnings []Warning
	Failures []AnalyzerFailure
}

type ProgressPhase string

const (
	ProgressStarted  ProgressPhase = "started"
	ProgressFinished ProgressPhase = "finished"
	ProgressFailed   ProgressPhase = "failed"
)

type ProgressEvent struct {
	AnalyzerID   string
	AnalyzerName string
	// Index is a monotonic, zero-based sequence within the event phase.
	Index int
	// Completed is the number of analyzers that have finished or failed.
	Completed int
	Total     int
	Phase     ProgressPhase
}

// Analyzer is the extension point used by Runner. Implementations should bind
// their source during construction and remain free of SaaS infrastructure.
type Analyzer interface {
	ID() string
	Name() string
	Analyze(context.Context) (AnalyzerResult, error)
}

type PartialFailureError struct {
	Failures []AnalyzerFailure
}

func (e *PartialFailureError) Error() string {
	parts := make([]string, 0, len(e.Failures))
	for _, failure := range e.Failures {
		parts = append(parts, fmt.Sprintf("%s: %s", failure.AnalyzerID, failure.Error))
	}
	return "one or more analyzers failed: " + strings.Join(parts, "; ")
}
