package coverage

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// SimpleCovParser parses SimpleCov's .resultset.json files (Ruby).
// Supports both the legacy format (array of hit counts per line) and the
// newer format that wraps line data under a "lines" key.
type SimpleCovParser struct{}

func (p *SimpleCovParser) CanParse(path string) bool {
	base := filepath.Base(path)
	return base == ".resultset.json" || base == "coverage.json"
}

func (p *SimpleCovParser) Parse(path string) (*Report, error) {
	data, err := readArtifact(path)
	if err != nil {
		return nil, fmt.Errorf("simplecov: %w", err)
	}

	// SimpleCov format:
	// { "SuiteOrResultName": { "coverage": { "file.rb": <lines> }, "timestamp": N } }
	// where <lines> is either:
	//   - []interface{} (legacy: each element is null or an integer hit count)
	//   - {"lines": []interface{}, "branches": {...}} (newer format)
	var raw map[string]struct {
		Coverage map[string]json.RawMessage `json:"coverage"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("simplecov: %w", err)
	}

	// Merge all test suites; take the max hit count per line per file.
	merged := make(map[string]map[int]int)

	for _, suite := range raw {
		for filePath, rawLines := range suite.Coverage {
			// Try newer {"lines": [...]} format first.
			var newFmt struct {
				Lines []interface{} `json:"lines"`
			}
			if json.Unmarshal(rawLines, &newFmt) == nil && newFmt.Lines != nil {
				mergeSimpleCovLines(merged, filePath, newFmt.Lines)
				continue
			}
			// Fall back to legacy flat array.
			var legacyFmt []interface{}
			if json.Unmarshal(rawLines, &legacyFmt) == nil {
				mergeSimpleCovLines(merged, filePath, legacyFmt)
			}
		}
	}

	files := make([]FileCoverage, 0, len(merged))
	for filePath, hits := range merged {
		fc := FileCoverage{
			Path:             normalizeFilePath(filePath),
			BranchCoverage:   -1,
			FunctionCoverage: -1,
		}
		for lineNo, count := range hits {
			fc.LinesTotal++
			if count > 0 {
				fc.LinesCovered++
			} else {
				fc.UncoveredLines = append(fc.UncoveredLines, lineNo)
			}
		}
		if fc.LinesTotal > 0 {
			fc.LineCoverage = float64(fc.LinesCovered) / float64(fc.LinesTotal) * 100
		}
		files = append(files, fc)
	}

	linePct, branchPct := computeOverall(files)
	return &Report{
		Files:            files,
		OverallLinePct:   linePct,
		OverallBranchPct: branchPct,
		Format:           "simplecov",
	}, nil
}

// mergeSimpleCovLines merges a per-line array into the accumulator map.
// Null entries represent non-executable lines and are skipped.
func mergeSimpleCovLines(merged map[string]map[int]int, filePath string, lines []interface{}) {
	if merged[filePath] == nil {
		merged[filePath] = make(map[int]int)
	}
	for i, v := range lines {
		if v == nil {
			continue
		}
		lineNo := i + 1
		var count int
		switch val := v.(type) {
		case float64:
			count = int(val)
		case int:
			count = val
		}
		if existing, ok := merged[filePath][lineNo]; !ok || count > existing {
			merged[filePath][lineNo] = count
		}
	}
}
