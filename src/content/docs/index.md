---
title: Overview
description: Scan technical debt locally and enforce quality gates in CI with one open-source scanner.
---

**DebtDrone** is a Technical Debt Analyzer for engineering teams that need both human insight and machine enforcement — in a single binary.

---

## The Dual-Mode Philosophy

Modern software teams operate in two contexts simultaneously: a developer sitting at a terminal exploring an unfamiliar codebase, and a CI/CD pipeline enforcing quality standards on every pull request. Most tools serve one context well and fail the other. DebtDrone serves both without compromise.

```
debtdrone                        # Launch the interactive TUI — for humans
debtdrone scan ./src --fail-on=high  # Headless quality gate — for machines
```

The same analysis engine powers interactive and headless scans. Supporting
commands differ between the two interfaces; the guides below call out those
differences explicitly.

### Interactive Mode — for Humans

Launch `debtdrone` with no arguments and you enter a full-screen **Terminal UI** built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and styled with [Lipgloss](https://github.com/charmbracelet/lipgloss). A command bar lets you run `/scan`, `/history`, `/config`, and `/update` against any local repository. Results are presented in a navigable master-detail layout where you can drill into every flagged function, read its debt estimate, and follow concrete refactoring suggestions — all without leaving the terminal.

### Headless Mode — for Machines

The headless [Cobra](https://github.com/spf13/cobra) interface exposes `scan`,
`init`, `config`, and `history`. The `scan` command emits a JSON array,
integrates with GitHub Actions, and supports a **Quality Gate** (`--fail-on`)
that exits non-zero when debt reaches a chosen severity threshold. Configure
current scans with flags; the generated YAML file is not loaded yet.

---

## What DebtDrone Analyzes

DebtDrone's analysis engine parses syntax trees and computes multiple metrics per function across **14 languages**: Go, JavaScript, TypeScript, Python, Java, C#, PHP, Ruby, Rust, Kotlin, Swift, C, C++, and JSX/TSX.

| Metric | What It Measures |
|---|---|
| **Cyclomatic Complexity** | Number of independent execution paths through a function |
| **Cognitive Complexity** | How difficult the code is for a human to reason about |
| **Nesting Depth** | Maximum depth of nested control structures |
| **Parameter Count** | Arity of a function — a proxy for coupling |
| **Lines of Code** | Raw function size, correlating with maintainability burden |
| **Halstead Metrics** | Volume, effort, and estimated defect density (bugs/LOC) |
| **Security Vulnerabilities** | CVEs in dependencies and secrets in code, via [Trivy](https://trivy.dev/) |

Every finding is assigned a severity of **critical**, **high**, **medium**, or **low**, and a **debt estimate in minutes** — a concrete number your team can use in sprint planning.

---

## Key Features at a Glance

- **14-language support** via tree-sitter syntax analysis
- **Security scanning** powered by Trivy (CVEs + secrets detection)
- **Quality Gates** — block merges when debt exceeds your threshold
- **Session scan history** — revisit runs completed before the TUI exits
- **Interactive TUI** with Vim keybindings and a master-detail layout
- **Structured JSON output** for pipeline integration and reporting
- **`debtdrone init`** generates a preview `.debtdrone.yaml` template
- **Built-in auto-updater** — the binary keeps itself current

---

## Quick Start

```bash
# Install via go install
go install github.com/endrilickollari/debtdrone-cli/v2/cmd/debtdrone@latest

# Explore your codebase interactively
debtdrone

# Run a headless scan and fail if any HIGH severity debt is found
debtdrone scan ./src --fail-on=high
```

:::tip[New to the tool?]
Start with the [Interactive TUI Explorer](./tui-usage/) to get a feel for what DebtDrone surfaces. Once you understand the findings, move to [CI/CD & Headless CLI](./headless-usage/) to enforce those standards automatically.
:::

---

## Navigation

| Section | Description |
|---|---|
| [Installation](./installation/) | Binary downloads, `go install`, and Homebrew |
| [Interactive TUI Explorer](./tui-usage/) | Full guide to the terminal UI and its commands |
| [CI/CD & Headless CLI](./headless-usage/) | `scan`, `history`, and Quality Gates for pipelines |
| [Coverage execution for Go consumers](./scanner-coverage/) | Artifact parsing and optional isolated test execution |
| [Scanner ownership](./ownership/) | Contribution boundary between the public scanner and SaaS |
| [Configuration](./configuration/) | Current flags and the status of `.debtdrone.yaml` support |
| [System Architecture](./architecture/) | Hexagonal design and the Bubble Tea router pattern |
