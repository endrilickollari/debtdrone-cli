package coverage

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
)

// CloverParser parses Clover XML coverage files.
// Used by PHP (PHPUnit) and some Ruby / Python setups.
type CloverParser struct{}

func (p *CloverParser) CanParse(path string) bool {
	base := filepath.Base(path)
	return base == "clover.xml"
}

func (p *CloverParser) Parse(path string) (*Report, error) {
	data, err := readArtifact(path)
	if err != nil {
		return nil, fmt.Errorf("clover: %w", err)
	}

	type xmlMetrics struct {
		Statements        int `xml:"statements,attr"`
		CoveredStatements int `xml:"coveredstatements,attr"`
		Conditionals      int `xml:"conditionals,attr"`
		CoveredConds      int `xml:"coveredconditionals,attr"`
		Methods           int `xml:"methods,attr"`
		CoveredMethods    int `xml:"coveredmethods,attr"`
	}
	type xmlLine struct {
		Num   int    `xml:"num,attr"`
		Type  string `xml:"type,attr"`
		Count int    `xml:"count,attr"`
	}
	type xmlFile struct {
		Name    string     `xml:"name,attr"`
		Metrics xmlMetrics `xml:"metrics"`
		Lines   []xmlLine  `xml:"line"`
	}
	type xmlPackage struct {
		Files []xmlFile `xml:"file"`
	}
	type xmlProject struct {
		Packages []xmlPackage `xml:"package"`
		// Files may appear directly under project (no package grouping).
		Files []xmlFile `xml:"file"`
	}
	type xmlCoverage struct {
		XMLName xml.Name   `xml:"coverage"`
		Project xmlProject `xml:"project"`
	}

	var cov xmlCoverage
	if err := xml.Unmarshal(data, &cov); err != nil {
		return nil, fmt.Errorf("clover: %w", err)
	}

	// Flatten all file nodes.
	var allFiles []xmlFile
	allFiles = append(allFiles, cov.Project.Files...)
	for _, pkg := range cov.Project.Packages {
		allFiles = append(allFiles, pkg.Files...)
	}

	files := make([]FileCoverage, 0, len(allFiles))
	for _, xf := range allFiles {
		fc := FileCoverage{
			Path:             normalizeFilePath(xf.Name),
			BranchCoverage:   -1,
			FunctionCoverage: -1,
		}

		if len(xf.Lines) > 0 {
			for _, l := range xf.Lines {
				if l.Type != "stmt" && l.Type != "method" {
					continue
				}
				fc.LinesTotal++
				if l.Count > 0 {
					fc.LinesCovered++
				} else {
					fc.UncoveredLines = append(fc.UncoveredLines, l.Num)
				}
			}
		} else {
			// Fall back to summary counters when no per-line data is present.
			fc.LinesTotal = xf.Metrics.Statements
			fc.LinesCovered = xf.Metrics.CoveredStatements
		}

		if fc.LinesTotal > 0 {
			fc.LineCoverage = float64(fc.LinesCovered) / float64(fc.LinesTotal) * 100
		}
		if xf.Metrics.Conditionals > 0 {
			fc.BranchCoverage = float64(xf.Metrics.CoveredConds) / float64(xf.Metrics.Conditionals) * 100
		}
		if xf.Metrics.Methods > 0 {
			fc.FunctionCoverage = float64(xf.Metrics.CoveredMethods) / float64(xf.Metrics.Methods) * 100
		}

		files = append(files, fc)
	}

	linePct, branchPct := computeOverall(files)
	return &Report{
		Files:            files,
		OverallLinePct:   linePct,
		OverallBranchPct: branchPct,
		Format:           "clover",
	}, nil
}
