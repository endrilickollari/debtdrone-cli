---
title: Troubleshooting
description: Diagnose common DebtDrone installation, scan, output, TUI, and configuration problems.
---

Start by recording the installed version and the exact command that failed:

```bash
debtdrone --version
debtdrone scan . --security-scan=false
```

Disabling the optional security analyzer is a useful first isolation step. It
does not disable complexity or line-count analysis.

## `debtdrone: command not found`

If you installed with `go install`, ensure Go's binary directory is on `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Persist that line in your shell profile after confirming it works. For binary
and Homebrew installation options, see [Install the CLI](../installation/).

## Building from source fails with a compiler error

DebtDrone's tree-sitter analyzers use CGO. Install a C compiler such as `clang`
or `gcc`, confirm `CGO_ENABLED=1`, and use Go 1.25.1 or later:

```bash
go version
go env CGO_ENABLED
go build -o debtdrone ./cmd/debtdrone
```

## Trivy is missing or cannot complete

Security analysis requires the `trivy` executable on `PATH`. When Trivy is not
available, DebtDrone reports a warning and continues with the other analyzers.
When Trivy starts but fails, DebtDrone can print valid partial results and then
return a non-zero status.

To isolate the scanner from Trivy:

```bash
debtdrone scan . --security-scan=false
```

Install or repair Trivy before re-enabling security analysis.

## JSON parsing fails in a pipeline

Request JSON explicitly and keep stdout and stderr separate:

```bash
debtdrone scan . --format=json --security-scan=false > debt-report.json
```

The JSON document is an array, not an object with a `findings` property:

```bash
jq '.[] | select(.severity == "critical")' debt-report.json
```

If the command returns non-zero, the report can still contain valid partial
results. Inspect stderr for the analyzer or quality-gate error.

## A scan fails even though JSON was produced

DebtDrone prints available results before returning a partial analyzer error.
It also returns non-zero when `--fail-on` finds a matching severity. Check the
error text and rerun without the quality gate when isolating the cause:

```bash
debtdrone scan . --format=json --security-scan=false
```

## Configuration changes have no effect on scans

This is expected in the current release. `debtdrone init` generates a preview
repository file, but scans do not load it. The user-level `debtdrone config set`
command persists values safely, but scan, MCP, and TUI entry points do not apply
them until the remaining integration lands.

Pass scan settings as flags instead:

```bash
debtdrone scan . --max-complexity=12 --fail-on=high
```

If a config command reports malformed or incompatible YAML, follow the path in
the error. Fix the file, upgrade DebtDrone for a newer schema, or move the file
aside before retrying; DebtDrone will not overwrite data it cannot validate.

## `history` reports a corrupt or incompatible store

DebtDrone refuses to overwrite malformed history or a schema version it cannot
read. The error identifies the local `history.json` path and whether you should
upgrade DebtDrone or move the file aside. Preserve the original file when you
may need to inspect or migrate its records later.

The TUI history browser still shows only scans completed during the current
process. Use `debtdrone history list` to inspect summaries persisted by earlier
headless or TUI scans.

## The TUI updater cannot replace the binary

The `/update` workflow replaces the running executable. A binary installed in
a protected directory may not be writable by the current user. Prefer updating
through the original installation method, such as:

```bash
brew upgrade debtdrone
```

## An MCP client cannot connect

First confirm that the installed binary exposes the server and that the
configured root exists:

```bash
debtdrone mcp --help
command -v debtdrone
test -d /absolute/path/to/repository
test -x /absolute/path/to/repository
```

The agent may not inherit your shell `PATH`; use the executable's absolute
path in that case. Keep the transport set to stdio and never merge stderr into
stdout, which is reserved for protocol messages.

The [MCP and coding agents guide](../mcp-and-agents/#troubleshoot-the-connection)
covers Codex and Claude Code status checks, permissions, stderr diagnostics,
protocol failures, and scan timeouts.

## Report a reproducible problem

Search the [GitHub issues](https://github.com/endrilickollari/debtdrone-cli/issues)
before opening a report. Include:

- `debtdrone --version` output;
- operating system and architecture;
- the exact command and exit status;
- stderr output with secrets and private paths removed; and
- a minimal public reproduction when possible.

Do not post suspected vulnerabilities publicly. Follow the private contact
instructions in the repository's [Security Policy](https://github.com/endrilickollari/debtdrone-cli/security/policy).
