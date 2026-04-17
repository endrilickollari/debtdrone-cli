# CI/CD & Headless CLI

DebtDrone's headless CLI mode is built on [Cobra](https://github.com/spf13/cobra) and is designed for integration into automated pipelines. Every subcommand produces predictable, scriptable output and respects standard POSIX exit codes.

---

## Subcommand Overview

| Subcommand | Purpose |
|---|---|
| `debtdrone scan <path>` | Analyze a directory for technical debt |
| `debtdrone init` | Bootstrap a `.debtdrone.yaml` config file |
| `debtdrone config list` | Print all current settings |
| `debtdrone config set <key> <value>` | Update a single setting headlessly |
| `debtdrone history` | List previous scan runs |

---

## `debtdrone scan`

The `scan` subcommand runs the full analysis engine against a given path and writes results to stdout.

### Syntax

```bash
debtdrone scan [path] [flags]
```

If `path` is omitted, the current directory (`.`) is used.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--format` | `text` | Output format: `text` or `json` |
| `--fail-on` | _(none)_ | Exit `1` if debt of this severity or higher is found: `critical`, `high`, `medium`, `low` |
| `--max-complexity` | `15` | Cyclomatic complexity threshold for raising a finding |
| `--security-scan` | `true` | Enable Trivy-based vulnerability and secrets scanning |

### Text Output

```bash
debtdrone scan ./src --format=text
```

Produces a human-readable table of findings suitable for log tailing:

```
SEVERITY   FILE                        LINE  FUNCTION            DEBT
critical   internal/api/handler.go     112   ProcessRequest      85 min
high       internal/db/query_builder.go 44   BuildDynamicQuery   40 min
medium     pkg/parser/tokenizer.go      88   Tokenize            22 min
...

Total findings: 14  |  Total debt: 4h 32min
```

### JSON Output

```bash
debtdrone scan ./src --format=json
```

Emits a structured JSON payload, suitable for downstream processing with `jq` or ingestion into a reporting dashboard:

```json
{
  "run_id": "d3f1a2b4-...",
  "scanned_at": "2026-04-17T09:00:00Z",
  "path": "./src",
  "total_debt_minutes": 272,
  "summary": {
    "critical": 1,
    "high": 2,
    "medium": 6,
    "low": 5
  },
  "findings": [
    {
      "file_path": "internal/api/handler.go",
      "line_number": 112,
      "function_name": "ProcessRequest",
      "issue_type": "complexity",
      "severity": "critical",
      "cyclomatic_complexity": 28,
      "cognitive_complexity": 19,
      "nesting_depth": 6,
      "parameter_count": 4,
      "loc": 145,
      "debt_minutes": 85,
      "message": "Function exceeds cyclomatic complexity threshold of 15",
      "suggestions": [
        "Extract nested conditional blocks into named helper functions",
        "Consider using a strategy pattern to replace the switch statement"
      ]
    }
  ]
}
```

!!! tip "Filtering with `jq`"
    Parse JSON output with `jq` to build custom reports:
    ```bash
    # List all critical findings
    debtdrone scan . --format=json | jq '.findings[] | select(.severity == "critical")'

    # Sum total debt in hours
    debtdrone scan . --format=json | jq '.total_debt_minutes / 60'
    ```

---

## Quality Gates — `--fail-on`

The `--fail-on` flag is the primary integration point for CI/CD pipelines. When set, `debtdrone scan` exits with **`os.Exit(1)`** if any finding at or above the specified severity is present. Pipelines that treat non-zero exits as failures will block the merge automatically.

### Severity Ladder

```
critical  ←  most severe
high
medium
low       ←  least severe
```

Setting `--fail-on=high` causes the command to exit `1` if any `high` **or** `critical` finding is detected. Lower severities (`medium`, `low`) are reported but do not trigger a failure.

### Exit Codes

| Exit Code | Meaning |
|---|---|
| `0` | Scan completed; no findings at or above the specified threshold |
| `1` | Findings at or above `--fail-on` were detected, OR the scan itself failed (e.g., invalid path) |

!!! warning "No `--fail-on` set"
    If `--fail-on` is not provided (and not set in `.debtdrone.yaml`), `debtdrone scan` always exits `0`, even if critical debt is found. This is intentional for informational-only pipelines. Add `--fail-on` explicitly or set `quality_gate.fail_on` in your config file to enforce a gate.

### Example

```bash
# Block the build on any HIGH or CRITICAL debt
debtdrone scan ./src --fail-on=high

# Block only on CRITICAL
debtdrone scan ./src --fail-on=critical

# Informational scan — always exits 0
debtdrone scan ./src
```

---

## GitHub Actions Integration

The following workflow step runs DebtDrone as a quality gate on every pull request. The build fails if any `high` or `critical` debt is introduced.

```yaml
# .github/workflows/debt-gate.yml
name: Technical Debt Gate

on:
  pull_request:
    branches:
      - main
      - develop

jobs:
  debt-analysis:
    name: DebtDrone Quality Gate
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Install DebtDrone
        run: go install github.com/endrilickollari/debtdrone-cli/cmd/debtdrone@latest

      - name: Run debt analysis
        run: debtdrone scan . --format=json --fail-on=high | tee debt-report.json

      - name: Upload debt report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: debt-report
          path: debt-report.json
```

!!! tip "Uploading the artifact"
    The `tee` command above writes the JSON report to a file while still streaming to stdout (so the exit code propagates correctly). The artifact upload step uses `if: always()` to ensure the report is saved even when the gate fails — giving developers a detailed breakdown of what triggered the failure.

!!! note "Using `.debtdrone.yaml` in CI"
    If a `.debtdrone.yaml` file is committed at the repository root, DebtDrone picks it up automatically. You can set `quality_gate.fail_on` there instead of passing `--fail-on` on every invocation. See [Configuration Management](configuration.md) for details.

---

## `debtdrone history`

List all scan runs recorded on the current machine.

```bash
debtdrone history [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--format` | `text` | Output format: `text` or `json` |
| `--limit` | `10` | Maximum number of entries to display |

```bash
# Show the 5 most recent scans as JSON
debtdrone history --limit=5 --format=json
```

Text output:

```
DATE                 REPOSITORY          ISSUES  CRITICAL  HIGH
2026-04-17 09:00     ./src               14      1         2
2026-04-15 14:32     ./src               18      2         3
2026-04-10 11:15     ./src               22      3         5
```
