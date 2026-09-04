package tui

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/models"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startScanning puts a model into the running phase without executing the scan
// command, so update-loop behaviour can be driven with explicit messages.
func startScanning(t *testing.T, path string) *ScanModel {
	t.Helper()
	model := newScanModel()
	command := model.Start(path, service.ScanOptions{}, scanDisplayOptions{outputFormat: "text"}, false)
	require.NotNil(t, command)
	require.True(t, model.Scanning())
	return model
}

func TestScanStartMovesWorkOffTheUpdateLoop(t *testing.T) {
	model := startScanning(t, "/repo")

	// Start returns immediately with the scan pending; the update loop is never
	// blocked waiting for a result.
	assert.Equal(t, scanRunning, model.phase)
	assert.Equal(t, "/repo", model.scanPath)
	assert.NotNil(t, model.cancel)
	assert.Equal(t, scanRunID(1), model.runID)
}

func TestScanProgressReportsStageAndCompletedAnalyzers(t *testing.T) {
	model := startScanning(t, "/repo")

	_, command := model.Update(scanProgressMsg{runID: model.runID, stage: "Complexity", completed: 0, total: 3})
	require.NotNil(t, command, "the model keeps listening for the next scan message")
	assert.Equal(t, "Complexity", model.stage)
	assert.Equal(t, 0, model.completedAnalyzers)
	assert.Equal(t, 3, model.totalAnalyzers)

	// A finish event carries no stage name; it must advance the count without
	// blanking the stage the reader is looking at.
	model.Update(scanProgressMsg{runID: model.runID, stage: "", completed: 1, total: 3})
	assert.Equal(t, "Complexity", model.stage)
	assert.Equal(t, 1, model.completedAnalyzers)

	rendered := model.renderScanning()
	assert.Contains(t, rendered, "Complexity")
	assert.Contains(t, rendered, "1/3 analyzers")
	assert.NotContains(t, rendered, "%", "progress is reported as analyzers completed, not a synthetic percentage")
}

func TestScanProgressShowsNoBarBeforeTheScannerReportsATotal(t *testing.T) {
	model := startScanning(t, "/repo")

	rendered := model.renderScanning()
	assert.Contains(t, rendered, "Preparing analyzers")
	assert.NotContains(t, rendered, "analyzers completed")
	assert.NotContains(t, rendered, "0/0")
}

func TestScanCompletionDeliversResultsAndFinishesTheRun(t *testing.T) {
	model := startScanning(t, "/repo")
	issue := models.TechnicalDebtIssue{FilePath: "main.go", Severity: "high", Message: "complex function"}

	_, command := model.Update(scanCompleteMsg{
		runID:  model.runID,
		path:   "/repo",
		issues: []models.TechnicalDebtIssue{issue},
	})
	require.NotNil(t, command)

	assert.Equal(t, scanResults, model.phase)
	assert.False(t, model.Scanning())
	assert.Nil(t, model.cancel, "a finished run holds no cancel function")
	assert.NoError(t, model.err)
	require.Len(t, model.issues, 1)

	finished, ok := command().(ScanFinishedMsg)
	require.True(t, ok)
	assert.NoError(t, finished.Err)
	assert.Len(t, finished.Entry.issues, 1)
}

func TestScanFailureKeepsContextForRetryAndTroubleshooting(t *testing.T) {
	model := startScanning(t, "/private/work/customer-repo")
	model.startedAt = time.Now().Add(-3 * time.Second)
	failure := errors.New("repository path is not readable")

	_, command := model.Update(scanCompleteMsg{runID: model.runID, path: "/private/work/customer-repo", err: failure})
	require.NotNil(t, command)

	assert.Equal(t, scanResults, model.phase)
	assert.ErrorIs(t, model.err, failure)

	finished, ok := command().(ScanFinishedMsg)
	require.True(t, ok)
	assert.ErrorIs(t, finished.Err, failure)

	rendered := model.renderResults()
	assert.Contains(t, rendered, "Scan failed")
	assert.Contains(t, rendered, "customer-repo", "the failure names the repository that was scanned")
	assert.Contains(t, rendered, "3s", "the failure reports how long the scan ran")
	assert.Contains(t, rendered, "repository path is not readable")
	assert.Contains(t, rendered, "retry this scan")
}

