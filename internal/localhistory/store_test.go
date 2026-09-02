package localhistory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testNow = time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)

func TestRecordScanPersistsPrivacySafeSummary(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "debtdrone", "history.json")
	options := DefaultOptions()
	options.Now = func() time.Time { return testNow }
	options.NewID = func() string { return "d53f9d84-66ef-49d5-bd28-93aaef1e3008" }
	store, err := NewWithOptions(storePath, options)
	require.NoError(t, err)

	record, err := store.RecordScan(context.Background(), RecordInput{
		RepositoryPath: "https://agent:super-secret-token@example.com/private/customer-repository.git?credential=raw",
		StartedAt:      testNow.Add(-2 * time.Minute),
		CompletedAt:    testNow,
		Outcome:        OutcomePartial,
		Summary: Summary{
			Findings: 12, Critical: 1, High: 2, Medium: 4, Low: 5,
			TechnicalDebtHours: 3.5, Warnings: 1, AnalyzerFailures: 1,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "customer-repository.git", record.Repository)
	assert.Equal(t, OutcomePartial, record.Outcome)

	data, err := os.ReadFile(storePath)
	require.NoError(t, err)
	encoded := string(data)
	for _, sensitive := range []string{"agent", "super-secret-token", "private/", "credential=raw"} {
		assert.NotContains(t, encoded, sensitive)
	}
	assert.NotContains(t, encoded, "source")
	assert.Contains(t, encoded, `"technical_debt_hours": 3.5`)

	records, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, record, records[0])

	if runtime.GOOS != "windows" {
		fileInfo, err := os.Stat(storePath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
		directoryInfo, err := os.Stat(filepath.Dir(storePath))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), directoryInfo.Mode().Perm())
	}
}

func TestStorePreservesExistingDirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose portable Unix permission bits")
	}

	directory := filepath.Join(t.TempDir(), "existing")
	require.NoError(t, os.Mkdir(directory, 0o755))
	require.NoError(t, os.Chmod(directory, 0o755))
	store, err := New(filepath.Join(directory, "history.json"))
	require.NoError(t, err)

	_, err = store.List(context.Background())
	require.NoError(t, err)
	info, err := os.Stat(directory)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestRepositoryDisplayName(t *testing.T) {
	longName := strings.Repeat("a", maximumRepositoryNameRunes+10)
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "Unix path", input: "/Users/alice/work/debtdrone", want: "debtdrone"},
		{name: "Windows path", input: `C:\Users\alice\work\debtdrone`, want: "debtdrone"},
		{name: "URL drops credentials", input: "https://user:token@example.com/org/repo.git", want: "repo.git"},
		{name: "URL without path uses host", input: "https://user:token@example.com", want: "example.com"},
		{name: "SSH shorthand drops user and host", input: "git@example.com:org/repo.git", want: "repo.git"},
		{name: "relative URL drops query", input: "example.com/org/repo?token=secret", want: "repo"},
		{name: "relative current directory", input: ".", want: "repository"},
		{name: "control characters", input: "/work/repo\nname", want: "reponame"},
		{name: "bounded", input: "/work/" + longName, want: strings.Repeat("a", maximumRepositoryNameRunes)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, RepositoryDisplayName(test.input))
		})
	}
}

func TestRetentionCountAndNewestFirstOrdering(t *testing.T) {
	options := DefaultOptions()
	options.Now = func() time.Time { return testNow }
	options.Retention = 24 * time.Hour
	options.MaximumRecords = 2
	ids := []string{
		"15198912-9af0-4512-8b23-a949612e057a",
		"b3d8125f-9ba3-4771-acfe-f7be5546ac92",
		"60596433-ca87-4264-b6e3-57b917c41e31",
		"f71839c8-8215-40a0-a86b-a64d6abebd01",
	}
	var idIndex int
	options.NewID = func() string {
		id := ids[idIndex]
		idIndex++
		return id
	}
	store, err := NewWithOptions(filepath.Join(t.TempDir(), "history.json"), options)
	require.NoError(t, err)

	completed := []time.Time{
		testNow.Add(-48 * time.Hour),
		testNow.Add(-3 * time.Hour),
		testNow.Add(-1 * time.Hour),
		testNow.Add(-2 * time.Hour),
	}
	for index, completedAt := range completed {
		_, err := store.RecordScan(context.Background(), validInput(fmt.Sprintf("repo-%d", index), completedAt))
		if index == 0 {
			require.ErrorContains(t, err, "falls outside the configured history bounds")
			continue
		}
		require.NoError(t, err)
	}

	records, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "repo-2", records[0].Repository)
	assert.Equal(t, "repo-3", records[1].Repository)
}

