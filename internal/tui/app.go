package tui

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

// Build-time variables injected by the linker (e.g. via -ldflags).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// state enumerates every top-level screen in the application.
type state int

const (
	stateMenu     state = iota
	stateScanning       // scan in progress
	stateResults        // scan finished (or history replay)
	stateHistory
	stateConfig
	stateUpdating
	stateHelp // rendered by MenuModel.renderHelp
)

// allCommands is the canonical command registry used by the help screen and
// the menu's autocomplete dropdown.
var allCommands = []struct {
	cmd  string
	desc string
}{
	{"/scan", "Analyze repository for technical debt"},
	{"/update", "Check for and install updates"},
	{"/history", "View past scan results"},
	{"/config", "View or edit configuration"},
	{"/help", "Show available commands"},
	{"/quit", "Exit the application"},
}

// tickMsg is sent on a fixed interval by tickCmd to drive spinner animations
// in ScanModel and UpdateModel.
type tickMsg struct{}

// ─────────────────────────────────────────────────────────────────────────────
// AppModel — the root "router" model
// ─────────────────────────────────────────────────────────────────────────────

// AppModel is the root Bubble Tea model. It owns:
//   - Navigation state (activeState)
//   - Global terminal dimensions (width, height)
//   - The shared scan-history list (historyEntries)
//   - Singleton instances of every child model
//
// Communication contract
//
//	Children NEVER mutate AppModel directly. Instead they return a tea.Cmd
//	whose function yields a router message (NavigateMsg, StartScanMsg, …).
//	AppModel intercepts those messages inside Update and acts on them before
//	any child ever sees them. This keeps all cross-screen orchestration in
//	one place and keeps child models completely self-contained.
type AppModel struct {
	activeState    state
	width, height  int
	historyEntries []historyEntry // shared by ScanModel (write) and HistoryModel (read)

	// Child models are stored as concrete pointer types so we can call
	// both the tea.Model interface and any model-specific helper methods
	// (e.g. GetValue, Start, LoadResults) without type assertions.
	menu    *MenuModel
	scan    *ScanModel
	history *HistoryModel
	config  *ConfigModel
	update  *UpdateModel
}

// NewAppModel constructs and wires up the AppModel with all child models.
func NewAppModel() *AppModel {
	return &AppModel{
		activeState: stateMenu,
		width:       120,
		height:      40,
		menu:        newMenuModel(),
		scan:        newScanModel(),
		history:     newHistoryModel(),
		config:      newConfigModel(),
		update:      newUpdateModel(),
	}
}

// RunTUI is the package-level entry point called by cmd/debtdrone/main.go.
func RunTUI() error {
	_, err := tea.NewProgram(NewAppModel()).Run()
	return err
}

// Init satisfies tea.Model. We fan Init commands from all children so that
// any child that needs to kick off background work can do so immediately.
func (m *AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.menu.Init(),
		m.scan.Init(),
		m.history.Init(),
		m.config.Init(),
		m.update.Init(),
	)
}

// Update is the central dispatcher. It classifies each incoming message into
// one of three categories:
//
//  1. Global messages — WindowSizeMsg and ctrl+c are handled here.
//     WindowSizeMsg is fanned to ALL children so every model pre-computes
//     its layout even while off-screen.
//
//  2. Router messages — NavigateMsg, StartScanMsg, ScanFinishedMsg,
//     LoadHistoryRunMsg, and StartUpdateMsg are intercepted here and are
//     NEVER forwarded to any child. This is the single authoritative place
//     for screen transitions and shared-state mutations.
//
//  3. All other messages — delegated exclusively to the currently-active
//     child via delegateToActive.
func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ── 1. Global ──────────────────────────────────────────────────────────

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Fan resize to every child regardless of which is active. This
		// ensures that ScanModel's split pane, HistoryModel's list height,
		// etc. are always correct when the user switches screens.
		var cmds []tea.Cmd
		for _, child := range []tea.Model{m.menu, m.scan, m.history, m.config, m.update} {
			_, c := child.Update(msg)
			cmds = append(cmds, c)
		}
		return m, tea.Batch(cmds...)

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			fmt.Println("👋 Goodbye!")
			os.Exit(0)
		}
		// All other keys fall through to active-model delegation below.

	// ── 2. Router messages ─────────────────────────────────────────────────

	// NavigateMsg: any child can request a screen change by returning a Cmd
	// that yields this message. AppModel performs the transition here and
	// no child ever needs to know about other children.
	case NavigateMsg:
		return m.navigateTo(msg.State)

	// StartScanMsg: MenuModel parsed a /scan command and resolved the path.
	// AppModel merges in the current config values and kicks off the scan.
	case StartScanMsg:
		maxComplexity := 15
		fmt.Sscanf(m.config.GetValue("Max Complexity"), "%d", &maxComplexity)
		securityScan := m.config.GetValue("Security Scan") == "true"
		outputFormat := m.config.GetValue("Output Format")
		cmd := m.scan.Start(msg.Path, maxComplexity, securityScan, outputFormat)
		m.activeState = stateScanning
		return m, cmd

	// ScanFinishedMsg: ScanModel completed (successfully or not). On success
	// prepend the new run to the shared history and notify HistoryModel so
	// it is ready the next time the user opens /history.
	case ScanFinishedMsg:
		if msg.Err == nil {
			m.historyEntries = append([]historyEntry{msg.Entry}, m.historyEntries...)
			m.history.SetEntries(m.historyEntries)
		}
		m.activeState = stateResults
		return m, nil

	// LoadHistoryRunMsg: HistoryModel wants to replay a past scan as results.
	// Hydrate ScanModel with the historical data then show the results screen.
	case LoadHistoryRunMsg:
		outputFormat := m.config.GetValue("Output Format")
		m.scan.LoadResults(msg.Entry, outputFormat)
		m.activeState = stateResults
		return m, nil

	// StartUpdateMsg: MenuModel parsed a /update command.
	case StartUpdateMsg:
		cmd := m.update.Start()
		m.activeState = stateUpdating
		return m, cmd
	}

	// ── 3. Active-child delegation ─────────────────────────────────────────
	return m.delegateToActive(msg)
}

// navigateTo changes the active screen and performs any per-screen setup that
// cannot live inside the child model itself (because it requires shared state
// such as historyEntries or cross-model dimension data).
func (m *AppModel) navigateTo(s state) (tea.Model, tea.Cmd) {
	m.activeState = s
	switch s {
	case stateHistory:
		// Hydrate HistoryModel with the latest entries before showing it.
		m.history.SetEntries(m.historyEntries)
	case stateConfig:
		// Reset cursor/mode so the screen is always clean on entry.
		m.config.Reset()
	case stateMenu:
		m.menu.Reset()
	case stateHelp:
		m.menu.ShowHelp()
	}
	return m, nil
}

// delegateToActive forwards msg to the currently-active child and discards
// the returned model value. Because every child Update method uses a pointer
// receiver and returns the same receiver pointer, mutations happen in-place —
// there is no need to re-assign the child field. Only the returned Cmd
// matters here.
func (m *AppModel) delegateToActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.activeState {
	case stateMenu, stateHelp:
		_, cmd := m.menu.Update(msg)
		return m, cmd
	case stateScanning, stateResults:
		_, cmd := m.scan.Update(msg)
		return m, cmd
	case stateHistory:
		_, cmd := m.history.Update(msg)
		return m, cmd
	case stateConfig:
		_, cmd := m.config.Update(msg)
		return m, cmd
	case stateUpdating:
		_, cmd := m.update.Update(msg)
		return m, cmd
	}
	return m, nil
}