func TestRetryFromAFailureReusesTheNormalScanLifecycle(t *testing.T) {
	model := startScanning(t, "/repo")
	model.Update(scanCompleteMsg{runID: model.runID, path: "/repo", err: errors.New("boom")})

	_, command := model.handleKey("r")
	require.NotNil(t, command)

	// Retry goes through StartScanMsg so the app rebuilds options from the
	// current configuration, rather than through a second execution path.
	start, ok := command().(StartScanMsg)
	require.True(t, ok)
	assert.Equal(t, "/repo", start.Path)
}

func TestCancellingAScanStopsItAndReturnsToTheDashboard(t *testing.T) {
	model := startScanning(t, "/repo")
	cancelledRun := model.runID

	_, command := model.handleKey("esc")
	require.NotNil(t, command)

	navigate, ok := command().(NavigateMsg)
	require.True(t, ok)
	assert.Equal(t, stateMenu, navigate.State)
	assert.Equal(t, scanIdle, model.phase)
	assert.False(t, model.Scanning())
	assert.Nil(t, model.cancel)
	assert.NotEqual(t, cancelledRun, model.runID, "the cancelled run is retired")
}

func TestACancelledScanCannotDeliverResultsLater(t *testing.T) {
	model := startScanning(t, "/repo")
	cancelledRun := model.runID
	model.handleKey("esc")

	// The scanner may already have been mid-flight when the reader cancelled.
	_, command := model.Update(scanCompleteMsg{
		runID:  cancelledRun,
		path:   "/repo",
		issues: []models.TechnicalDebtIssue{{FilePath: "main.go", Severity: "critical"}},
	})

	assert.Nil(t, command)
	assert.Equal(t, scanIdle, model.phase, "a cancelled run must not push the reader into results")
	assert.Empty(t, model.issues)
}

func TestASecondScanSupersedesTheFirstWithoutRacing(t *testing.T) {
	model := startScanning(t, "/first")
	firstRun := model.runID

	model.Start("/second", service.ScanOptions{}, scanDisplayOptions{outputFormat: "text"}, false)
	secondRun := model.runID
	require.NotEqual(t, firstRun, secondRun)
	assert.Equal(t, "/second", model.scanPath)

	// Results from the superseded scan must not overwrite the active run.
	model.Update(scanCompleteMsg{
		runID:  firstRun,
		path:   "/first",
		issues: []models.TechnicalDebtIssue{{FilePath: "stale.go", Severity: "critical"}},
	})
	assert.Equal(t, scanRunning, model.phase)
	assert.Empty(t, model.issues)
	assert.Equal(t, "/second", model.scanPath)

	// The current run still delivers normally.
	model.Update(scanCompleteMsg{
		runID:  secondRun,
		path:   "/second",
		issues: []models.TechnicalDebtIssue{{FilePath: "fresh.go", Severity: "high"}},
	})
	assert.Equal(t, scanResults, model.phase)
	require.Len(t, model.issues, 1)
	assert.Equal(t, "fresh.go", model.issues[0].FilePath)
}

func TestStartingASecondScanCancelsTheFirstInsteadOfLeavingItRunning(t *testing.T) {
	model := startScanning(t, "/first")

	cancelled := false
	previous := model.cancel
	model.cancel = func() {
		cancelled = true
		previous()
	}

	model.Start("/second", service.ScanOptions{}, scanDisplayOptions{outputFormat: "text"}, false)

	// Retiring the run id alone would hide the old results but leave the
	// superseded scan consuming the machine until it finished on its own.
	assert.True(t, cancelled, "the superseded scan must be cancelled, not merely ignored")
}

func TestQuittingCancelsAnActiveScanAndExitsThroughBubbleTea(t *testing.T) {
	app := NewConfiguredAppModel(localconfig.Defaults())
	app.scan = startScanning(t, "/repo")

	cancelled := false
	previous := app.scan.cancel
	app.scan.cancel = func() {
		cancelled = true
		previous()
	}

	_, command := app.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	require.NotNil(t, command)

	assert.True(t, cancelled, "ctrl+c stops the running scan")
	// Quitting through Bubble Tea lets it restore the terminal, which an
	// os.Exit from inside the update loop would skip.
	_, isQuit := command().(tea.QuitMsg)
	assert.True(t, isQuit, "ctrl+c quits through Bubble Tea rather than exiting the process directly")
}

