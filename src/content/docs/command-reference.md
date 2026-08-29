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
| `init` | Generate a preview `.debtdrone.yaml` file |
| `config list` | Print static configuration defaults |
| `config set [key] [value]` | Compatibility placeholder; does not persist changes |
| `history` | Print demonstration history entries |
| `completion` | Generate shell-completion scripts |

:::caution[Current persistence limitations]
The current release does not load `.debtdrone.yaml`, persist values supplied to
`config set`, or persist headless scan history. Use `scan` flags for automation.
:::

## `debtdrone scan`

```text
debtdrone scan [path] [flags]
```

`path` is optional and defaults to `.`. At most one path is accepted.

| Flag | Shorthand | Default | Description |
|---|---|---|---|
| `--format string` | `-f` | `text` | Select `text` or `json` output |
| `--fail-on string` | — | unset | Fail on `critical`, `high`, `medium`, or `low` and above |
| `--max-complexity int` | — | `15` | Report high cyclomatic complexity above this value; critical starts above twice this value |
| `--security-scan` | — | `true` | Enable the Trivy analyzer |
| `--coverage` | — | `false` | Parse supported coverage artifacts already present in the repository |

Boolean flags support Cobra's explicit form when you need to turn a default on
or off:

```bash
debtdrone scan . --security-scan=false --coverage=true
```

Text output is a findings table. JSON output is an array of
`TechnicalDebtIssue` objects. Warnings use stderr. A non-zero exit can mean a
quality-gate violation, invalid input, or an analyzer error after partial
results were printed.

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
debtdrone config set [key] [value]
```

`config list` prints a text table of static defaults. It has no JSON flag and
does not read `.debtdrone.yaml`.

`config set` requires exactly two arguments and prints an acknowledgement. It
does not validate the key, change a scan, or write a configuration file.

## `debtdrone history`

```text
debtdrone history [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--format string` | `text` | Select `text` or `json` output |
| `--limit int` | `10` | Limit the number of returned entries |

The current command returns demonstration data generated at invocation time.
It is not a persistent record of prior headless or TUI scans.

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
| `/config` | Edit session-only TUI settings |
| `/update` | Check for and apply a CLI release |
| `/help` | Show TUI help |
| `/quit` | Exit the TUI |

There is no headless `debtdrone update` command. See the
[Interactive TUI guide](../tui-usage/) for keys and view behavior.
