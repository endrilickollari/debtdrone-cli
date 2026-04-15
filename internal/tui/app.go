package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/endrilickollari/debtdrone-cli/internal/models"
	"github.com/endrilickollari/debtdrone-cli/internal/update"
	"github.com/google/uuid"
)

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

type updatePhase int

const (
	updateChecking updatePhase = iota
	updatePrompt
	updateInstalling
	updateSuccess
	updateError
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

type model struct {
	state     state
	input     string
	cursorPos int
	issues    []models.TechnicalDebtIssue
	scanPath  string
	err       error
	scanning  bool

	list   issueList
	detail issueViewport

	historyEntries []historyEntry
	historyCursor  int
	historyOffset  int
	historyDetail  issueViewport

	configItems       []configItem
	configCursor      int
	configOffset      int
	configCurrentMode configMode
	configEditBuffer  string

	updateStatus updatePhase
	updateInfo   *update.UpdateInfo
	updateErr    error

	scanTask     string
	scanProgress float64
	scanChan     chan tea.Msg

	mu                 sync.Mutex
	spinnerFrame       int
	width              int
	height             int
	suggestions        []string
	selectedSuggestion int
}

func initialModel() *model {
	return &model{
		state:              stateMenu,
		scanning:           false,
		spinnerFrame:       0,
		width:              120,
		height:             40,
		selectedSuggestion: -1,
		configItems:        defaultConfigItems(),
		scanChan:           make(chan tea.Msg, 10),
	}
}

func (m *model) listenForScanProgress() tea.Cmd {
	return func() tea.Msg {
		return <-m.scanChan
	}
}

func (m *model) getConfigValue(key string) string {
	for _, item := range m.configItems {
		if item.Key == key {
			return item.Value
		}
	}
	return ""
}

func RunTUI() error {
	m := initialModel()
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

type tickMsg struct{}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listH, detailH := splitHeight(m.height)
		if m.state == stateResults {
			m.list.height = listH
			m.list.width = m.width
			m.detail.height = detailH
			m.detail.width = m.width - 4
		}
		if m.state == stateHistory {
			m.historyDetail.height = detailH
			m.historyDetail.width = m.width - 4
			if len(m.historyEntries) > 0 {
				m.historyDetail.setContent(
					formatHistoryDetail(m.historyEntries[m.historyCursor], m.historyDetail.width),
				)
			}
		}

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tickMsg:
		switch {
		case m.state == stateScanning:
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerChars)
			return m, tickCmd()
		case m.state == stateUpdating &&
			(m.updateStatus == updateChecking || m.updateStatus == updateInstalling):
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerChars)
			return m, tickCmd()
		}

	case scanProgressMsg:
		m.scanTask = msg.Task
		m.scanProgress = msg.Progress
		return m, m.listenForScanProgress()

	case scanCompleteMsg:
		m.mu.Lock()
		m.scanning = false
		if msg.err != nil {
			m.err = msg.err
			m.state = stateMenu
		} else {
			m.issues = msg.issues
			m.state = stateResults

			listH, detailH := splitHeight(m.height)
			m.list = newIssueList(msg.issues, m.width, listH)

			// If JSON output is requested, use the detail viewport as a full-screen scrollable area
			if m.getConfigValue("Output Format") == "json" {
				jsonData, _ := json.MarshalIndent(msg.issues, "", "  ")
				m.detail = issueViewport{
					height: m.height - 4, // More space for full-screen JSON
					width:  m.width - 4,
				}
				m.detail.setContent(string(jsonData))
			} else {
				m.detail = issueViewport{
					height: detailH,
					width:  m.width - 4,
				}
				m.detail.setContent(formatIssueDetail(m.list.selected(), m.detail.width))
			}

			now := time.Now()
			run := models.AnalysisRun{
				ID:                  uuid.New(),
				StartedAt:           now,
				Status:              "completed",
				TotalIssuesFound:    len(msg.issues),
				CriticalIssuesCount: countBySeverity(msg.issues, "critical"),
				HighIssuesCount:     countBySeverity(msg.issues, "high"),
				MediumIssuesCount:   countBySeverity(msg.issues, "medium"),
				LowIssuesCount:      countBySeverity(msg.issues, "low"),
			}
			run.CompletedAt = &now
			run.RepositoryName = &msg.path
			m.historyEntries = append([]historyEntry{{
				run:    run,
				path:   msg.path,
				issues: msg.issues,
			}}, m.historyEntries...)
		}
		m.mu.Unlock()
		return m, nil

	case checkUpdateMsg:
		m.mu.Lock()
		m.scanning = false
		if msg.err != nil {
			m.updateErr = msg.err
			m.updateStatus = updateError
		} else if msg.info == nil || !msg.info.Available {
			m.updateInfo = msg.info
			m.updateStatus = updateSuccess
		} else {
			m.updateInfo = msg.info
			m.updateStatus = updatePrompt
		}
		m.mu.Unlock()
		return m, nil

	case updateCompleteMsg:
		m.mu.Lock()
		if msg.err != nil {
			m.updateErr = msg.err
			m.updateStatus = updateError
		} else {
			m.updateStatus = updateSuccess
		}
		m.mu.Unlock()
		return m, nil
	}

	return m, nil
}

