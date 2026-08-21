package coverage

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// GoCoverageParser parses Go's native coverage.out format produced by
// `go test -coverprofile=coverage.out`.
type GoCoverageParser struct{}

func (p *GoCoverageParser) CanParse(path string) bool {
	return filepath.Base(path) == "coverage.out"
}

func (p *GoCoverageParser) Parse(path string) (*Report, error) {
	data, err := readArtifact(path)
	if err != nil {
		return nil, fmt.Errorf("go coverage: %w", err)
	}

	type fileCoverage struct {
		statements        int
		coveredStatements int
		uncoveredLines    map[int]struct{}
	}
	filesByPath := make(map[string]*fileCoverage)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "mode:") {
			continue
		}
		// format: pkg/path/file.go:startLine.startCol,endLine.endCol numStmt count
		colonIdx := strings.LastIndex(text, ":")
		if colonIdx < 0 {
			continue
		}
		filePart := text[:colonIdx]
		rest := text[colonIdx+1:]

		fields := strings.Fields(rest)
		if len(fields) < 3 {
			continue
		}
		rangeParts := strings.SplitN(fields[0], ",", 2)
		if len(rangeParts) != 2 {
			continue
		}
		startFields := strings.SplitN(rangeParts[0], ".", 2)
		endFields := strings.SplitN(rangeParts[1], ".", 2)
		if len(startFields) < 1 || len(endFields) < 1 {
			continue
		}
		startLine, err := strconv.Atoi(startFields[0])
		if err != nil {
			continue
		}
		endLine, err := strconv.Atoi(endFields[0])
		if err != nil {
			continue
		}
		numStatements, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}

		if startLine < 1 || endLine < startLine || numStatements < 1 {
			continue
		}
		file := filesByPath[filePart]
		if file == nil {
			file = &fileCoverage{uncoveredLines: make(map[int]struct{})}
			filesByPath[filePart] = file
		}
		file.statements += numStatements
		if count > 0 {
			file.coveredStatements += numStatements
		} else {
			// Coverage profiles expose statement counts per block, not a list of
			// executable lines. Keep the block's first line as a stable location
			// hint while using numStatements for the percentage calculation.
			file.uncoveredLines[startLine] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("go coverage: %w", err)
	}

	files := make([]FileCoverage, 0, len(filesByPath))
	for filePath, parsed := range filesByPath {
		// Go coverage paths are package paths (e.g. github.com/org/repo/pkg/file.go)
		// We normalize them by keeping only the part after the last /cmd/, /internal/, /pkg/, etc.
		// if they exist, or just use the base name if all else fails.
		// Actually, the most robust way is to just keep the file path from a known root.
		normPath := filePath
		for _, seg := range []string{"/cmd/", "/internal/", "/pkg/", "/src/", "/lib/"} {
			if idx := strings.LastIndex(filePath, seg); idx >= 0 {
				normPath = filePath[idx+1:]
				break
			}
		}
		// If it's still a full package path with no conventional segments,
		// try to at least remove the domain part (e.g. github.com/user/repo/)
		if normPath == filePath && strings.Count(filePath, "/") >= 3 {
			parts := strings.SplitN(filePath, "/", 4)
			if len(parts) == 4 {
				normPath = parts[3]
			}
		}

		fc := FileCoverage{
			Path:             normPath,
			LinesCovered:     parsed.coveredStatements,
			LinesTotal:       parsed.statements,
			BranchCoverage:   -1,
			FunctionCoverage: -1,
		}
		for line := range parsed.uncoveredLines {
			fc.UncoveredLines = append(fc.UncoveredLines, line)
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
		Format:           "go",
	}, nil
}
