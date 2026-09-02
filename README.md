<p align="center">
  <img src="public/debtdrone-logo.svg" alt="DebtDrone icon" width="96" height="96">
</p>

# DebtDrone CLI

![Go Version](https://img.shields.io/github/go-mod/go-version/endrilickollari/debtdrone-cli)
![Build Status](https://img.shields.io/github/actions/workflow/status/endrilickollari/debtdrone-cli/ci.yml?branch=main)
![License](https://img.shields.io/github/license/endrilickollari/debtdrone-cli)
![Release](https://img.shields.io/github/v/release/endrilickollari/debtdrone-cli)

**DebtDrone CLI** is a lightning-fast, highly configurable technical debt analyzer. 

Built with a **Hexagonal Architecture**, DebtDrone ships as a single, statically-linked Go binary that serves three distinct purposes:
1. **Interactive TUI:** A beautiful, responsive terminal interface for developers to explore code complexity locally.
2. **Headless CLI:** A robust, pipeline-ready executable for CI/CD environments with strict quality gates and JSON outputs.
3. **Coding-agent MCP:** A local stdio server on `main` that scans within an explicit repository root without modifying its contents.

---

## ✨ Features

### 🎨 The Interactive TUI (For Humans)
Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss).
* **Master-Detail Explorer:** Navigate hundreds of issues effortlessly without text truncation.
* **Privacy-Conscious History:** Completed headless and TUI scans persist bounded
  summary metadata locally; the current TUI browser still displays only scans
  from its active session.
* **Inline Configuration:** Modify thresholds and rules directly within the terminal—no need to touch Vim.
* **Seamless Updates:** Built-in auto-updater with changelog modals (`/update`).

### 🤖 The Headless CLI (For Machines)
Built on [Cobra](https://github.com/spf13/cobra).
* **CI/CD Quality Gates:** Fail your build pipelines automatically if new Critical or High debt is introduced using `--fail-on`.
* **Structured Output:** Export results to standard Text tables or machine-readable JSON (`--format=json`).
* **Deterministic Execution:** Bypasses all interactive prompts to ensure pipelines never hang.
* **Explicit Scan Controls:** Configure each run with flags such as `--fail-on`, `--max-complexity`, and `--security-scan`.
* **Local History Commands:** List, inspect, delete, or explicitly clear bounded scan summaries without storing source contents.

### 🔌 MCP Server (For Coding Agents)
* **Supported Clients:** Connect Codex or Claude Code through local stdio.
* **Scoped Access:** Every tool path stays within the repository root supplied to `debtdrone mcp --root`.
* **Stable Results:** `scan_repository` returns a versioned, deterministic scanner report.

[Configure DebtDrone for a coding agent →](https://cli.debtdrone.net/mcp-and-agents/)

> **Release status:** MCP is available on `main` but is not part of the
> latest tagged release, `v2.1.0`. The agent guide includes a temporary
> source installation until the next approved release.

---

## 🚀 Installation

**Via Homebrew (Recommended):**
```bash
brew install endrilickollari/tap/debtdrone
```

**Via Shell Script (Mac/Linux):**
```bash
curl -sL https://raw.githubusercontent.com/endrilickollari/debtdrone-cli/main/installation_scripts/install.sh | bash
```

**Via PowerShell (Windows):**
```powershell
iwr -useb https://raw.githubusercontent.com/endrilickollari/debtdrone-cli/main/installation_scripts/install.ps1 | iex
```

**Via Go Install:**
```bash
go install github.com/endrilickollari/debtdrone-cli/v2/cmd/debtdrone@latest
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
* `/history` - View scans completed during the current TUI session.
* `/config` - Adjust session settings initialized from your resolved local configuration.
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

Inspect the most recent privacy-conscious scan summaries:

```bash
debtdrone history list
debtdrone history show <id> --format=json
```

### The Quality Gate (Failing Builds)
Prevent bad code from being merged by setting a severity threshold. If the scanner finds any issue matching or exceeding this level, it returns a non-zero exit code (`os.Exit(1)`).

```bash
# Fails the pipeline if Critical or High debt is found
debtdrone scan ./my-project --fail-on=high
```

### Configuration
Initialize a preview `.debtdrone.yaml` template in your repository:
```bash
debtdrone init
```

The current scanner does not load this file yet. Configure a headless scan with
flags:

```bash
debtdrone scan . --max-complexity=15 --fail-on=high
```

Manage the versioned user-level configuration with `config list`, `get`, `set`,
and `unset`:

```bash
debtdrone config set scan.max_complexity 20
debtdrone config list
```

These values are applied consistently to CLI, MCP, and TUI scans. Explicit CLI
flags and MCP tool inputs override them for one invocation. See the
[configuration guide](https://cli.debtdrone.net/configuration/) for storage,
validation, and precedence details.

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
* **`internal/mcpserver/`**: The local stdio adapter for the reusable scanner.
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
* 📦 [Versioning and Releases](src/content/docs/versioning.md) - SemVer policy and the test-gated maintainer workflow
* 📋 [Issues](https://github.com/endrilickollari/debtdrone-cli/issues) - Report bugs or request features

<div align="center">

**Built with ❤️.**

</div>

---

## ☕ Support the Project
If DebtDrone helped you fix a critical issue or saved you time, consider buying me a coffee!

<a href="https://www.buymeacoffee.com/endri.lickollari" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me A Coffee" style="height: 60px !important;width: 217px !important;" ></a>
