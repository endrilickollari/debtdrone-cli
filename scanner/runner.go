package scanner

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

type Runner struct {
	MaxParallel int
	OnProgress  func(ProgressEvent)
}

type analyzerDescriptor struct {
	analyzer Analyzer
	id       string
	name     string
	failure  *AnalyzerFailure
}

type analyzerOutcome struct {
	result   AnalyzerResult
	failure  *AnalyzerFailure
	warnings []Warning
}

func (r Runner) Run(ctx context.Context, analyzers []Analyzer) (Report, error) {
	report := Report{Metrics: make(map[string][]Metric)}
	if len(analyzers) == 0 {
		return report, nil
	}

	descriptors := inspectAnalyzers(analyzers)
	limit := r.MaxParallel
	if limit <= 0 || limit > len(analyzers) {
		limit = len(analyzers)
	}

	outcomes := make([]analyzerOutcome, len(analyzers))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var progressMu sync.Mutex
	started, completed := 0, 0
	emit := func(event ProgressEvent) (panicErr error) {
		if r.OnProgress == nil {
			return nil
		}
		progressMu.Lock()
		defer progressMu.Unlock()
		switch event.Phase {
		case ProgressStarted:
			event.Index = started
			started++
		case ProgressFinished, ProgressFailed:
			event.Index = completed
			completed++
			event.Completed = completed
		}
		defer func() {
			if recovered := recover(); recovered != nil {
				panicErr = fmt.Errorf("progress handler panic: %v", recovered)
			}
		}()
		r.OnProgress(event)
		return nil
	}

	for index, descriptor := range descriptors {
		index, descriptor := index, descriptor
		if descriptor.failure != nil {
			outcomes[index].failure = descriptor.failure
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			event := ProgressEvent{AnalyzerID: descriptor.id, AnalyzerName: descriptor.name, Total: len(analyzers), Phase: ProgressStarted}
			if err := emit(event); err != nil {
				outcomes[index].warnings = append(outcomes[index].warnings, Warning{AnalyzerID: descriptor.id, Message: err.Error()})
			}

			result, err := analyzeSafely(ctx, descriptor.analyzer)
			if err != nil {
				outcomes[index].failure = &AnalyzerFailure{AnalyzerID: descriptor.id, AnalyzerName: descriptor.name, Error: err.Error()}
				event.Phase = ProgressFailed
			} else {
				outcomes[index].result = result
				event.Phase = ProgressFinished
			}
			if err := emit(event); err != nil {
				outcomes[index].warnings = append(outcomes[index].warnings, Warning{AnalyzerID: descriptor.id, Message: err.Error()})
			}
		}()
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return report, err
	}
	for index, outcome := range outcomes {
		report.Warnings = append(report.Warnings, outcome.warnings...)
		if outcome.failure != nil {
			report.Failures = append(report.Failures, *outcome.failure)
			continue
		}
		id := descriptors[index].id
		report.Findings = append(report.Findings, outcome.result.Findings...)
		report.Warnings = append(report.Warnings, outcome.result.Warnings...)
		if outcome.result.Metrics != nil {
			report.Metrics[id] = outcome.result.Metrics
		}
	}

	sort.SliceStable(report.Findings, func(i, j int) bool {
		a, b := report.Findings[i], report.Findings[j]
		if a.AnalyzerID != b.AnalyzerID {
			return a.AnalyzerID < b.AnalyzerID
		}
		if a.Location.Path != b.Location.Path {
			return a.Location.Path < b.Location.Path
		}
		lineA, lineB := 0, 0
		if a.Location.Line != nil {
			lineA = *a.Location.Line
		}
		if b.Location.Line != nil {
			lineB = *b.Location.Line
		}
		if lineA != lineB {
			return lineA < lineB
		}
		return a.Message < b.Message
	})

	if len(report.Failures) > 0 {
		return report, &PartialFailureError{Failures: append([]AnalyzerFailure(nil), report.Failures...)}
	}
	return report, nil
}

func inspectAnalyzers(analyzers []Analyzer) []analyzerDescriptor {
	descriptors := make([]analyzerDescriptor, len(analyzers))
	seen := make(map[string]struct{}, len(analyzers))
	for index, analyzer := range analyzers {
		descriptor := inspectAnalyzer(index, analyzer)
		if descriptor.failure == nil {
			if _, duplicate := seen[descriptor.id]; duplicate {
				descriptor.failure = &AnalyzerFailure{AnalyzerID: descriptor.id, AnalyzerName: descriptor.name, Error: "duplicate analyzer ID"}
			} else {
				seen[descriptor.id] = struct{}{}
			}
		}
		descriptors[index] = descriptor
	}
	return descriptors
}

func inspectAnalyzer(index int, analyzer Analyzer) (descriptor analyzerDescriptor) {
	descriptor = analyzerDescriptor{analyzer: analyzer, id: fmt.Sprintf("analyzer_%d", index), name: fmt.Sprintf("Analyzer %d", index)}
	defer func() {
		if recovered := recover(); recovered != nil {
			descriptor.failure = &AnalyzerFailure{AnalyzerID: descriptor.id, AnalyzerName: descriptor.name, Error: fmt.Sprintf("analyzer metadata panic: %v", recovered)}
		}
	}()
	if analyzer == nil {
		descriptor.failure = &AnalyzerFailure{AnalyzerID: descriptor.id, AnalyzerName: descriptor.name, Error: "nil analyzer"}
		return descriptor
	}
	descriptor.id = analyzer.ID()
	descriptor.name = analyzer.Name()
	if descriptor.id == "" {
		descriptor.failure = &AnalyzerFailure{AnalyzerID: fmt.Sprintf("analyzer_%d", index), AnalyzerName: descriptor.name, Error: "empty analyzer ID"}
	}
	return descriptor
}

func analyzeSafely(ctx context.Context, analyzer Analyzer) (result AnalyzerResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic: %v", recovered)
		}
	}()
	return analyzer.Analyze(ctx)
}
