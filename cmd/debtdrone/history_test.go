package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/endrilickollari/debtdrone-cli/v2/internal/localhistory"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newHistoryTestRoot(store historyStore) *cobra.Command {
	root := &cobra.Command{Use: "debtdrone", SilenceUsage: true}
	root.AddCommand(newHistoryCommand(func() (historyStore, error) { return store, nil }))
	return root
}

func newHistoryTestStore(t *testing.T) (*localhistory.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.json")
	store, err := localhistory.New(path)
	require.NoError(t, err)
	return store, path
}

func addHistoryRecord(t *testing.T, store *localhistory.Store, repository string, completedAt time.Time) localhistory.Record {
	t.Helper()
	record, err := store.RecordScan(t.Context(), localhistory.RecordInput{
		RepositoryPath: repository,
		StartedAt:      completedAt.Add(-time.Minute),
		CompletedAt:    completedAt,
		Outcome:        localhistory.OutcomeCompleted,
		Summary: localhistory.Summary{
			Findings: 6, Critical: 1, High: 2, Medium: 2, Low: 1,
			TechnicalDebtHours: 3.25, Warnings: 1,
		},
	})
	require.NoError(t, err)
	return record
}

func TestHistoryListEmptyAndMachineReadable(t *testing.T) {
	store, _ := newHistoryTestStore(t)

	output, err := executeCommand(newHistoryTestRoot(store), "history", "list")
	require.NoError(t, err)
	assert.Contains(t, output, "No local scan history found.")

	output, err = executeCommand(newHistoryTestRoot(store), "history", "list", "--format", "json")
	require.NoError(t, err)
	assert.JSONEq(t, `[]`, output)
}

func TestHistoryListIsNewestFirstAndBounded(t *testing.T) {
	store, _ := newHistoryTestStore(t)
	now := time.Now().UTC()
	older := addHistoryRecord(t, store, "/work/older-repo", now.Add(-time.Hour))
	newer := addHistoryRecord(t, store, "/work/newer-repo", now)

	output, err := executeCommand(newHistoryTestRoot(store), "history", "list", "--format", "json", "--limit", "1")
	require.NoError(t, err)
	var records []localhistory.Record
	require.NoError(t, json.Unmarshal([]byte(output), &records))
	require.Len(t, records, 1)
	assert.Equal(t, newer.ID, records[0].ID)
	assert.NotEqual(t, older.ID, records[0].ID)

	output, err = executeCommand(newHistoryTestRoot(store), "history", "--format", "text", "--limit", "2")
	require.NoError(t, err)
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, newer.ID)
	assert.Contains(t, output, older.ID)
	assert.Less(t, strings.Index(output, newer.ID), strings.Index(output, older.ID))
}

func TestHistoryListRejectsInvalidBoundsAndFormat(t *testing.T) {
	store, _ := newHistoryTestStore(t)

	_, err := executeCommand(newHistoryTestRoot(store), "history", "list", "--limit", "0")
	require.ErrorContains(t, err, "history limit must be between 1 and 200")

	_, err = executeCommand(newHistoryTestRoot(store), "history", "list", "--format", "yaml")
	require.ErrorContains(t, err, `invalid history format "yaml"`)
}

func TestHistoryShowTextJSONAndMissingRecord(t *testing.T) {
	store, _ := newHistoryTestStore(t)
	record := addHistoryRecord(t, store, "/work/customer-repo", time.Now().UTC())

	output, err := executeCommand(newHistoryTestRoot(store), "history", "show", record.ID)
	require.NoError(t, err)
	assert.Contains(t, output, record.ID)
	assert.Contains(t, output, "customer-repo")
	assert.Contains(t, output, "3.25")

	output, err = executeCommand(newHistoryTestRoot(store), "history", "show", record.ID, "--format", "json")
	require.NoError(t, err)
	var decoded localhistory.Record
	require.NoError(t, json.Unmarshal([]byte(output), &decoded))
	assert.Equal(t, record, decoded)

	_, err = executeCommand(newHistoryTestRoot(store), "history", "show", "44bb97be-e655-4da5-99e5-834991994c2e")
	require.ErrorContains(t, err, "was not found")

	_, err = executeCommand(newHistoryTestRoot(store), "history", "show", "not-a-uuid")
	require.ErrorContains(t, err, "invalid history ID")
}

func TestHistoryDeleteRemovesOnlyRequestedRecord(t *testing.T) {
	store, _ := newHistoryTestStore(t)
	now := time.Now().UTC()
	removed := addHistoryRecord(t, store, "/work/remove-me", now.Add(-time.Minute))
	kept := addHistoryRecord(t, store, "/work/keep-me", now)

	output, err := executeCommand(newHistoryTestRoot(store), "history", "delete", removed.ID)
	require.NoError(t, err)
	assert.Contains(t, output, removed.ID)
	records, err := store.List(t.Context())
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, kept.ID, records[0].ID)

	_, err = executeCommand(newHistoryTestRoot(store), "history", "delete", removed.ID)
	require.ErrorContains(t, err, "was not found")
}

func TestHistoryClearRequiresConfirmationOrForce(t *testing.T) {
	t.Run("rejecting confirmation preserves history", func(t *testing.T) {
		store, _ := newHistoryTestStore(t)
		addHistoryRecord(t, store, "/work/repo", time.Now().UTC())
		root := newHistoryTestRoot(store)
		root.SetIn(strings.NewReader("no\n"))

		output, stderr, err := executeCommandWithStreams(root, "history", "clear")
		require.NoError(t, err)
		assert.Contains(t, stderr, `Type "yes"`)
		assert.Contains(t, output, "cancelled")
		records, err := store.List(t.Context())
		require.NoError(t, err)
		assert.Len(t, records, 1)
	})

	t.Run("confirmation clears history", func(t *testing.T) {
		store, _ := newHistoryTestStore(t)
		addHistoryRecord(t, store, "/work/repo", time.Now().UTC())
		root := newHistoryTestRoot(store)
		root.SetIn(strings.NewReader("yes\n"))

		output, _, err := executeCommandWithStreams(root, "history", "clear")
		require.NoError(t, err)
		assert.Contains(t, output, "history cleared")
		records, err := store.List(t.Context())
		require.NoError(t, err)
		assert.Empty(t, records)
	})

	t.Run("force clears without reading input", func(t *testing.T) {
		store, _ := newHistoryTestStore(t)
		addHistoryRecord(t, store, "/work/repo", time.Now().UTC())

		output, _, err := executeCommandWithStreams(newHistoryTestRoot(store), "history", "clear", "--force")
		require.NoError(t, err)
		assert.Contains(t, output, "history cleared")
		records, err := store.List(t.Context())
		require.NoError(t, err)
		assert.Empty(t, records)
	})
}

func TestHistoryCommandsReportCorruptStore(t *testing.T) {
	store, path := newHistoryTestStore(t)
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"records":[`), 0o600))

	_, err := executeCommand(newHistoryTestRoot(store), "history", "list")
	require.ErrorContains(t, err, "invalid history file")
	assert.ErrorContains(t, err, "move it aside")
}