func (m *model) computeSuggestions() {
	if m.input == "" {
		m.suggestions = nil
		m.selectedSuggestion = -1
		return
	}
	prefix := strings.ToLower(strings.Fields(m.input)[0])
	var matches []string
	for _, c := range allCommands {
		if strings.HasPrefix(c.cmd, prefix) && c.cmd != prefix {
			matches = append(matches, c.cmd)
		}
	}
	m.suggestions = matches
	if m.selectedSuggestion >= len(matches) {
		m.selectedSuggestion = -1
	}
}

func (m *model) acceptSuggestion() {
	var chosen string
	if m.selectedSuggestion >= 0 && m.selectedSuggestion < len(m.suggestions) {
		chosen = m.suggestions[m.selectedSuggestion]
	} else if len(m.suggestions) > 0 {
		chosen = m.suggestions[0]
	}
	if chosen == "" {
		return
	}
	m.input = chosen + " "
	m.cursorPos = len(m.input)
	m.suggestions = nil
	m.selectedSuggestion = -1
}

func (m *model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	str := msg.String()

	switch m.state {
	case stateMenu:
		switch str {
		case "enter":
			if m.selectedSuggestion >= 0 && len(m.suggestions) > 0 {
				m.acceptSuggestion()
				return m, nil
			}
			return m.handleCommand()
		case "tab":
			if len(m.suggestions) > 0 {
				m.selectedSuggestion = (m.selectedSuggestion + 1) % len(m.suggestions)
			}
		case "shift+tab":
			if len(m.suggestions) > 0 {
				m.selectedSuggestion--
				if m.selectedSuggestion < 0 {
					m.selectedSuggestion = len(m.suggestions) - 1
				}
			}
		case "down":
			if len(m.suggestions) > 0 {
				m.selectedSuggestion = (m.selectedSuggestion + 1) % len(m.suggestions)
			}
		case "up":
			if len(m.suggestions) > 0 {
				m.selectedSuggestion--
				if m.selectedSuggestion < 0 {
					m.selectedSuggestion = len(m.suggestions) - 1
				}
			}
		case "right":
			if m.cursorPos < len(m.input) {
				m.cursorPos++
			} else if len(m.suggestions) > 0 {
				m.acceptSuggestion()
			}
		case "left":
			if m.cursorPos > 0 {
				m.cursorPos--
			}
		case "backspace":
			if m.cursorPos > 0 {
				m.input = m.input[:m.cursorPos-1] + m.input[m.cursorPos:]
				m.cursorPos--
				m.computeSuggestions()
			}
		case "ctrl+c":
			fmt.Println("👋 Goodbye!")
			os.Exit(0)
		default:
			if len(str) == 1 {
				m.input = m.input[:m.cursorPos] + str + m.input[m.cursorPos:]
				m.cursorPos++
				m.computeSuggestions()
			}
		}
		return m, nil

	case stateScanning:
		if str == "ctrl+c" {
			fmt.Println("👋 Goodbye!")
			os.Exit(0)
		}
		return m, nil

	case stateResults:
		isJSON := m.getConfigValue("Output Format") == "json"

		updateDetail := func() {
			if !isJSON {
				m.detail.setContent(formatIssueDetail(m.list.selected(), m.detail.width))
			}
		}

		switch str {
		case "q", "esc":
			m.state = stateMenu
			m.input = ""
			m.cursorPos = 0
		case "j", "down":
			if isJSON {
				m.detail.scrollDown(1)
			} else {
				m.list.moveDown()
				updateDetail()
			}
		case "k", "up":
			if isJSON {
				m.detail.scrollUp(1)
			} else {
				m.list.moveUp()
				updateDetail()
			}
		case "pgdn":
			m.list.pageDown()
			updateDetail()
		case "pgup":
			m.list.pageUp()
			updateDetail()
		case "g":
			m.list.goTop()
			updateDetail()
		case "G":
			m.list.goBottom()
			updateDetail()
		case "J":
			m.detail.scrollDown(3)
		case "K":
			m.detail.scrollUp(3)
		case "r":
			m.state = stateMenu
			m.input = ""
			m.cursorPos = 0
		}
		return m, nil

	case stateHistory:
		updateHistoryDetail := func() {
			if len(m.historyEntries) == 0 {
				return
			}
			m.historyDetail.setContent(
				formatHistoryDetail(m.historyEntries[m.historyCursor], m.historyDetail.width),
			)
		}

		historyListH, _ := splitHeight(m.height)

		switch str {
		case "q", "esc":
			m.state = stateMenu
			m.input = ""
			m.cursorPos = 0
		case "j", "down":
			if m.historyCursor < len(m.historyEntries)-1 {
				m.historyCursor++
				if m.historyCursor >= m.historyOffset+historyListH {
					m.historyOffset++
				}
				updateHistoryDetail()
			}
		case "k", "up":
			if m.historyCursor > 0 {
				m.historyCursor--
				if m.historyCursor < m.historyOffset {
					m.historyOffset--
				}
				updateHistoryDetail()
			}
		case "g":
			m.historyCursor = 0
			m.historyOffset = 0
			updateHistoryDetail()
		case "G":
			if len(m.historyEntries) > 0 {
				m.historyCursor = len(m.historyEntries) - 1
				m.historyOffset = max(0, m.historyCursor-historyListH+1)
				updateHistoryDetail()
			}
		case "J":
			m.historyDetail.scrollDown(3)
		case "K":
			m.historyDetail.scrollUp(3)
		case "enter":
			if len(m.historyEntries) == 0 {
				break
			}
			entry := m.historyEntries[m.historyCursor]
			m.issues = entry.issues
			m.scanPath = entry.path
			m.err = nil
			listH, detailH := splitHeight(m.height)
			m.list = newIssueList(entry.issues, m.width, listH)
			m.detail = issueViewport{height: detailH, width: m.width - 4}
			m.detail.setContent(formatIssueDetail(m.list.selected(), m.detail.width))
			m.state = stateResults
		}
		return m, nil

	case stateConfig:
		switch m.configCurrentMode {
		case configNavigating:
			visibleRows := max(m.height-8, 4)
			switch str {
			case "j", "down":
				if m.configCursor < len(m.configItems)-1 {
					m.configCursor++
					if m.configCursor >= m.configOffset+visibleRows {
						m.configOffset++
					}
				}
			case "k", "up":
				if m.configCursor > 0 {
					m.configCursor--
					if m.configCursor < m.configOffset {
						m.configOffset--
					}
				}
			case "g":
				m.configCursor = 0
				m.configOffset = 0
			case "G":
				m.configCursor = len(m.configItems) - 1
				m.configOffset = max(0, m.configCursor-visibleRows+1)
			case "q", "esc":
				m.state = stateMenu
				m.input = ""
				m.cursorPos = 0
			case "enter", " ":
				item := &m.configItems[m.configCursor]
				if item.Type == "bool" {
					if item.Value == "true" {
						item.Value = "false"
					} else {
						item.Value = "true"
					}
				} else if item.IsOption {
					// Cycle through options
					currentIndex := -1
					for i, opt := range item.Options {
						if opt == item.Value {
							currentIndex = i
							break
						}
					}
					nextIndex := (currentIndex + 1) % len(item.Options)
					item.Value = item.Options[nextIndex]
				} else {
					m.configEditBuffer = item.Value
					m.configCurrentMode = configEditing
				}
			case "right":
				item := &m.configItems[m.configCursor]
				if item.IsOption {
					currentIndex := -1
					for i, opt := range item.Options {
						if opt == item.Value {
							currentIndex = i
							break
						}
					}
					nextIndex := (currentIndex + 1) % len(item.Options)
					item.Value = item.Options[nextIndex]
				}
			case "left":
				item := &m.configItems[m.configCursor]
				if item.IsOption {
					currentIndex := -1
					for i, opt := range item.Options {
						if opt == item.Value {
							currentIndex = i
							break
						}
					}
					prevIndex := (currentIndex - 1 + len(item.Options)) % len(item.Options)
					item.Value = item.Options[prevIndex]
				}
			}

		case configEditing:
			switch str {
			case "esc":
				m.configEditBuffer = ""
				m.configCurrentMode = configNavigating
			case "enter":
				m.configItems[m.configCursor].Value = m.configEditBuffer
				m.configEditBuffer = ""
				m.configCurrentMode = configNavigating
			case "backspace":
				runes := []rune(m.configEditBuffer)
				if len(runes) > 0 {
					m.configEditBuffer = string(runes[:len(runes)-1])
				}
			default:
				if isEditableChar(str) {
					m.configEditBuffer += str
				}
			}
		}
		return m, nil

	case stateUpdating:
		switch m.updateStatus {
		case updateChecking, updateInstalling:
			return m, nil
		case updatePrompt:
			switch str {
			case "y":
				m.updateStatus = updateInstalling
				m.spinnerFrame = 0
				return m, tea.Batch(performUpdateCmd, tickCmd())
			case "n", "q", "esc":
				m.state = stateMenu
				m.updateInfo = nil
				m.updateErr = nil
			}
			return m, nil
		case updateSuccess, updateError:
			m.state = stateMenu
			m.updateInfo = nil
			m.updateErr = nil
			return m, nil
		}
		return m, nil

	case stateHelp:
		if str == "esc" || str == "q" {
			m.state = stateMenu
		}
		return m, nil
	}

	return m, nil
}

