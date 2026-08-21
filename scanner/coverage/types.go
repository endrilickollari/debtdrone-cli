// Package coverage parses coverage artifacts and optionally invokes local test
// runners without depending on CLI or SaaS infrastructure.
package coverage

// Artifact is an in-memory coverage report supplied by a scanner consumer.
type Artifact struct {
	Name string
	// Root identifies the repository-relative build root that produced the
	// artifact. It is required to disambiguate uploads in monorepositories.
	Root    string
	Content []byte
}

// FileCoverage describes coverage for one source file.
type FileCoverage struct {
	Path             string  `json:"path"`
	LineCoverage     float64 `json:"line_coverage"`
	LinesCovered     int     `json:"lines_covered"`
	LinesTotal       int     `json:"lines_total"`
	BranchCoverage   float64 `json:"branch_coverage"`
	FunctionCoverage float64 `json:"function_coverage"`
	UncoveredLines   []int   `json:"uncovered_lines,omitempty"`
}

// Report is the normalized result of one or more coverage artifacts.
type Report struct {
	Files            []FileCoverage `json:"files"`
	OverallLinePct   float64        `json:"overall_line_percentage"`
	OverallBranchPct float64        `json:"overall_branch_percentage"`
	Format           string         `json:"format"`
}

// Parser reads a supported coverage file format.
type Parser interface {
	CanParse(path string) bool
	Parse(path string) (*Report, error)
}

// Options controls coverage collection. Test execution remains separately
// opt-in because it executes repository code and may create coverage artifacts.
type Options struct {
	Artifacts     []Artifact
	RunLocalTests bool
}

// Result contains normalized coverage data and recoverable diagnostics.
type Result struct {
	Report   *Report
	Warnings []string
}

func defaultParsers() []Parser {
	return []Parser{
		&GoCoverageParser{},
		&LCOVParser{},
		&CoberturaParser{},
		&JaCoCoParser{},
		&CloverParser{},
		&SimpleCovParser{},
	}
}

func computeOverall(files []FileCoverage) (linePct, branchPct float64) {
	var totalLines, coveredLines, branchWeightTotal int
	var branchWeightSum float64
	for _, file := range files {
		totalLines += file.LinesTotal
		coveredLines += file.LinesCovered
		if file.BranchCoverage >= 0 {
			branchWeightSum += file.BranchCoverage * float64(file.LinesTotal)
			branchWeightTotal += file.LinesTotal
		}
	}
	if totalLines > 0 {
		linePct = float64(coveredLines) / float64(totalLines) * 100
	}
	if branchWeightTotal == 0 {
		return linePct, -1
	}
	return linePct, branchWeightSum / float64(branchWeightTotal)
}
