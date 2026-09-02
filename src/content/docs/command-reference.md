---
title: Command reference
description: Reference for every command and flag exposed by the current DebtDrone CLI.
---

This page reflects the command tree exposed by `debtdrone --help`. Run
`debtdrone <command> --help` against your installed version when scripting
across multiple CLI releases.

## `debtdrone`

```text
debtdrone [flags]
debtdrone [command]
```

Running without a command opens the interactive TUI in the current directory.

| Global flag | Description |
|---|---|
| `-h`, `--help` | Show command help |
| `-v`, `--version` | Print version, commit, and build metadata |

| Command | Purpose |
|---|---|
| `scan [path]` | Run a headless technical-debt scan |
| `mcp --root <path>` | Run the local, repository-scoped MCP server over stdio |
| `init` | Generate a preview `.debtdrone.yaml` file |
| `config [command]` | Inspect and manage user-level configuration |
| `history [command]` | Inspect and manage bounded local scan summaries |
| `completion` | Generate shell-completion scripts |

:::caution[Repository template limitation]
The current release does not load the repository-level `.debtdrone.yaml`
template. Scans use the versioned user configuration, environment, and explicit
command or MCP overrides instead.
:::

## `debtdrone scan`

```text
debtdrone scan [path] [flags]
```

`path` is optional and defaults to `.`. At most one path is accepted.

| Flag | Shorthand | Default | Description |
|---|---|---|---|
| `--format string` | `-f` | resolved config (`text` built in) | Select `text` or `json` output |
| `--fail-on string` | — | resolved config (`none` built in) | Fail on `critical`, `high`, `medium`, or `low` and above |
| `--max-complexity int` | — | resolved config (`15` built in) | Report high cyclomatic complexity above this value; critical starts above twice this value |
| `--security-scan` | — | resolved config (`true` built in) | Enable the Trivy analyzer |
| `--coverage` | — | resolved config (`false` built in) | Parse supported coverage artifacts already present in the repository |

Boolean flags support Cobra's explicit form when you need to turn a default on
or off:

```bash
debtdrone scan . --security-scan=false --coverage=true
```

Text output is a findings table. JSON output is an array of
`TechnicalDebtIssue` objects. Warnings use stderr. A non-zero exit can mean a
quality-gate violation, invalid input, or an analyzer error after partial
results were printed.

## `debtdrone mcp`

:::note[Release availability]
This command is available on `main` but is not included in `v2.1.0`. Follow
[MCP and coding agents](../mcp-and-agents/#before-you-connect-an-agent) for the
temporary source installation.
:::

```text
debtdrone mcp --root <path>
```

Starts the Model Context Protocol server on stdin and stdout. `--root` is
required and must be an existing directory. DebtDrone resolves the root to a
canonical absolute path before starting the server.

The server exposes the non-destructive `scan_repository` tool. It does not
modify repository contents. Omitted scan settings use the same resolved local
configuration as the CLI and TUI; explicit tool arguments override it. Tool paths must be
relative to the configured root; absolute paths, parent traversal, and symlink
escapes are rejected. Stdout is reserved for MCP protocol messages, so use an
MCP client rather than parsing this command directly.

See [MCP and coding agents](../mcp-and-agents/) for Codex and Claude Code
configuration, tool inputs, verification, and security guidance.

## `debtdrone init`

```text
debtdrone init
```

Creates `.debtdrone.yaml` in the current directory. It returns an error rather
than overwriting an existing file. The generated file is a preview contract;
the scanner does not load it in the current release.

## `debtdrone config`

```text
debtdrone config list
debtdrone config get <key>
debtdrone config set <key> <value>
debtdrone config unset <key>
```

`config list` prints every effective value, type, source, and description. It
supports `--format text|json`. Sources are `default`, `config_file`, or
`environment`; explicit scan flags and MCP arguments override those values for
their invocation.

`config get <key>` prints the effective value and supports `--format text|json`.
`config set` validates and atomically persists one supported dotted key.
`config unset` removes only its config-file override. Unknown keys and invalid
values fail without modifying the file.

These commands manage the OS-native user configuration file, not the legacy
repository `.debtdrone.yaml` template.

## `debtdrone history`

```text
debtdrone history [flags]
debtdrone history list [flags]
debtdrone history show <id> [flags]
debtdrone history delete <id>
debtdrone history clear [--force]
```

Running `debtdrone history` without a subcommand is equivalent to `history
list`, preserving the original command shape.

| List flag | Default | Description |
|---|---|---|
| `-f`, `--format string` | `text` | Select `text` or `json` output |
| `--limit int` | `10` | Return between 1 and 200 newest entries |

`history list` returns newest-first summaries. Text mode reports an explicit
empty state; JSON mode returns `[]`, making it safe for scripts. Each entry
includes its stable UUID, UTC timestamps, repository display name, outcome,
severity counts, technical-debt hours, warnings, and analyzer failures.

`history show <id>` renders one complete stored summary and supports
`--format text|json`. `history delete <id>` removes only that record. Missing or
invalid IDs return a non-zero exit with an actionable error.

`history clear` prompts you to type `yes` before removing all summaries. Scripts
must pass `--force` to skip the prompt:

```bash
debtdrone history clear --force
```

Corrupt or incompatible stores are never overwritten. The command reports the
history path and recovery guidance instead.

## `debtdrone completion`

```text
debtdrone completion [bash|fish|powershell|zsh]
```

Each shell subcommand prints an installation script and shell-specific setup
instructions. Bash, Fish, PowerShell, and Zsh expose
`--no-descriptions` to omit completion descriptions:

```bash
debtdrone completion zsh --help
```

## Interactive commands

The full-screen TUI uses slash commands rather than Cobra subcommands:

| Command | Purpose |
|---|---|
| `/scan [path]` | Scan the supplied directory, or the launch directory when omitted |
| `/history` | Browse scans completed in the current TUI session |
| `/config` | Edit session settings initialized from resolved local configuration |
| `/update` | Check for and apply a CLI release |
| `/help` | Show TUI help |
| `/quit` | Exit the TUI |

There is no headless `debtdrone update` command. See the
[Interactive TUI guide](../tui-usage/) for keys and view behavior.
