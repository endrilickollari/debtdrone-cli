---
title: CI/CD and headless CLI
description: Run DebtDrone scans and enforce severity-based quality gates in automation.
---

Use `debtdrone scan` in CI/CD pipelines and scripted workflows. Findings are
written to stdout, analyzer warnings are written to stderr, and a configured
quality gate returns a non-zero exit when its threshold is reached.

## Run a scan

```bash
debtdrone scan [path] [flags]
```

The path defaults to the current directory.

| Flag | Default | Description |
|---|---|---|
| `--format` | `text` | Output `text` or `json` |
| `--fail-on` | unset | Fail on `critical`, `high`, `medium`, or `low` findings |
| `--max-complexity` | `15` | Set the cyclomatic-complexity threshold per function |
| `--security-scan` | `true` | Enable Trivy vulnerability scanning |
| `--coverage` | `false` | Parse existing coverage artifacts without running tests |

## Text output

```bash
debtdrone scan ./src --format=text --security-scan=false
```

Text output is a table with one row per finding:

```text
SEVERITY   FILE:LINE                         RULE   MESSAGE
--------   ---------                         ----   -------
CRITICAL   /workspace/src/handler.go:112     N/A    Function 'ProcessRequest' has high cyclomatic complexity of 28 (threshold: 10)
```

If no findings are produced, the command prints:

```text
No technical debt issues found.
```

## JSON output

```bash
debtdrone scan ./src --format=json --security-scan=false
```

JSON output is an array of `TechnicalDebtIssue` objects. This abbreviated
example shows the fields most commonly used in automation; actual objects also
contain identifiers, status, confidence, timestamps, and optional context:

```json
[
  {
    "file_path": "/workspace/src/handler.go",
    "line_number": 112,
    "issue_type": "complexity",
    "severity": "critical",
    "category": "complexity",
    "message": "Function 'ProcessRequest' has high cyclomatic complexity of 28 (threshold: 10)",
    "tool_name": "complexity_analyzer",
    "technical_debt_hours": 1.42
  }
]
```

Filter the array directly with `jq`:

```bash
# Print critical findings
debtdrone scan . --format=json --security-scan=false \
  | jq '.[] | select(.severity == "critical")'

# Sum estimated debt hours
debtdrone scan . --format=json --security-scan=false \
  | jq '[.[].technical_debt_hours] | add // 0'
```

Coverage and analyzer warnings remain on stderr, so stdout stays valid JSON.

## Enforce a quality gate

`--fail-on` fails when a finding matches or exceeds the selected severity:

```text
critical  ← most severe
high
medium
low       ← least severe
```

For example, `--fail-on=high` fails on `high` and `critical` findings:

```bash
debtdrone scan . --fail-on=high
```

| Exit code | Meaning |
|---|---|
| `0` | Scan completed without a gate violation |
| `1` | The gate failed, input was invalid, or the scan completed with an analyzer error |

If `--fail-on` is omitted, findings alone do not fail the command. Analyzer
errors can still produce a non-zero exit. `.debtdrone.yaml` does not currently
set a gate; pass the flag explicitly.

## GitHub Actions example

```yaml
name: Technical Debt Gate

on:
  pull_request:
    branches: [main]

jobs:
  debt-analysis:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Install DebtDrone
        run: go install github.com/endrilickollari/debtdrone-cli/v2/cmd/debtdrone@latest

      - name: Run debt analysis
        run: debtdrone scan . --format=json --fail-on=high | tee debt-report.json

      - name: Upload debt report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: debt-report
          path: debt-report.json
```

GitHub Actions enables pipeline failure propagation for the shell step, so a
quality-gate error still fails the job while `tee` preserves stdout as an
artifact.

## Parse coverage artifacts

Coverage analysis is opt-in:

```bash
debtdrone scan . --coverage --format=json
```

The CLI discovers supported Go coverage, LCOV, Cobertura, JaCoCo, Clover, and
SimpleCov files. It parses existing artifacts only; it does not run tests,
package managers, build tools, or Docker. Missing or malformed artifacts
produce warnings on stderr.

## Configuration and history commands

- `debtdrone init` creates a `.debtdrone.yaml` template, but scans do not load
  that file yet.
- `debtdrone config list` prints static defaults; `config set` does not persist
  changes yet.
- `debtdrone history` currently returns demonstration entries rather than a
  persistent record of prior CLI runs.

See [Configuration](../configuration/) for the exact current limitations.
