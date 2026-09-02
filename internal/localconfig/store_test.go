package localconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreSetAndUnsetPreserveComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := `# DebtDrone settings
version: 1
scan:
  # Keep this threshold explanation.
  max_complexity: 15 # inline threshold
  output_format: text
`
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	store, err := NewStore(path)
	require.NoError(t, err)

	require.NoError(t, store.Set(context.Background(), KeyMaxComplexity, "20"))
	removed, err := store.Unset(context.Background(), KeyOutputFormat)
	require.NoError(t, err)
	assert.True(t, removed)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "# DebtDrone settings")
	assert.Contains(t, string(data), "# Keep this threshold explanation.")
	assert.Contains(t, string(data), "# inline threshold")
	assert.Contains(t, string(data), "max_complexity: 20")
	assert.NotContains(t, string(data), "output_format")
	overrides, found, err := store.Load()
	require.NoError(t, err)
	assert.True(t, found)
	require.NotNil(t, overrides.MaxComplexity)
	assert.Equal(t, 20, *overrides.MaxComplexity)
	assert.Nil(t, overrides.OutputFormat)
}

func TestStoreSetPopulatesValidNullSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("version: 1\nscan:\n"), 0o600))
	store, err := NewStore(path)
	require.NoError(t, err)

	require.NoError(t, store.Set(context.Background(), KeyMaxComplexity, "20"))
	overrides, found, err := store.Load()
	require.NoError(t, err)
	assert.True(t, found)
	require.NotNil(t, overrides.MaxComplexity)
	assert.Equal(t, 20, *overrides.MaxComplexity)
}

func TestStoreCreatesPrivateFileWithoutChangingExistingDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "existing")
	require.NoError(t, os.Mkdir(directory, 0o755))
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Chmod(directory, 0o755))
	}
	path := filepath.Join(directory, "config.yaml")
	store, err := NewStore(path)
	require.NoError(t, err)
	require.NoError(t, store.Set(context.Background(), KeyHistoryEnabled, "false"))

	if runtime.GOOS != "windows" {
		fileInfo, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
		directoryInfo, err := os.Stat(directory)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), directoryInfo.Mode().Perm())
	}
}

func TestStoreCreatesPrivateDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose portable Unix permission bits")
	}
	directory := filepath.Join(t.TempDir(), "missing", "debtdrone")
	store, err := NewStore(filepath.Join(directory, "config.yaml"))
	require.NoError(t, err)
	require.NoError(t, store.Set(context.Background(), KeyCoverage, "true"))

	info, err := os.Stat(directory)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestStoreDirectoryErrorIncludesRecoveryGuidance(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, []byte("blocked"), 0o600))
	store, err := NewStore(filepath.Join(blocker, "config.yaml"))
	require.NoError(t, err)

	err = store.Set(context.Background(), KeyCoverage, "true")
	require.ErrorContains(t, err, "check directory permissions")
}

func TestStoreRejectsUnknownDataWithoutOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := "version: 1\nfuture:\n  enabled: true\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	store, err := NewStore(path)
	require.NoError(t, err)

	err = store.Set(context.Background(), KeyMaxComplexity, "20")
	require.ErrorContains(t, err, "field future not found")
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, original, string(after))
}

func TestAtomicConfigWritePreservesExistingFileWhenReplacementFails(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "config.yaml")
	original := []byte("version: 1\n")
	replacement := []byte("version: 1\nscan:\n  max_complexity: 20\n")
	require.NoError(t, os.WriteFile(destination, original, 0o600))

	err := writeConfigAtomicallyWithReplace(destination, replacement, func(temporaryPath, targetPath string) error {
		assert.Equal(t, destination, targetPath)
		written, readErr := os.ReadFile(temporaryPath)
		require.NoError(t, readErr)
		assert.Equal(t, replacement, written)
		return errors.New("injected replacement failure")
	})
	require.ErrorContains(t, err, "injected replacement failure")
	after, readErr := os.ReadFile(destination)
	require.NoError(t, readErr)
	assert.Equal(t, original, after)
	temporaryFiles, globErr := filepath.Glob(filepath.Join(directory, ".config-*.tmp"))
	require.NoError(t, globErr)
	assert.Empty(t, temporaryFiles)
}

func TestConcurrentStoresPreserveIndependentUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	first, err := NewStore(path)
	require.NoError(t, err)
	second, err := NewStore(path)
	require.NoError(t, err)

	start := make(chan struct{})
	errorsChannel := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		errorsChannel <- first.Set(context.Background(), KeyOutputFormat, "json")
	}()
	go func() {
		defer wait.Done()
		<-start
		errorsChannel <- second.Set(context.Background(), KeyMaxComplexity, "25")
	}()
	close(start)
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		require.NoError(t, err)
	}

	overrides, found, err := first.Load()
	require.NoError(t, err)
	assert.True(t, found)
	require.NotNil(t, overrides.OutputFormat)
	require.NotNil(t, overrides.MaxComplexity)
	assert.Equal(t, "json", *overrides.OutputFormat)
	assert.Equal(t, 25, *overrides.MaxComplexity)
}
