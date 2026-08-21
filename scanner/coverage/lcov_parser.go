package coverage

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// LCOVParser parses LCOV info files (lcov.info / coverage.info).
// This format is produced by Istanbul/V8 (JS/TS), gcov+lcov (C/C++),
// cargo-llvm-cov (Rust), llvm-cov (Swift), and coverage.py --lcov (Python).
type LCOVParser struct{}

func (p *LCOVParser) CanParse(path string) bool {
	base := filepath.Base(path)
	return base == "lcov.info" || base == "coverage.info" || base == "lcov.report"
}

func (p *LCOVParser) Parse(path string) (*Report, error) {
	data, err := readArtifact(path)
	if err != nil {
		return nil, fmt.Errorf("lcov: %w", err)
	}

	// Intermediate per-record state.
	type record struct {
		fc          FileCoverage
		branchHit   int
		branchTotal int
		funcHit     int
		funcTotal   int
	}

	var files []FileCoverage
	var cur *record
	finishRecord := func() {
		if cur == nil {
			return
		}
		if cur.fc.LinesTotal > 0 {
			cur.fc.LineCoverage = float64(cur.fc.LinesCovered) / float64(cur.fc.LinesTotal) * 100
		}
		if cur.branchTotal > 0 {
			cur.fc.BranchCoverage = float64(cur.branchHit) / float64(cur.branchTotal) * 100
		}
		if cur.funcTotal > 0 {
			cur.fc.FunctionCoverage = float64(cur.funcHit) / float64(cur.funcTotal) * 100
		}
		files = append(files, cur.fc)
		cur = nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "SF:"):
			finishRecord()
			cur = &record{
				fc: FileCoverage{
					Path:             line[3:],
					BranchCoverage:   -1,
					FunctionCoverage: -1,
				},
			}

		case line == "end_of_record":
			finishRecord()

		case cur == nil:
			continue

		case strings.HasPrefix(line, "DA:"):
			// DA:<lineNo>,<hitCount>
			parts := strings.SplitN(line[3:], ",", 2)
			if len(parts) < 2 {
				continue
			}
			lineNo, err := strconv.Atoi(parts[0])
			if err != nil {
				continue
			}
			hits, err := strconv.Atoi(parts[1])
			if err != nil {
				continue
			}
			cur.fc.LinesTotal++
			if hits > 0 {
				cur.fc.LinesCovered++
			} else {
				cur.fc.UncoveredLines = append(cur.fc.UncoveredLines, lineNo)
			}

		case strings.HasPrefix(line, "BRH:"):
			if v, err := strconv.Atoi(line[4:]); err == nil {
				cur.branchHit = v
			}

		case strings.HasPrefix(line, "BRF:"):
			if v, err := strconv.Atoi(line[4:]); err == nil {
				cur.branchTotal = v
			}

		case strings.HasPrefix(line, "FNH:"):
			if v, err := strconv.Atoi(line[4:]); err == nil {
				cur.funcHit = v
			}

		case strings.HasPrefix(line, "FNF:"):
			if v, err := strconv.Atoi(line[4:]); err == nil {
				cur.funcTotal = v
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("lcov: %w", err)
	}
	finishRecord()

	linePct, branchPct := computeOverall(files)
	return &Report{
		Files:            files,
		OverallLinePct:   linePct,
		OverallBranchPct: branchPct,
		Format:           "lcov",
	}, nil
}
