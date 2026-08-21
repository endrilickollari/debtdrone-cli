package coverage

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
)

// CoberturaParser parses Cobertura XML files.
// Used by: Python (coverage.py --xml), C# (Coverlet), Rust (cargo-tarpaulin),
// PHP (PHPUnit with Cobertura reporter), Java/Kotlin (some plugins).
type CoberturaParser struct{}

func (p *CoberturaParser) CanParse(path string) bool {
	base := filepath.Base(path)
	return base == "coverage.xml" ||
		base == "cobertura.xml" ||
		base == "coverage.cobertura.xml" ||
		base == "tarpaulin-report.xml"
}

func (p *CoberturaParser) Parse(path string) (*Report, error) {
	data, err := readArtifact(path)
	if err != nil {
		return nil, fmt.Errorf("cobertura: %w", err)
	}

	// Cobertura schema
	type xmlLine struct {
		Number int `xml:"number,attr"`
		Hits   int `xml:"hits,attr"`
	}
	type xmlMethod struct {
		Lines []xmlLine `xml:"lines>line"`
	}
	type xmlClass struct {
		Filename   string      `xml:"filename,attr"`
		LineRate   float64     `xml:"line-rate,attr"`
		BranchRate float64     `xml:"branch-rate,attr"`
		Lines      []xmlLine   `xml:"lines>line"`
		Methods    []xmlMethod `xml:"methods>method"`
	}
	type xmlPackage struct {
		Classes []xmlClass `xml:"classes>class"`
	}
	type xmlCoverage struct {
		XMLName  xml.Name     `xml:"coverage"`
		Packages []xmlPackage `xml:"packages>package"`
	}

	var cov xmlCoverage
	if err := xml.Unmarshal(data, &cov); err != nil {
		return nil, fmt.Errorf("cobertura: %w", err)
	}

	// Merge classes that share the same filename (some tools emit multiple
	// class nodes per file when a file has multiple classes).
	type fileAgg struct {
		lineHits    map[int]int
		branchRates []float64
	}
	agg := make(map[string]*fileAgg)

	for _, pkg := range cov.Packages {
		for _, cls := range pkg.Classes {
			fname := filepath.FromSlash(cls.Filename)
			if _, ok := agg[fname]; !ok {
				agg[fname] = &fileAgg{lineHits: make(map[int]int)}
			}
			fa := agg[fname]

			// Collect all lines from the class-level list and its methods.
			lines := append([]xmlLine(nil), cls.Lines...)
			for _, m := range cls.Methods {
				lines = append(lines, m.Lines...)
			}
			for _, l := range lines {
				if current, exists := fa.lineHits[l.Number]; !exists || l.Hits > current {
					fa.lineHits[l.Number] = l.Hits
				}
			}

			if cls.BranchRate >= 0 {
				fa.branchRates = append(fa.branchRates, cls.BranchRate*100)
			}
		}
	}

	files := make([]FileCoverage, 0, len(agg))
	for fname, fa := range agg {
		fc := FileCoverage{
			Path:             normalizeFilePath(fname),
			LinesTotal:       len(fa.lineHits),
			BranchCoverage:   -1,
			FunctionCoverage: -1,
		}
		for line, hits := range fa.lineHits {
			if hits > 0 {
				fc.LinesCovered++
			} else {
				fc.UncoveredLines = append(fc.UncoveredLines, line)
			}
		}
		if fc.LinesTotal > 0 {
			fc.LineCoverage = float64(fc.LinesCovered) / float64(fc.LinesTotal) * 100
		}
		if len(fa.branchRates) > 0 {
			var sum float64
			for _, r := range fa.branchRates {
				sum += r
			}
			fc.BranchCoverage = sum / float64(len(fa.branchRates))
		}
		files = append(files, fc)
	}

	linePct, branchPct := computeOverall(files)
	return &Report{
		Files:            files,
		OverallLinePct:   linePct,
		OverallBranchPct: branchPct,
		Format:           "cobertura",
	}, nil
}

// normalizeFilePath converts backslashes and ensures a leading slash,
// matching the convention used by other analyzers.
func normalizeFilePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}