func (m *model) handleCommand() (tea.Model, tea.Cmd) {
	cmd := strings.TrimSpace(m.input)
	m.input = ""
	m.cursorPos = 0
	m.suggestions = nil
	m.selectedSuggestion = -1

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return m, nil
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "/scan":
		path := "."
		if len(parts) > 1 {
			path = parts[1]
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			m.err = fmt.Errorf("failed to resolve path: %w", err)
			return m, nil
		}

		maxComplexity := 15
		fmt.Sscanf(m.getConfigValue("Max Complexity"), "%d", &maxComplexity)
		securityScan := m.getConfigValue("Security Scan") == "true"

		m.scanPath = absPath
		m.scanning = true
		m.scanTask = "Initializing scan..."
		m.scanProgress = 0
		m.state = stateScanning
		return m, tea.Batch(
			startScan(absPath, maxComplexity, securityScan, m.scanChan),
			m.listenForScanProgress(),
		)

	case "/update":
		m.updateStatus = updateChecking
		m.updateErr = nil
		m.updateInfo = nil
		m.spinnerFrame = 0
		m.scanning = true
		m.state = stateUpdating
		return m, tea.Batch(startUpdateCheck, tickCmd())

	case "/history":
		m.historyCursor = 0
		m.historyOffset = 0
		_, detailH := splitHeight(m.height)
		m.historyDetail = issueViewport{height: detailH, width: m.width - 4}
		if len(m.historyEntries) > 0 {
			m.historyDetail.setContent(
				formatHistoryDetail(m.historyEntries[0], m.historyDetail.width),
			)
		}
		m.state = stateHistory
		return m, nil

	case "/config":
		if len(m.configItems) == 0 {
			m.configItems = defaultConfigItems()
		}
		m.configCursor = 0
		m.configOffset = 0
		m.configCurrentMode = configNavigating
		m.configEditBuffer = ""
		m.state = stateConfig
		return m, nil

	case "/help", "/h", "?":
		m.state = stateHelp
		return m, nil

	case "/quit", "/q", "exit":
		fmt.Println("👋 Goodbye!")
		os.Exit(0)
	}

	return m, nil
}
