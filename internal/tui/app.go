package tui

import (
	"fmt"
	"os"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/localconfig"
	"github.com/endrilickollari/debtdrone-cli/v2/internal/service"
)

// Build-time variables injected by the linker (e.g. via -ldflags).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type state int

const (
	stateMenu state = iota
	stateScanning
	stateResults
	stateHistory
	stateConfig
	stateUpdating
	stateHelp
)

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
	return NewConfiguredAppModel(localconfig.Defaults())
}

// NewConfiguredAppModel initializes every TUI setting from the same resolved
// values used by the headless CLI and MCP server.
func NewConfiguredAppModel(values localconfig.Values) *AppModel {
	return &AppModel{
		activeState: stateMenu,
		width:       120,
		height:      40,
		menu:        newMenuModel(),
		scan:        newScanModel(),
		history:     newHistoryModel(),
		config:      newConfigModelWithValues(values),
		update:      newUpdateModel(),
	}
}

func RunTUI(values localconfig.Values) error {
	_, err := tea.NewProgram(NewConfiguredAppModel(values)).Run()
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
		maxComplexity, _ := strconv.Atoi(m.config.ConfigValue(localconfig.KeyMaxComplexity))
		maxResults, _ := strconv.Atoi(m.config.ConfigValue(localconfig.KeyMaxResults))
		scanOptions := service.ScanOptions{
			MaxComplexity: maxComplexity,
			SecurityScan:  m.config.ConfigValue(localconfig.KeySecurityScan) == "true",
			Coverage:      m.config.ConfigValue(localconfig.KeyCoverage) == "true",
		}
		display := scanDisplayOptions{
			outputFormat:    m.config.ConfigValue(localconfig.KeyOutputFormat),
			showLineNumbers: m.config.ConfigValue(localconfig.KeyShowLineNumbers) == "true",
			maxResults:      maxResults,
		}
		cmd := m.scan.Start(msg.Path, scanOptions, display, m.config.ConfigValue(localconfig.KeyHistoryEnabled) == "true")
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
		outputFormat := m.config.ConfigValue(localconfig.KeyOutputFormat)
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
		return m, m.menu.RefreshHistory()
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
