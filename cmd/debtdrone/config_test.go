package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newConfigTestStore(t *testing.T) (*localconfig.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "debtdrone", "config.yaml")
	store, err := localconfig.NewStore(path)
	require.NoError(t, err)
	return store, path
}

func newConfigTestRoot(store configStore, environment map[string]string) *cobra.Command {
	root := &cobra.Command{Use: "debtdrone", SilenceUsage: true}
	root.AddCommand(newConfigCommand(
		func() (configStore, error) { return store, nil },
		func() map[string]string { return environment },
	))
	return root
}

func TestConfigListReportsEffectiveValuesAndSources(t *testing.T) {
	store, _ := newConfigTestStore(t)
	require.NoError(t, store.Set(t.Context(), localconfig.KeyMaxComplexity, "20"))
	environment := map[string]string{
		"DEBTDRONE_MAX_COMPLEXITY": "30",
		"UNRELATED_API_TOKEN":      "super-secret-value",
	}

	output, err := executeCommand(newConfigTestRoot(store, environment), "config", "list")
	require.NoError(t, err)
	assert.Contains(t, output, "KEY")
	assert.Contains(t, output, "SOURCE")
	assert.Contains(t, output, "scan.max_complexity")
	assert.Contains(t, output, "30")
	assert.Contains(t, output, "environment")
	assert.NotContains(t, output, "super-secret-value")

	output, err = executeCommand(newConfigTestRoot(store, environment), "config", "list", "--format", "json")
	require.NoError(t, err)
	var items []configOutput
	require.NoError(t, json.Unmarshal([]byte(output), &items))
	require.Len(t, items, len(localconfig.Definitions()))
	for _, item := range items {
		if item.Key == localconfig.KeyMaxComplexity {
			assert.Equal(t, "30", item.Value)
			assert.Equal(t, localconfig.SourceEnvironment, item.Source)
			return
		}
	}
	t.Fatal("scan.max_complexity missing from config list")
}

func TestConfigGetSetAndUnsetUseIsolatedFile(t *testing.T) {
	store, path := newConfigTestStore(t)

	output, err := executeCommand(newConfigTestRoot(store, nil), "config", "set", "scan.max_complexity", "25")
	require.NoError(t, err)
	assert.Contains(t, output, path)
	overrides, found, err := store.Load()
	require.NoError(t, err)
	assert.True(t, found)
	require.NotNil(t, overrides.MaxComplexity)
	assert.Equal(t, 25, *overrides.MaxComplexity)

	output, err = executeCommand(newConfigTestRoot(store, nil), "config", "get", "scan.max_complexity")
	require.NoError(t, err)
	assert.Equal(t, "25\n", output)

	output, err = executeCommand(newConfigTestRoot(store, nil), "config", "get", "scan.max_complexity", "--format", "json")
	require.NoError(t, err)
	var item configOutput
	require.NoError(t, json.Unmarshal([]byte(output), &item))
	assert.Equal(t, localconfig.KeyMaxComplexity, item.Key)
	assert.Equal(t, "25", item.Value)
	assert.Equal(t, localconfig.SourceConfigFile, item.Source)

	output, err = executeCommand(newConfigTestRoot(store, nil), "config", "unset", "scan.max_complexity")
	require.NoError(t, err)
	assert.Contains(t, output, "Unset scan.max_complexity")
	overrides, found, err = store.Load()
	require.NoError(t, err)
	assert.True(t, found)
	assert.Nil(t, overrides.MaxComplexity)

	output, err = executeCommand(newConfigTestRoot(store, nil), "config", "get", "scan.max_complexity")
	require.NoError(t, err)
	assert.Equal(t, "15\n", output)

	output, err = executeCommand(newConfigTestRoot(store, nil), "config", "unset", "scan.max_complexity")
	require.NoError(t, err)
	assert.Contains(t, output, "No config-file value")
}

func TestConfigCommandsRejectInvalidInputWithoutWriting(t *testing.T) {
	store, path := newConfigTestStore(t)

	_, err := executeCommand(newConfigTestRoot(store, nil), "config", "set", "scan.max_complexity", "zero")
	require.ErrorContains(t, err, "must be an integer")
	_, statErr := os.Stat(path)
	assert.ErrorIs(t, statErr, os.ErrNotExist)

	_, err = executeCommand(newConfigTestRoot(store, nil), "config", "get", "database.password")
	require.ErrorContains(t, err, "unknown configuration key")
	assert.NotContains(t, err.Error(), "super-secret")

	_, err = executeCommand(newConfigTestRoot(store, nil), "config", "list", "--format", "yaml")
	require.ErrorContains(t, err, "invalid config format")
}

func TestConfigCommandsReportMalformedFileRecovery(t *testing.T) {
	store, path := newConfigTestStore(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	original := []byte("version: 1\nscan:\n  unknown: true\n")
	require.NoError(t, os.WriteFile(path, original, 0o600))

	_, err := executeCommand(newConfigTestRoot(store, nil), "config", "list")
	require.ErrorContains(t, err, "invalid configuration")
	assert.ErrorContains(t, err, "fix it or move it aside")

	_, err = executeCommand(newConfigTestRoot(store, nil), "config", "set", "scan.max_complexity", "20")
	require.ErrorContains(t, err, "invalid configuration")
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, original, after)
}

func TestConfigUnknownEnvironmentDoesNotExposeItsValue(t *testing.T) {
	store, _ := newConfigTestStore(t)
	_, err := executeCommand(newConfigTestRoot(store, map[string]string{
		"DEBTDRONE_PRIVATE_TOKEN": "super-secret-value",
	}), "config", "list")
	require.ErrorContains(t, err, "DEBTDRONE_PRIVATE_TOKEN")
	assert.NotContains(t, err.Error(), "super-secret-value")
}
