# System Architecture

This document describes the internal architecture of DebtDrone for contributors and maintainers. It covers two primary design decisions: the **Hexagonal (Ports & Adapters)** layout that keeps the analysis engine independent of any UI, and the **Bubble Tea Nested Router Pattern** that prevents the TUI from collapsing into a monolithic state struct.

---

## High-Level Layout

```
debtdrone-cli/
├── cmd/debtdrone/          # Cobra CLI — primary adapter for machine consumers
│   ├── main.go             # Root command, TUI entry point
│   ├── scan.go             # `scan` subcommand
│   ├── init.go             # `init` subcommand
│   ├── config.go           # `config list / set` subcommands
│   └── history.go          # `history` subcommand
│
├── internal/
│   ├── models/             # Domain — pure data types & business logic
│   │   ├── complexity.go   # ComplexityMetric, severity rules, debt calculation
│   │   └── database.go     # TechnicalDebtIssue, AnalysisRun
│   │
│   ├── analysis/           # Core port — Analyzer interface & implementations
│   │   ├── analyzer.go     # Analyzer interface (port)
│   │   └── analyzers/
│   │       ├── complexity/ # Language-specific adapters (14 languages)
│   │       └── security/   # Trivy adapter
│   │
│   ├── store/              # Storage port & in-memory adapter
│   │   ├── *.go            # Store interfaces (ports)
│   │   └── memory/         # In-memory implementations (adapters)
│   │
│   ├── service/            # Application layer — orchestration
│   │   └── scan_service.go # Coordinates analyzers, merges results
│   │
│   ├── git/                # Git adapter (local open, remote clone)
│   ├── config/             # Config loading
│   ├── update/             # Self-updater
│   └── tui/                # Bubble Tea TUI — primary adapter for human consumers
│       ├── app.go          # AppModel — root state machine & router
│       ├── messages.go     # Custom tea.Msg event types
│       ├── menu.go         # MenuModel — command bar
│       ├── scanning.go     # ScanModel — scan progress & results
│       ├── history.go      # HistoryModel — past scans browser
│       ├── config.go       # ConfigModel — settings editor
│       └── update_view.go  # UpdateModel — self-update UI
```

---

## Hexagonal (Ports & Adapters) Architecture

The central principle is that the **analysis domain has no knowledge of how its results are consumed**. It does not know whether it is running inside a terminal UI, a Cobra command, or a future HTTP server. This is achieved through three distinct layers.

### Layer 1 — Domain (`internal/models/`)

The domain layer contains pure Go structs and functions with no external dependencies. This is where business logic lives.

`ComplexityMetric` holds all per-function measurements. `DetermineSeverity()` and `CalculateTechnicalDebt()` encode the rules that convert raw numbers into actionable findings:

```go
// internal/models/complexity.go

// Debt is calculated from the excess above each threshold:
//   Cyclomatic > 20:   (cc  - 20) × 15 min
//   Cognitive  > 15:   (cog - 15) × 8  min
//   Nesting    > 5:    (n   - 5)  × 20 min
//   Parameters > 7:    (p   - 7)  × 15 min
//   LOC        > 300:  ((loc - 300) / 50) × 30 min
func (m *ComplexityMetric) CalculateTechnicalDebt() float64 { ... }
```

The domain never imports `bubbletea`, `cobra`, or any I/O package.

### Layer 2 — Ports (`internal/analysis/analyzer.go`, `internal/store/`)

Ports are Go interfaces that define what the application layer can ask for, without specifying how the answer is produced.

```go
// internal/analysis/analyzer.go

// Analyzer is the primary port for the analysis engine.
// Any concrete language analyzer, security scanner, or mock
// in tests satisfies this interface.
type Analyzer interface {
    Name() string
    Analyze(path string) ([]models.TechnicalDebtIssue, error)
}
```

Similarly, the store interfaces (`ComplexityStoreInterface`, etc.) define persistence operations without tying the application to any specific database or in-memory structure.

### Layer 3 — Adapters

Adapters are concrete implementations of ports. DebtDrone ships several:

**Language Analyzers** (`internal/analysis/analyzers/complexity/`)

Each of the 14 supported languages has its own adapter that uses tree-sitter to parse a syntax tree and extract complexity metrics. A `Factory` function maps file extensions to the correct adapter at runtime:

```go
// internal/analysis/analyzers/complexity/factory.go
func NewAnalyzer(ext string) (LanguageAnalyzer, bool) {
    switch ext {
    case ".go":
        return &GoAnalyzer{}, true
    case ".ts", ".tsx":
        return &TypeScriptAnalyzer{}, true
    // ... 12 more languages
    }
}
```

Adding support for a new language means implementing one interface and registering one `case` — no other code needs to change.

**Security Adapter** (`internal/analysis/analyzers/security/trivy.go`)

Shells out to the `trivy fs` command and translates its output into `TechnicalDebtIssue` objects, satisfying the same `Analyzer` interface.

**Storage Adapters** (`internal/store/memory/`)

In-memory implementations used by both the TUI (ephemeral scan sessions) and the CLI (single-run aggregation). Replacing these with SQL-backed adapters requires only a new struct satisfying the existing store interfaces.

