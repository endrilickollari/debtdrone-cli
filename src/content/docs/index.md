---
title: DebtDrone CLI documentation
description: Scan technical debt locally, explore findings in the TUI, and enforce quality gates in CI.
---

DebtDrone is an open-source technical-debt scanner with two local interfaces:
an interactive terminal UI for investigation and a headless CLI for automation.
Both use the same reusable scanner that the DebtDrone SaaS consumes as a
versioned Go package. Teams that need hosted repositories, dashboards,
organizations, and issue workflows can use the
[DebtDrone SaaS](https://debtdrone.net); this documentation remains focused on
the open-source CLI and scanner API.

```bash
debtdrone                              # Open the interactive TUI
debtdrone scan . --format=json        # Produce a machine-readable report
debtdrone scan . --fail-on=high       # Enforce a CI quality gate
```

:::tip[First time here?]
Follow [Run your first scan](./quickstart/) for a predictable installation and
scan workflow that does not require Trivy.
:::

## Choose your workflow

### Explore a repository locally

Launch `debtdrone` from a repository root, enter `/scan`, and inspect findings
in the master-detail TUI. History and settings are held only for the current
TUI process.

[Use the interactive TUI →](./tui-usage/)

### Automate a scan

Use `debtdrone scan` to write a text table or JSON array. Add `--fail-on` when a
matching severity should return a non-zero status in CI.

[Run scans in CI/CD →](./headless-usage/)

### Call DebtDrone from an agent

Connect Codex or Claude Code to the local, read-only MCP server. The
`scan_repository` tool uses the same scanner as the CLI and stays within the
repository root you configure. MCP is currently available on `main` and
will enter tagged distribution with the next approved release.

[Connect a coding agent →](./mcp-and-agents/)

### Embed the scanner in Go

Go consumers import `github.com/endrilickollari/debtdrone-cli/v2/scanner` and
receive a neutral report without CLI, TUI, or SaaS types.

[Understand the scanner architecture →](./architecture/)

## Documentation by type

### Tutorials

Tutorials teach a complete workflow from beginning to end.

- [Run your first scan](./quickstart/)

### How-to guides

Use these when you already know the outcome you need.

- [Install the CLI](./installation/)
- [Explore findings in the TUI](./tui-usage/)
- [Run scans in CI/CD](./headless-usage/)
- [Configure current scans](./configuration/)
- [Troubleshoot common failures](./troubleshooting/)

### Reference

Reference pages describe the exact current interface and reusable APIs.

- [Command reference](./command-reference/)
- [Coverage execution for Go consumers](./scanner-coverage/)

### Concepts

Concept pages explain boundaries and design decisions.

- [System architecture](./architecture/)
- [Scanner ownership and contribution boundary](./ownership/)
- [MCP and coding agents](./mcp-and-agents/)

### Project information

- [Contributing](./contributing/)
- [Documentation deployment](./deployment/)
- [Versioning and releases](./versioning/)

## Current capability boundaries

- Headless configuration is supplied through `scan` flags.
- `.debtdrone.yaml` is generated as a preview but is not loaded by scans.
- `config set` does not persist changes.
- Headless `history` returns demonstration entries; TUI history is session-only.
- Trivy security analysis is optional and can be disabled with
  `--security-scan=false`.
- The MCP server is local, stdio-only, read-only, and restricted to an explicit
  repository root.

The documentation calls out these limitations explicitly so examples remain
safe for scripts, CI systems, and agents.