func TestMaximumFileSizeEvictsOldestAndPreservesExistingOnSizeFailure(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "history.json")
	options := DefaultOptions()
	options.Now = func() time.Time { return testNow }
	options.NewID = func() string { return "7856b86e-0e16-4b4a-9867-8f88f4a2f3e9" }
	store, err := NewWithOptions(storePath, options)
	require.NoError(t, err)
	_, err = store.RecordScan(context.Background(), validInput("a", testNow.Add(-time.Minute)))
	require.NoError(t, err)

	original, err := os.ReadFile(storePath)
	require.NoError(t, err)
	limitedOptions := options
	limitedOptions.MaximumBytes = len(original)
	limitedOptions.NewID = func() string { return "8407f548-d0db-44ba-a8e2-99641b375f01" }
	limitedStore, err := NewWithOptions(storePath, limitedOptions)
	require.NoError(t, err)

	_, err = limitedStore.RecordScan(context.Background(), validInput(strings.Repeat("b", maximumRepositoryNameRunes), testNow))
	require.ErrorContains(t, err, "history record exceeds")
	after, readErr := os.ReadFile(storePath)
	require.NoError(t, readErr)
	assert.Equal(t, original, after, "a rejected size-bounded write must preserve the prior history file")

	evictingOptions := options
	evictingOptions.MaximumBytes = len(original) + 180
	evictingOptions.NewID = func() string { return "39a4980a-4d31-4d3d-b3fd-6ac644fbb1f4" }
	evictingStore, err := NewWithOptions(storePath, evictingOptions)
	require.NoError(t, err)
	newest, err := evictingStore.RecordScan(context.Background(), validInput("newest", testNow))
	require.NoError(t, err)
	records, err := evictingStore.List(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, records)
	assert.Equal(t, newest.ID, records[0].ID)
	info, err := os.Stat(storePath)
	require.NoError(t, err)
	assert.LessOrEqual(t, info.Size(), int64(evictingOptions.MaximumBytes))
}

func TestAtomicWritePreservesExistingFileWhenReplacementFails(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "history.json")
	original := []byte("original history\n")
	replacement := []byte("replacement history\n")
	require.NoError(t, os.WriteFile(destination, original, 0o600))

	replaceCalled := false
	err := writeFileAtomicallyWithReplace(destination, replacement, func(temporaryPath, targetPath string) error {
		replaceCalled = true
		assert.Equal(t, destination, targetPath)
		written, readErr := os.ReadFile(temporaryPath)
		require.NoError(t, readErr)
		assert.Equal(t, replacement, written)
		return errors.New("injected replacement failure")
	})

	require.ErrorContains(t, err, "injected replacement failure")
	assert.True(t, replaceCalled)
	after, readErr := os.ReadFile(destination)
	require.NoError(t, readErr)
	assert.Equal(t, original, after)
	temporaryFiles, globErr := filepath.Glob(filepath.Join(directory, ".history-*.tmp"))
	require.NoError(t, globErr)
	assert.Empty(t, temporaryFiles)
}

func TestConcurrentStoresDoNotLoseRecords(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "history.json")
	first, err := New(storePath)
	require.NoError(t, err)
	second, err := New(storePath)
	require.NoError(t, err)

	const total = 30
	base := time.Now().UTC()
	errorsChannel := make(chan error, total)
	var wait sync.WaitGroup
	for index := 0; index < total; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			store := first
			if index%2 == 1 {
				store = second
			}
			_, err := store.RecordScan(context.Background(), validInput(fmt.Sprintf("repo-%d", index), base.Add(time.Duration(index)*time.Millisecond)))
			errorsChannel <- err
		}(index)
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		require.NoError(t, err)
	}

	records, err := first.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, total)
	assert.True(t, records[0].CompletedAt.After(records[len(records)-1].CompletedAt))
}

func TestCorruptAndIncompatibleHistoryFailsWithoutOverwrite(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "empty", content: "", want: "history file is empty"},
		{name: "malformed", content: "{", want: "decode version"},
		{name: "missing version", content: `{"records":[]}`, want: "version is required"},
		{name: "old version", content: `{"version":0,"records":[]}`, want: "no longer supported"},
		{name: "future version with future fields", content: `{"version":2,"future":true}`, want: "upgrade DebtDrone"},
		{name: "unknown current field", content: `{"version":1,"future":true,"records":[]}`, want: "unknown field"},
		{name: "multiple documents", content: `{"version":1,"records":[]} {"version":1}`, want: "one JSON document"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			storePath := filepath.Join(t.TempDir(), "history.json")
			require.NoError(t, os.WriteFile(storePath, []byte(test.content), 0o600))
			store, err := New(storePath)
			require.NoError(t, err)
			_, err = store.List(context.Background())
			require.ErrorContains(t, err, test.want)

			_, recordErr := store.RecordScan(context.Background(), validInput("repo", time.Now().UTC()))
			require.Error(t, recordErr)
			after, readErr := os.ReadFile(storePath)
			require.NoError(t, readErr)
			assert.Equal(t, test.content, string(after))
		})
	}
}

