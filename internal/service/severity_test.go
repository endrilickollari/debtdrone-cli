package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeverityOrderIsMostSevereFirstAndImmutable(t *testing.T) {
	order := SeverityOrder()
	require.Equal(t, []string{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}, order)

	order[0] = "tampered"
	assert.Equal(t, SeverityCritical, SeverityOrder()[0], "callers must not be able to reorder the canonical vocabulary")
}

func TestSeverityRankOrdersKnownLevelsAndSortsUnknownLast(t *testing.T) {
	assert.Less(t, SeverityRank("critical"), SeverityRank("high"))
	assert.Less(t, SeverityRank("high"), SeverityRank("medium"))
	assert.Less(t, SeverityRank("medium"), SeverityRank("low"))
	assert.Less(t, SeverityRank("low"), SeverityRank("informational"))
	assert.Equal(t, SeverityRank("unknown-a"), SeverityRank("unknown-b"))
}

func TestSeverityRankNormalizesCasingAndPadding(t *testing.T) {
	assert.Equal(t, SeverityRank("critical"), SeverityRank("  CRITICAL "))
	assert.Equal(t, SeverityCritical, NormalizeSeverity(" Critical "))
}
