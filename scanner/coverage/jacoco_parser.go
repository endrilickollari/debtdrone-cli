package coverage

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
)

// JaCoCoParser parses JaCoCo XML report files.
// Used by Java and Kotlin projects built with Maven or Gradle.
type JaCoCoParser struct{}

func (p *JaCoCoParser) CanParse(path string) bool {
	base := filepath.Base(path)
	return base == "jacoco.xml" || base == "jacocoTestReport.xml"
}

func (p *JaCoCoParser) Parse(path string) (*Report, error) {
	data, err := readArtifact(path)
	if err != nil {
		return nil, fmt.Errorf("jacoco: %w", err)
	}

	type xmlCounter struct {
		Type    string `xml:"type,attr"`
		Missed  int    `xml:"missed,attr"`
		Covered int    `xml:"covered,attr"`
	}
	type xmlLine struct {
		Nr int `xml:"nr,attr"`
		Mi int `xml:"mi,attr"` // missed instructions
		Ci int `xml:"ci,attr"` // covered instructions
		Mb int `xml:"mb,attr"` // missed branches
		Cb int `xml:"cb,attr"` // covered branches
	}
	type xmlSourceFile struct {
		Name     string       `xml:"name,attr"`
		Lines    []xmlLine    `xml:"line"`
		Counters []xmlCounter `xml:"counter"`
	}
	type xmlPackage struct {
		Name        string          `xml:"name,attr"`
		SourceFiles []xmlSourceFile `xml:"sourcefile"`
	}
	type xmlReport struct {
		XMLName  xml.Name     `xml:"report"`
		Packages []xmlPackage `xml:"package"`
	}

	var report xmlReport
	if err := xml.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("jacoco: %w", err)
	}

	var files []FileCoverage
	for _, pkg := range report.Packages {
		pkgPath := strings.ReplaceAll(pkg.Name, ".", "/")
		for _, sf := range pkg.SourceFiles {
			fc := FileCoverage{
				Path:             normalizeFilePath(pkgPath + "/" + sf.Name),
				BranchCoverage:   -1,
				FunctionCoverage: -1,
			}

			// Use per-line data when available (more precise than summary counters).
			if len(sf.Lines) > 0 {
				for _, l := range sf.Lines {
					// A line is covered if it has at least one covered instruction.
					fc.LinesTotal++
					if l.Ci > 0 {
						fc.LinesCovered++
					} else {
						fc.UncoveredLines = append(fc.UncoveredLines, l.Nr)
					}
				}
			}

			// Extract branch and method coverage from counter summary.
			for _, c := range sf.Counters {
				total := c.Missed + c.Covered
				if total == 0 {
					continue
				}
				switch c.Type {
				case "LINE":
					// Only use summary if we had no per-line data.
					if len(sf.Lines) == 0 {
						fc.LinesTotal = total
						fc.LinesCovered = c.Covered
					}
				case "BRANCH":
					fc.BranchCoverage = float64(c.Covered) / float64(total) * 100
				case "METHOD":
					fc.FunctionCoverage = float64(c.Covered) / float64(total) * 100
				}
			}

			if fc.LinesTotal > 0 {
				fc.LineCoverage = float64(fc.LinesCovered) / float64(fc.LinesTotal) * 100
			}
			files = append(files, fc)
		}
	}

	linePct, branchPct := computeOverall(files)
	return &Report{
		Files:            files,
		OverallLinePct:   linePct,
		OverallBranchPct: branchPct,
		Format:           "jacoco",
	}, nil
}
