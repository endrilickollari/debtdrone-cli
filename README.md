# 🚁 DebtDrone CLI

![Go Version](https://img.shields.io/github/go-mod/go-version/endrilickollari/debtdrone-cli)
![Build Status](https://img.shields.io/github/actions/workflow/status/endrilickollari/debtdrone-cli/ci.yml?branch=main)
![License](https://img.shields.io/github/license/endrilickollari/debtdrone-cli)
![Release](https://img.shields.io/github/v/release/endrilickollari/debtdrone-cli)

**DebtDrone CLI** is a lightning-fast, highly configurable technical debt analyzer. 

Built with a **Hexagonal Architecture**, DebtDrone ships as a single, statically-linked Go binary that serves two distinct purposes:
1. **Interactive TUI:** A beautiful, responsive terminal interface for developers to explore code complexity locally.
2. **Headless CLI:** A robust, pipeline-ready executable for CI/CD environments with strict quality gates and JSON outputs.

---

## ✨ Features

### 🎨 The Interactive TUI (For Humans)
Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss).
* **Master-Detail Explorer:** Navigate hundreds of issues effortlessly without text truncation.
* **Historical Tracking:** View past scans and track whether your debt is shrinking or growing over time.
* **Inline Configuration:** Modify thresholds and rules directly within the terminal—no need to touch Vim.
* **Seamless Updates:** Built-in auto-updater with changelog modals (`/update`).

### 🤖 The Headless CLI (For Machines)
Built on [Cobra](https://github.com/spf13/cobra).
* **CI/CD Quality Gates:** Fail your build pipelines automatically if new Critical or High debt is introduced using `--fail-on`.
* **Structured Output:** Export results to standard Text tables or machine-readable JSON (`--format=json`).
* **Deterministic Execution:** Bypasses all interactive prompts to ensure pipelines never hang.
* **Config as Code:** Commit a `.debtdrone.yaml` to your repo to ensure local and pipeline scans share the exact same ruleset.

---

## 🚀 Installation

**Via Go Install:**
```bash
go install github.com/endrilickollari/debtdrone-cli/cmd/debtdrone@latest
```

**Via Pre-compiled Binaries:**
Check the [Releases](https://github.com/endrilickollari/debtdrone-cli/releases) page for static binaries for macOS, Linux, and Windows.

---

## 🎮 Usage: Interactive TUI
To launch the interactive dashboard, simply run the tool with no arguments:

```bash
debtdrone
```

### TUI Commands & Navigation
Once inside the TUI, you can use standard Vim bindings (`j`/`k`) to navigate. Use the command bar to jump between modules:

* `/scan` - Start a new technical debt scan on the current directory.
* `/history` - View a list of previous scans and their severity breakdowns.
* `/config` - Open the Settings App to adjust global or repository-specific thresholds.
* `/update` - Check for new releases and install them in-place.

---

## ⚙️ Usage: Headless CLI (CI/CD)
The headless CLI is designed for automation, scripting, and CI/CD workflows.

### Running a Scan
Run a silent scan and output a clean text table:
```bash
debtdrone scan ./my-project
```

Output results as JSON for pipeline parsing:
```bash
debtdrone scan ./my-project --format=json
```

### The Quality Gate (Failing Builds)
Prevent bad code from being merged by setting a severity threshold. If the scanner finds any issue matching or exceeding this level, it returns a non-zero exit code (`os.Exit(1)`).

```bash
# Fails the pipeline if Critical or High debt is found
debtdrone scan ./my-project --fail-on=high
```

### Configuration Management
Initialize a default `.debtdrone.yaml` in your repository:
```bash
debtdrone init
```

View or edit settings via the CLI:
```bash
debtdrone config list
debtdrone config set thresholds.max_complexity 15
```

---

## 🛠 GitHub Actions Integration
DebtDrone is built to live in your CI/CD pipeline. Here is a copy-paste example of how to implement a DebtDrone Quality Gate in your GitHub Actions:

```yaml
name: Code Quality Gate

on: [push, pull_request]

jobs:
  debtdrone-scan:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Code
        uses: actions/checkout@v4
        
      - name: Install DebtDrone
        run: |
          curl -sL https://github.com/endrilickollari/debtdrone-cli/releases/latest/download/debtdrone_Linux_x86_64.tar.gz | tar xz
          sudo mv debtdrone /usr/local/bin/

      - name: Run DebtDrone Quality Gate
        # Fails the PR if high or critical technical debt is introduced
        run: debtdrone scan ./ --format=text --fail-on=high
```

---

## 🏗 Architecture
DebtDrone uses a strict **Ports & Adapters (Hexagonal)** architecture to ensure the core analysis engine remains decoupled from the presentation layer.

* **`internal/analysis/`**: The core business logic. Pure Go, UI-blind, highly concurrent scanning engine.
* **`cmd/debtdrone/`**: The Cobra routing layer. Handles headless execution, flag parsing, and OS exit codes.
* **`internal/tui/`**: The presentation layer. Implements the Bubble Tea Nested Router Pattern. Every major screen (AppModel, ConfigModel, ScanModel) is fully encapsulated and communicates via event-driven `tea.Msg` passing.

---

## 💻 Development & Contributing
We welcome contributions! To get started:

1. Clone the repository.
2. Run `go mod tidy`.
3. Build the binary: `go build -o debtdrone ./cmd/debtdrone/main.go`.

### Testing
We maintain two distinct test suites:
* **Headless Tests**: `go test ./cmd/...` tests the Cobra buffers, structured JSON output, and OS exit codes.
* **TUI Tests**: `go test ./internal/tui/...` tests the Bubble Tea state machines using pure functional state injection. *(Note: Our test helpers forcefully apply TrueColor profiles to ensure Lipgloss strings render deterministically in headless CI environments).*

---

## 📄 License
DebtDrone CLI is distributed under the **MIT License**. Free to use, modify, and distribute.

See [LICENSE](LICENSE) for full details.

---

## 🤝 Contributing
This repository serves as the **public distribution channel** for DebtDrone CLI. The source code is proprietary, but we welcome:

* 🐛 Bug reports
* 💡 Feature requests
* 📖 Documentation improvements

Read our [Contributing Guide](CONTRIBUTING.md) to get started.

### Quick Links
* 📖 [Contributing Guidelines](CONTRIBUTING.md) - How to contribute
* 🔨 [Build Guide](BUILD.md) - Build system and release process
* 📋 [Issues](https://github.com/endrilickollari/debtdrone-cli/issues) - Report bugs or request features

<div align="center">

**Built with ❤️.**

</div>

---

## ☕ Support the Project
If DebtDrone helped you fix a critical issue or saved you time, consider buying me a coffee!

<a href="https://www.buymeacoffee.com/endri.lickollari" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me A Coffee" style="height: 60px !important;width: 217px !important;" ></a>
