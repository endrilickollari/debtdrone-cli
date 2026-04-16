package tui

// This file defines all cross-model message types used for event-driven
// communication between child models and the AppModel router.
//
// Design rule: NO child model may import another child model or call methods
// on AppModel directly. Instead, a child signals intent by returning a
// tea.Cmd whose function yields one of the messages below. AppModel receives
// these messages in its Update loop BEFORE any child sees them, allowing it
// to orchestrate screen transitions and shared-state mutations in one place.

// NavigateMsg requests a top-level screen change. Any child model can return
// a command that yields this message (e.g. pressing esc/q to go back).
// AppModel intercepts it, updates activeState, and does NOT forward it.
type NavigateMsg struct{ State state }

// StartScanMsg is dispatched by MenuModel after the user runs /scan <path>.
// AppModel intercepts it, reads the current config values (max complexity,
// security scan, output format) from ConfigModel, and calls ScanModel.Start.
type StartScanMsg struct{ Path string }

// ScanFinishedMsg is emitted by ScanModel after a scan attempt completes —
// successfully or with an error. On success, AppModel prepends the new run
// to the shared historyEntries slice and notifies HistoryModel. In both
// cases AppModel transitions activeState to stateResults.
type ScanFinishedMsg struct {
	Entry historyEntry
	Err   error // non-nil on failure; AppModel skips adding to history
}

// LoadHistoryRunMsg is dispatched by HistoryModel when the user selects a
// past run and presses Enter. AppModel intercepts it, calls
// ScanModel.LoadResults to hydrate the results pane with the historical data,
// and transitions to stateResults — bypassing a live scan entirely.
type LoadHistoryRunMsg struct{ Entry historyEntry }

// StartUpdateMsg is dispatched by MenuModel when the user runs /update.
// AppModel intercepts it, calls UpdateModel.Start, and transitions to
// stateUpdating.
type StartUpdateMsg struct{}