func TestGetDeleteAndClear(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "history.json"))
	require.NoError(t, err)
	first, err := store.RecordScan(context.Background(), validInput("first", time.Now().UTC().Add(-time.Minute)))
	require.NoError(t, err)
	_, err = store.RecordScan(context.Background(), validInput("second", time.Now().UTC()))
	require.NoError(t, err)

	found, ok, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, first, found)

	deleted, err := store.Delete(context.Background(), first.ID)
	require.NoError(t, err)
	assert.True(t, deleted)
	_, ok, err = store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, store.Clear(context.Background()))
	records, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestGeneratedIDCollisionDoesNotOverwriteHistory(t *testing.T) {
	options := DefaultOptions()
	options.NewID = func() string { return "6dd5db33-1b16-4fc7-b57a-9d7fdbf16a21" }
	store, err := NewWithOptions(filepath.Join(t.TempDir(), "history.json"), options)
	require.NoError(t, err)
	_, err = store.RecordScan(context.Background(), validInput("first", time.Now().UTC().Add(-time.Minute)))
	require.NoError(t, err)
	_, err = store.RecordScan(context.Background(), validInput("second", time.Now().UTC()))
	require.ErrorContains(t, err, "already exists")
	records, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "first", records[0].Repository)
}

func TestContextCancellationStopsLockWait(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "history.json")
	store, err := New(storePath)
	require.NoError(t, err)
	require.NoError(t, ensureDirectory(filepath.Dir(storePath)))
	lock, err := acquireHistoryLock(context.Background(), storePath+".lock")
	require.NoError(t, err)
	defer lock.release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.RecordScan(ctx, validInput("repo", time.Now().UTC()))
	require.ErrorIs(t, err, context.Canceled)
}

func TestNewWithOptionsValidation(t *testing.T) {
	valid := DefaultOptions()
	tests := []struct {
		name    string
		path    string
		options Options
		want    string
	}{
		{name: "missing path", path: " ", options: valid, want: "path is required"},
		{name: "retention", path: "history.json", options: func() Options { value := valid; value.Retention = 0; return value }(), want: "retention"},
		{name: "records", path: "history.json", options: func() Options { value := valid; value.MaximumRecords = 0; return value }(), want: "maximum records"},
		{name: "bytes", path: "history.json", options: func() Options { value := valid; value.MaximumBytes = 0; return value }(), want: "maximum bytes"},
		{name: "clock", path: "history.json", options: func() Options { value := valid; value.Now = nil; return value }(), want: "clock"},
		{name: "ID generator", path: "history.json", options: func() Options { value := valid; value.NewID = nil; return value }(), want: "ID generator"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewWithOptions(test.path, test.options)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestRecordScanValidation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*RecordInput)
		want   string
	}{
		{name: "start", change: func(input *RecordInput) { input.StartedAt = time.Time{} }, want: "start time is required"},
		{name: "completion", change: func(input *RecordInput) { input.CompletedAt = time.Time{} }, want: "completion time is required"},
		{name: "time order", change: func(input *RecordInput) { input.StartedAt = input.CompletedAt.Add(time.Minute) }, want: "cannot be before"},
		{name: "outcome", change: func(input *RecordInput) { input.Outcome = "failed" }, want: "history outcome"},
		{name: "negative count", change: func(input *RecordInput) { input.Summary.High = -1 }, want: "high cannot be negative"},
		{name: "severity total", change: func(input *RecordInput) { input.Summary.Findings = 1 }, want: "severity total"},
		{name: "negative debt", change: func(input *RecordInput) { input.Summary.TechnicalDebtHours = -1 }, want: "finite non-negative"},
		{name: "NaN debt", change: func(input *RecordInput) { input.Summary.TechnicalDebtHours = math.NaN() }, want: "finite non-negative"},
	}

	store, err := New(filepath.Join(t.TempDir(), "history.json"))
	require.NoError(t, err)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validInput("repo", time.Now().UTC())
			test.change(&input)
			_, err := store.RecordScan(context.Background(), input)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestPathInUsesUserConfigDirectory(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{name: "Linux XDG", root: filepath.Join("home", "developer", ".config")},
		{name: "macOS Application Support", root: filepath.Join("Users", "developer", "Library", "Application Support")},
		{name: "Windows AppData", root: filepath.Join("Users", "developer", "AppData", "Roaming")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, filepath.Join(test.root, "debtdrone", "history.json"), PathIn(test.root))
		})
	}
}

func validInput(repository string, completedAt time.Time) RecordInput {
	return RecordInput{
		RepositoryPath: repository,
		StartedAt:      completedAt.Add(-time.Minute),
		CompletedAt:    completedAt,
		Outcome:        OutcomeCompleted,
		Summary: Summary{
			Findings: 4, Critical: 1, High: 1, Medium: 1, Low: 1,
			TechnicalDebtHours: 1.5,
		},
	}
}