**CLI Adapter** (`cmd/debtdrone/`)

The Cobra commands are thin adapters that parse flags, call `scan_service.go`, and serialize the result to stdout in the requested format. They contain no analysis logic.

**TUI Adapter** (`internal/tui/`)

The Bubble Tea application is another adapter consuming the same `scan_service.go`. It presents results through an interactive UI instead of stdout.

!!! note "Testing benefit"
    Because the domain and service layer depend only on interfaces, unit tests can inject lightweight in-memory adapters without starting a real filesystem scan or shelling out to Trivy. Integration tests swap in the real adapters.

---

## Bubble Tea Nested Router Pattern

Bubble Tea's `Model` interface (`Init`, `Update`, `View`) is simple and composable, but naive implementations accumulate all application state into a single struct as the UI grows. DebtDrone uses a **Nested Router** pattern to prevent this.

### The Root State Machine — `AppModel`

`AppModel` (`internal/tui/app.go`) owns a `state` enum and holds references to all child models. It acts as a router, not a view:

```go
// internal/tui/app.go

type state int

const (
    stateMenu     state = iota // Command bar is active
    stateScanning              // Scan progress/results view is active
    stateResults               // (sub-state of stateScanning after completion)
    stateHistory               // History browser is active
    stateConfig                // Config editor is active
    stateUpdating              // Update view is active
    stateHelp                  // Help overlay
)

type AppModel struct {
    activeState state
    width, height int

    // Child models are always initialized; only one is rendered at a time.
    menu    *MenuModel
    scan    *ScanModel
    history *HistoryModel
    config  *ConfigModel
    update  *UpdateModel
}
```

All child models are instantiated at startup. Switching views is a state transition on `AppModel`, not the creation of a new model. This keeps every child model's internal state intact across navigations (e.g., returning to a history entry you were viewing after checking config).

### Event-Driven Routing via Custom Messages

Cross-model communication happens through custom `tea.Msg` types defined in `internal/tui/messages.go`. Child models never call methods on each other directly; they return a `tea.Cmd` that emits a message, and `AppModel.Update()` dispatches it:

```go
// internal/tui/messages.go

// MenuModel emits this when the user selects /scan
type StartScanMsg struct {
    Path string
}

// ScanModel emits this when analysis completes
type ScanFinishedMsg struct {
    Entry historyEntry
    Err   error
}

// Emitted by any child model to transition the active view
type NavigateMsg struct {
    State state
}

// Emitted to trigger the history detail view
type LoadHistoryRunMsg struct {
    Entry historyEntry
}
```

`AppModel.Update()` intercepts these messages before delegating to child models:

```go
// internal/tui/app.go (simplified)

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {

    case tea.WindowSizeMsg:
        m.width, m.height = msg.Width, msg.Height
        // Propagate size to all children

    case NavigateMsg:
        m.activeState = msg.State
        return m, nil

    case StartScanMsg:
        m.activeState = stateScanning
        return m, m.scan.StartScan(msg.Path) // Returns a tea.Cmd

    case ScanFinishedMsg:
        // Persist to history, transition to results view
        m.historyEntries = append(m.historyEntries, msg.Entry)
        m.activeState = stateResults
        return m, nil
    }

    // Delegate remaining messages to the active child model only
    return m.delegateToActive(msg)
}
```

### Encapsulated Child Models

Each child model (`ScanModel`, `ConfigModel`, etc.) has its own internal state enum, its own `Update` loop, and its own `View` renderer. `AppModel` never reads a child model's internal fields — it only sends messages and calls `View()` for rendering.

```
User Input
    │
    ▼
AppModel.Update()
    │
    ├── Handles cross-cutting messages (NavigateMsg, WindowSizeMsg, ...)
    │
    └── delegateToActive()
            │
            ├── activeState == stateScanning  →  ScanModel.Update(msg)
            ├── activeState == stateHistory   →  HistoryModel.Update(msg)
            ├── activeState == stateConfig    →  ConfigModel.Update(msg)
            └── ...
```

This means:

- **Adding a new view** requires writing one new `*Model` struct and adding one `case` to `delegateToActive()`. No existing model is modified.
- **Child models are fully testable in isolation** — pass messages in, assert on the returned `tea.Model` state and emitted `tea.Cmd`.
- **The global state surface is minimal** — `AppModel` holds only what is genuinely shared (window dimensions, cross-model history entries). Everything else is encapsulated in the child that owns it.

!!! tip "Contributing a new view"
    To add a new TUI feature (e.g., a `/report` export view):
    
    1. Create `internal/tui/report.go` with a `ReportModel` struct implementing `tea.Model`.
    2. Add a `stateReport` constant to the `state` enum in `app.go`.
    3. Add a `report *ReportModel` field to `AppModel` and initialize it in `New()`.
    4. Add a `case stateReport: return m.report.Update(msg)` in `delegateToActive()`.
    5. Add a `StartReportMsg` type in `messages.go` and handle it in `AppModel.Update()`.
    
    No existing child model needs to change.
