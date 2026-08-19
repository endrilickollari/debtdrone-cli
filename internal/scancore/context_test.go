package scancore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTargetFilesPreservesExplicitEmptyIncrementalSelection(t *testing.T) {
	ctx := WithTargetFiles(context.Background(), []string{}, true)
	files, incremental, ok := TargetFiles(ctx)
	require.True(t, ok)
	require.True(t, incremental)
	require.Empty(t, files)
}

func TestTargetFilesCopiesInput(t *testing.T) {
	input := []string{"/main.go"}
	ctx := WithTargetFiles(context.Background(), input, false)
	input[0] = "/changed.go"

	files, incremental, ok := TargetFiles(ctx)
	require.True(t, ok)
	require.False(t, incremental)
	require.Equal(t, []string{"/main.go"}, files)
}
