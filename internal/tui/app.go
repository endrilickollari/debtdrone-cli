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

// state enumerates the top-level screens.
type state int

const (
	stateMenu     state = iota
	stateScanning
	stateResults
	stateHistory
	stateConfig
	stateUpdating
	stateHelp
)

// allCommands is the command registry.
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

type tickMsg struct{}

// AppModel is the root Bubble Tea model.
type AppModel struct {
	activeState    state
	width, height  int
	historyEntries []historyEntry

	menu    *MenuModel
	scan    *ScanModel
	history *HistoryModel
	config  *ConfigModel
	update  *UpdateModel
}

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

func RunTUI() error {
	_, err := tea.NewProgram(NewAppModel()).Run()
	return err
}

func (m *AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.menu.Init(),
		m.scan.Init(),
		m.history.Init(),
		m.config.Init(),
		m.update.Init(),
	)
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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

	case NavigateMsg:
		return m.navigateTo(msg.State)

	case StartScanMsg:
		maxComplexity := 15
		fmt.Sscanf(m.config.GetValue("Max Complexity"), "%d", &maxComplexity)
		securityScan := m.config.GetValue("Security Scan") == "true"
		outputFormat := m.config.GetValue("Output Format")
		cmd := m.scan.Start(msg.Path, maxComplexity, securityScan, outputFormat)
		m.activeState = stateScanning
		return m, cmd

	case ScanFinishedMsg:
		if msg.Err == nil {
			m.historyEntries = append([]historyEntry{msg.Entry}, m.historyEntries...)
			m.history.SetEntries(m.historyEntries)
		}
		m.activeState = stateResults
		return m, nil

	case LoadHistoryRunMsg:
		outputFormat := m.config.GetValue("Output Format")
		m.scan.LoadResults(msg.Entry, outputFormat)
		m.activeState = stateResults
		return m, nil

	case StartUpdateMsg:
		cmd := m.update.Start()
		m.activeState = stateUpdating
		return m, cmd
	}

	return m.delegateToActive(msg)
}

func (m *AppModel) navigateTo(s state) (tea.Model, tea.Cmd) {
	m.activeState = s
	switch s {
	case stateHistory:
		m.history.SetEntries(m.historyEntries)
	case stateConfig:
		m.config.Reset()
	case stateMenu:
		m.menu.Reset()
	case stateHelp:
		m.menu.ShowHelp()
	}
	return m, nil
}

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
