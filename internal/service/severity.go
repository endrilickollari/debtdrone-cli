package service

import "strings"

// Severity levels emitted by the canonical scanner, ordered from most to least
// severe. Presentation layers order and group findings through this list so the
// severity vocabulary stays defined in one place.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
)

var severityOrder = []string{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}

// SeverityOrder returns the known severities, most severe first. The returned
// slice is a copy, so callers cannot reorder the canonical vocabulary.
func SeverityOrder() []string {
	return append([]string(nil), severityOrder...)
}

// NormalizeSeverity maps a scanner severity onto its canonical spelling. Values
// the scanner does not define are returned lowercased and unchanged so they
// remain visible rather than being silently reclassified.
func NormalizeSeverity(severity string) string {
	return strings.ToLower(strings.TrimSpace(severity))
}

// SeverityRank orders a severity for sorting. Lower ranks are more severe;
// unknown severities sort after every known level.
func SeverityRank(severity string) int {
	normalized := NormalizeSeverity(severity)
	for rank, known := range severityOrder {
		if normalized == known {
			return rank
		}
	}
	return len(severityOrder)
}
