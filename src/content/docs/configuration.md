---
title: Configuration
description: Configure current DebtDrone scans with CLI flags and understand the status of repository configuration.
---

DebtDrone scans are currently configured with command-line flags or with the
session-only settings in the interactive TUI. The CLI can generate a
`.debtdrone.yaml` template, but the current scanner does **not** load values
from that file yet.

## Configure a headless scan

Pass settings directly to `debtdrone scan`:

```bash
debtdrone scan . \
  --format=json \
  --fail-on=high \
  --max-complexity=15 \
  --security-scan=true
```

| Flag | Default | Purpose |
|---|---|---|
| `--format` | `text` | Select `text` or `json` output |
| `--fail-on` | unset | Return a non-zero exit when a finding reaches the selected severity |
| `--max-complexity` | `15` | Report high cyclomatic complexity above this value; critical starts above twice this value |
| `--security-scan` | `true` | Enable the Trivy analyzer |
| `--coverage` | `false` | Parse existing coverage artifacts without running tests |

Flags use their built-in defaults on every invocation. A committed
`.debtdrone.yaml` does not currently override those defaults.

:::caution[Trivy availability]
Security scanning requires `trivy` on `PATH`. When Trivy is absent, DebtDrone
skips that analyzer. When Trivy starts but cannot complete, the CLI prints the
available findings and returns a partial-scan error.
:::

## Generate the repository template

Run `debtdrone init` from the repository root:

```bash
debtdrone init
```

DebtDrone creates `.debtdrone.yaml` and prints:

```text
Initialized .debtdrone.yaml successfully.
```

The generated template is:

```yaml
quality_gate:
  fail_on: high

thresholds:
  max_complexity: 15
  security_scan: true

ignore_paths:
  - "node_modules"
  - "vendor"
  - "dist"
  - ".git"
```

Treat this file as a preview of the planned repository-owned configuration
contract. Creating or editing it does not change scan behavior in the current
release.

## Inspect the current defaults

`debtdrone config list` prints the settings represented by the CLI and TUI:

```bash
debtdrone config list
```

```text
KEY                   VALUE   TYPE     DESCRIPTION
Output Format         text    string   Render mode for scan results (text/json)
Auto-Update Checks    true    bool     Check for newer releases on startup
Fail on Severity      high    string   Min severity for non-zero exit code
Max Complexity        15      int      Cyclomatic-complexity threshold per function
Security Scan         true    bool     Run Trivy vulnerability detection
Show Line Numbers     true    bool     Include line:col in the results list
Max Results           500     int      Cap on issues rendered per scan
```

The command currently exposes text output only. It reports the built-in
defaults; it does not read `.debtdrone.yaml`.

## Current `config set` limitation

`debtdrone config set <key> <value>` is a compatibility placeholder. It prints
an acknowledgement but does not validate the key, update the running scanner,
or persist a file. Do not use it for automation yet.

To change a headless scan today, pass the corresponding flag directly. For TUI
behavior, see [Interactive TUI configuration](../tui-usage/#config--interactive-settings-editor).
For the complete current command surface, see the
[command reference](../command-reference/).
