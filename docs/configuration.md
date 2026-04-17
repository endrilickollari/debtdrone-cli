# Configuration Management

DebtDrone follows a **"Docs as Code"** approach to configuration: settings that govern how your codebase is analyzed live in a `.debtdrone.yaml` file committed alongside the source it describes. Every developer and every CI runner uses the same thresholds, the same ignored paths, and the same quality gate — because the config is part of the repository.

---

## Initializing Configuration — `debtdrone init`

Run `debtdrone init` once at the root of a repository to create a `.debtdrone.yaml` with sensible defaults:

```bash
cd /path/to/your/repo
debtdrone init
```

This writes a `.debtdrone.yaml` file and prints a confirmation:

```
Created .debtdrone.yaml with default configuration.
Commit this file to share settings with your team.
```

!!! tip "Commit the config file"
    Add `.debtdrone.yaml` to version control immediately after running `debtdrone init`. This ensures every developer and CI runner uses the same thresholds, and changes to analysis standards are reviewed through your normal pull request process.

---

## The `.debtdrone.yaml` Format

Below is a fully annotated sample configuration file:

```yaml
# .debtdrone.yaml

# Quality Gate — controls CI/CD behavior
quality_gate:
  # Fail the build (exit 1) if any finding at this severity or higher is detected.
  # Valid values: critical | high | medium | low | none
  # "none" disables the quality gate entirely (scan always exits 0).
  fail_on: high

# Analysis thresholds — tune what gets flagged
thresholds:
  # Cyclomatic complexity value above which a finding is raised.
  # Lower values enforce stricter code simplicity standards.
  max_complexity: 15

  # Enable Trivy-based scanning for CVEs in dependencies and secrets in code.
  security_scan: true

# Paths to exclude from analysis (relative to repository root).
# Supports glob patterns.
ignore_paths:
  - "node_modules"
  - "vendor"
  - "dist"
  - ".git"
  - "**/*_test.go"    # Exclude test files from complexity analysis
  - "migrations/**"   # Exclude generated migration files
```

### Configuration Keys Reference

| Key | Type | Default | Description |
|---|---|---|---|
| `quality_gate.fail_on` | string | `high` | Severity threshold for `os.Exit(1)` |
| `thresholds.max_complexity` | int | `15` | Cyclomatic complexity threshold |
| `thresholds.security_scan` | bool | `true` | Enable Trivy vulnerability scanning |
| `ignore_paths` | list | `[node_modules, vendor, dist, .git]` | Glob patterns for excluded paths |

!!! note "Flag precedence"
    CLI flags take precedence over `.debtdrone.yaml` values, which take precedence over built-in defaults. This means you can override a committed config for a single run without modifying the file:
    ```bash
    # Temporarily lower the gate to catch medium debt without changing the config
    debtdrone scan . --fail-on=medium
    ```

---

## Managing Configuration Headlessly

For scripted environments or onboarding automation, DebtDrone provides `debtdrone config` subcommands that read and write `.debtdrone.yaml` without a text editor.

### `debtdrone config list`

Prints all current settings in a formatted table:

```bash
debtdrone config list
```

```
SETTING                 VALUE
Output Format           text
Auto-Update Checks      true
Fail on Severity        high
Max Complexity          15
Security Scan           true
Show Line Numbers       true
Max Results             100
```

Add `--format=json` for machine-readable output:

```bash
debtdrone config list --format=json
```

### `debtdrone config set`

Update a single setting by key:

```bash
debtdrone config set <key> <value>
```

Valid keys and their accepted values:

| Key | Accepted Values |
|---|---|
| `fail_on` | `critical` \| `high` \| `medium` \| `low` \| `none` |
| `max_complexity` | Any positive integer |
| `security_scan` | `true` \| `false` |
| `output_format` | `text` \| `json` |
| `auto_update` | `true` \| `false` |
| `show_line_numbers` | `true` \| `false` |
| `max_results` | Any positive integer |

**Examples:**

```bash
# Raise the quality gate to only fail on critical debt
debtdrone config set fail_on critical

# Lower the complexity threshold for a strict codebase
debtdrone config set max_complexity 10

# Disable security scanning for a run environment without Trivy installed
debtdrone config set security_scan false
```

Each `config set` call modifies `.debtdrone.yaml` in place and prints a confirmation:

```
Updated: fail_on = critical
```

!!! warning "Trivy dependency"
    Setting `security_scan: true` requires [Trivy](https://trivy.dev/) to be installed and on the `PATH`. If `trivy` is not found, DebtDrone logs a warning and skips security analysis rather than failing the entire scan. Install Trivy via its [official installation guide](https://aquasecurity.github.io/trivy/latest/getting-started/installation/) to enable this feature.

---

## Using the Interactive Config Editor

If you prefer a guided approach, the TUI's `/config` command presents the same settings as an interactive form. See [Interactive TUI Explorer — `/config`](tui-usage.md#config--interactive-settings-editor) for details.
