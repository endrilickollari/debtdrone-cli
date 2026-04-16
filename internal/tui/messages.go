package tui

// NavigateMsg requests a top-level screen change.
type NavigateMsg struct{ State state }

// StartScanMsg is dispatched by MenuModel after the user runs /scan.
type StartScanMsg struct{ Path string }

// ScanFinishedMsg is emitted by ScanModel after a scan attempt completes.
type ScanFinishedMsg struct {
	Entry historyEntry
	Err   error
}

// LoadHistoryRunMsg is dispatched by HistoryModel when a past run is selected.
type LoadHistoryRunMsg struct{ Entry historyEntry }

// StartUpdateMsg is dispatched by MenuModel when the user runs /update.
type StartUpdateMsg struct{}
