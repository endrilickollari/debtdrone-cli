package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rootWithMCP(run mcpRunFunc) *cobra.Command {
	root := &cobra.Command{Use: "debtdrone", SilenceUsage: true}
	root.AddCommand(newMCPCommand("test-version", run))
	return root
}

func TestRootCommandAdvertisesMCPServer(t *testing.T) {
	root := newRootCmd()

	stdout, stderr, err := executeCommandWithStreams(root, "help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "mcp")
	assert.Contains(t, stdout, "Run the local DebtDrone MCP server over stdio")
	assert.Empty(t, stderr)
}

func TestMCPCommandRequiresRoot(t *testing.T) {
	root := rootWithMCP(func(context.Context, string, string) error {
		t.Fatal("runner must not be called when --root is missing")
		return nil
	})

	_, _, err := executeCommandWithStreams(root, "mcp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag(s) \"root\"")
}

func TestMCPCommandValidatesAndResolvesRoot(t *testing.T) {
	repositoryRoot := t.TempDir()
	workingDirectory, err := os.Getwd()
	require.NoError(t, err)
	relativeRoot, err := filepath.Rel(workingDirectory, repositoryRoot)
	require.NoError(t, err)

	var receivedRoot, receivedVersion string
	runner := func(_ context.Context, root, version string) error {
		receivedRoot = root
		receivedVersion = version
		return nil
	}

	root := rootWithMCP(runner)
	stdout, stderr, err := executeCommandWithStreams(root, "mcp", "--root", relativeRoot)
	require.NoError(t, err)
	assert.Empty(t, stdout, "MCP protocol stdout must not contain command diagnostics")
	assert.Empty(t, stderr)
	assert.Equal(t, filepath.Clean(repositoryRoot), receivedRoot)
	assert.Equal(t, "test-version", receivedVersion)
}

func TestMCPCommandRejectsInvalidRoots(t *testing.T) {
	tests := []struct {
		name string
		root func(*testing.T) string
		want string
	}{
		{
			name: "missing path",
			root: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") },
			want: "inspect MCP root",
		},
		{
			name: "regular file",
			root: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "file")
				require.NoError(t, os.WriteFile(path, []byte("not a repository"), 0o600))
				return path
			},
			want: "is not a directory",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := rootWithMCP(func(context.Context, string, string) error {
				t.Fatal("runner must not be called for an invalid root")
				return nil
			})

			_, _, err := executeCommandWithStreams(root, "mcp", "--root", test.root(t))
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestMCPCommandStopsCleanlyWhenContextIsCancelled(t *testing.T) {
	started := make(chan struct{})
	runner := func(ctx context.Context, _, _ string) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	root := rootWithMCP(runner)
	root.SetArgs([]string{"mcp", "--root", t.TempDir()})

	done := make(chan error, 1)
	go func() {
		done <- root.ExecuteContext(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("MCP runner did not start")
	}
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("MCP command did not stop after cancellation")
	}
}

func TestMCPCommandReturnsServerErrors(t *testing.T) {
	runnerErr := errors.New("transport failed")
	root := rootWithMCP(func(context.Context, string, string) error { return runnerErr })

	_, _, err := executeCommandWithStreams(root, "mcp", "--root", t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, runnerErr)
	assert.Contains(t, err.Error(), "MCP server failed")
}