func TestStaleProgressAndTicksAreIgnored(t *testing.T) {
	model := startScanning(t, "/repo")
	staleRun := model.runID - 1

	_, command := model.Update(scanProgressMsg{runID: staleRun, stage: "Ghost", completed: 9, total: 9})
	assert.Nil(t, command)
	assert.Empty(t, model.stage)
	assert.Zero(t, model.totalAnalyzers)

	_, command = model.Update(scanTickMsg{runID: staleRun})
	assert.Nil(t, command, "a superseded run must not keep a second animation loop alive")

	_, command = model.Update(scanTickMsg{runID: model.runID})
	assert.NotNil(t, command, "the active run keeps animating")
}

func TestTicksStopOnceTheScanLeavesTheRunningPhase(t *testing.T) {
	model := startScanning(t, "/repo")
	model.Update(scanCompleteMsg{runID: model.runID, path: "/repo"})

	_, command := model.Update(scanTickMsg{runID: model.runID})
	assert.Nil(t, command, "the ticker stops when the scan is no longer running")
}

func TestKeysOtherThanCancelAreIgnoredWhileScanning(t *testing.T) {
	model := startScanning(t, "/repo")

	for _, key := range []string{"j", "k", "/", "1", "e", "x", "r"} {
		_, command := model.handleKey(key)
		assert.Nil(t, command, "key %q must not act while a scan is running", key)
		assert.Equal(t, scanRunning, model.phase)
	}
}

func TestCancelIsSafeWhenNoScanIsRunning(t *testing.T) {
	model := newScanModel()
	assert.NotPanics(t, model.Cancel)
	assert.Nil(t, model.cancel)
}

func TestScanDeliversItsCompletionTaggedWithTheRunThatProducedIt(t *testing.T) {
	messages := make(chan tea.Msg, scanMessageBuffer)
	command := startScan(context.Background(), 7, t.TempDir(), service.ScanOptions{}, false, messages)
	require.Nil(t, command(), "starting a scan returns to the update loop immediately")

	deadline := time.After(30 * time.Second)
	for {
		select {
		case msg := <-messages:
			if complete, ok := msg.(scanCompleteMsg); ok {
				assert.Equal(t, scanRunID(7), complete.runID, "messages carry the id of the run that produced them")
				return
			}
		case <-deadline:
			t.Fatal("the scan never reported completion")
		}
	}
}

func TestCancelledScanDoesNotLeakItsGoroutine(t *testing.T) {
	// An unbuffered channel that is never read: the scan goroutine reaches its
	// first send and parks there. Only abandoning that send on cancellation can
	// return the goroutine count to its baseline.
	messages := make(chan tea.Msg)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	baseline := runtime.NumGoroutine()
	command := startScan(ctx, 1, t.TempDir(), service.ScanOptions{}, false, messages)
	require.Nil(t, command())

	// Polled inline rather than through Eventually, which would run its own
	// goroutine and inflate the very count being measured.
	waitForGoroutines := func(predicate func(int) bool) bool {
		for attempt := 0; attempt < 3000; attempt++ {
			if predicate(runtime.NumGoroutine()) {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
		return false
	}

	require.True(t, waitForGoroutines(func(count int) bool { return count > baseline }),
		"the scan goroutine never blocked on its send")

	cancel()
	assert.True(t, waitForGoroutines(func(count int) bool { return count <= baseline }),
		"the cancelled scan left a goroutine blocked on its send")
}

func TestScanMessageListenerStopsOnCancellation(t *testing.T) {
	model := startScanning(t, "/repo")
	listener := listenForScanMessages(model.scanDone, model.scanChan)
	listenerReturned := make(chan tea.Msg, 1)

	go func() {
		listenerReturned <- listener()
	}()

	model.Cancel()
	select {
	case msg := <-listenerReturned:
		assert.Nil(t, msg)
	case <-time.After(time.Second):
		t.Fatal("the scan message listener remained blocked after cancellation")
	}
}
