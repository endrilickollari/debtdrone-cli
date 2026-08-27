package service

import (
	"context"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/scanner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanServiceCoverageDoesNotEnableRepositoryExecution(t *testing.T) {
	var captured scanner.Options
	service := &ScanService{scan: func(_ context.Context, _ string, options scanner.Options) (scanner.Report, error) {
		captured = options
		return scanner.Report{}, nil
	}}

	_, err := service.RunDetailed(context.Background(), "/repo", ScanOptions{Coverage: true}, nil)
	require.NoError(t, err)
	assert.True(t, captured.Coverage.Enabled)
	assert.False(t, captured.Coverage.RunLocalTests)
	assert.Nil(t, captured.Coverage.IsolatedExecutor)
}
