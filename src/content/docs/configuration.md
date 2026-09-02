---
title: Configuration
description: Understand DebtDrone's versioned local settings, validation, storage paths, and precedence rules.
---

DebtDrone defines one versioned local configuration contract for the headless
CLI, MCP server, and interactive TUI. The shared resolver applies
settings in this order, from lowest to highest priority:

1. built-in defaults
2. the local configuration file
3. `DEBTDRONE_*` environment variables
4. explicitly supplied command flags

Only a value that is present in a higher-priority layer replaces the resolved
value. Explicit `false` and `0` values are preserved; they are not treated as
missing.

The `config list`, `get`, `set`, and `unset` commands manage this user-level
file. Headless, MCP, and TUI scans use the resolved values. Explicit headless
flags and MCP tool arguments remain the highest-priority overrides.

## Local configuration path

DebtDrone builds the path from the operating system's user configuration
directory instead of hard-coding a home-directory layout.

| Platform | Default path |
|---|---|
| Linux and other Unix systems | `$XDG_CONFIG_HOME/debtdrone/config.yaml`, or `$HOME/.config/debtdrone/config.yaml` when `XDG_CONFIG_HOME` is unset |
| macOS | `$HOME/Library/Application Support/debtdrone/config.yaml` |
| Windows | `%AppData%\debtdrone\config.yaml` |

The file is optional. A missing file contributes no overrides, so built-in
defaults still resolve normally. Read failures and malformed files are returned
to the caller instead of being silently ignored.

Writes use an owner-only temporary file, flush it, and atomically replace the
previous file. A cross-process lock prevents concurrent commands from losing
independent updates. Existing application-directory permissions are preserved;
a missing directory and new configuration file are created with owner-only
permissions where Unix permission bits are available.

## Version 1 schema

Every file must declare its schema version:

```yaml
version: 1

scan:
  output_format: text
  fail_on: none
  max_complexity: 15
  security_scan: true
  coverage: false

update:
  checks: true

ui:
  show_line_numbers: true
  max_results: 500

history:
  enabled: true
```

All sections and settings are optional after `version`. Omitted settings use
the next available precedence layer.

| Key | Environment variable | Default | Validation |
|---|---|---|---|
| `scan.output_format` | `DEBTDRONE_OUTPUT_FORMAT` | `text` | `text` or `json` |
| `scan.fail_on` | `DEBTDRONE_FAIL_ON` | `none` | `none`, `low`, `medium`, `high`, or `critical` |
| `scan.max_complexity` | `DEBTDRONE_MAX_COMPLEXITY` | `15` | Integer from `1` through `10000` |
| `scan.security_scan` | `DEBTDRONE_SECURITY_SCAN` | `true` | `true` or `false` |
| `scan.coverage` | `DEBTDRONE_COVERAGE` | `false` | `true` or `false` |
| `update.checks` | `DEBTDRONE_UPDATE_CHECKS` | `true` | `true` or `false` |
| `ui.show_line_numbers` | `DEBTDRONE_SHOW_LINE_NUMBERS` | `true` | `true` or `false` |
| `ui.max_results` | `DEBTDRONE_MAX_RESULTS` | `500` | Integer from `0` through `100000`; `0` means unlimited |
| `history.enabled` | `DEBTDRONE_HISTORY_ENABLED` | `true` | `true` or `false` |

## Precedence examples

Given this file:

```yaml
version: 1
scan:
  output_format: json
  max_complexity: 20
  security_scan: false
```

this environment value replaces only the configured complexity threshold:

```bash
export DEBTDRONE_MAX_COMPLEXITY=25
```

and this explicit flag wins over both:

```bash
debtdrone scan . --max-complexity=30
```

The result is JSON output, a complexity threshold of `30`, and security scanning
disabled. Every resolved setting retains its source (`default`, `config_file`,
`environment`, or `flag`) so commands can explain why a value was selected.

## Strict validation and compatibility

Configuration parsing is intentionally strict:

- unknown sections or keys are rejected with the YAML line and field name;
- unknown variables using the reserved `DEBTDRONE_*` prefix list the supported
  environment variables;
- invalid booleans, integers, enum values, and ranges name the affected key;
- empty environment values are rejected instead of being interpreted as
  defaults;
- multiple YAML documents in one file are rejected;
- read errors include the configuration path.

`set` and `unset` validate the complete current file before changing it. They
preserve YAML comments on retained settings. Unknown current-version fields and
newer schema versions are rejected rather than silently discarded; the error
explains whether to fix, upgrade, or move the file aside. Temporary-write or
replacement failures leave the previous file intact.

DebtDrone currently supports schema version `1`. A file without `version` is
invalid. An older version requests migration; a version newer than the running
binary fails with an instruction to upgrade DebtDrone. A newer file is never
partially interpreted as an older schema, which prevents unknown settings from
being silently discarded.

## Configure a headless scan

Persist reusable defaults with `debtdrone config set`, then override only the
values a particular automation run needs:

```bash
debtdrone scan . \
  --format=json \
  --fail-on=high \
  --max-complexity=15 \
  --security-scan=true \
  --coverage=false
```

MCP calls follow the same model: omitted `max_complexity`, `security_scan`, and
`coverage` inputs use the resolved local values, while explicit tool inputs win.
The mandatory MCP `--root` boundary is never read from stored configuration.

:::caution[Trivy availability]
Security scanning requires `trivy` on `PATH`. When Trivy is absent, DebtDrone
skips that analyzer. When Trivy starts but cannot complete, the CLI prints the
available findings and returns a partial-scan error.
:::

## Manage persistent values

List every effective value together with its type and winning source:

```bash
debtdrone config list
debtdrone config list --format=json
```

Inspect or update one supported dotted key:

```bash
debtdrone config get scan.max_complexity
debtdrone config set scan.max_complexity 20
debtdrone config unset scan.max_complexity
```

`unset` removes only the config-file override. A later `get` may therefore show
an environment value or built-in default. Commands never print raw environment
entries; only validated effective values from the supported allowlist appear.

The TUI settings screen starts from the same effective values. Changes made in
that screen are session overrides; use `debtdrone config set` when a value
should persist across launches. Disabling `history.enabled` prevents CLI, MCP,
and TUI scans from writing new local summaries without deleting existing ones.

## Repository template

`debtdrone init` still generates the legacy `.debtdrone.yaml` repository
template. That repository file is not the versioned user configuration above
and is not loaded by current scans.

For TUI behavior, see
[Interactive TUI configuration](../tui-usage/#config--interactive-settings-editor).
For the complete current command surface, see the
[command reference](../command-reference/).
