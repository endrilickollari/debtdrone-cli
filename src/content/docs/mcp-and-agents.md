---
title: MCP and agents
description: Understand the current agent workflow and the status of DebtDrone MCP support.
---

DebtDrone's reusable scanner is suitable for local agent workflows, but the
current CLI release does **not** expose a Model Context Protocol (MCP) server.
There is no MCP command, server executable, or supported MCP client
configuration to add yet.

## Use the CLI from an agent today

An agent with permission to run local commands can invoke the headless scanner
directly:

```bash
debtdrone scan /absolute/path/to/repository \
  --format=json \
  --security-scan=false
```

Use an explicit repository path so the target does not depend on the agent's
working directory. Parse stdout as a JSON array and retain stderr separately
for analyzer warnings.

Add a quality gate only when the caller is prepared for a non-zero exit:

```bash
debtdrone scan /absolute/path/to/repository \
  --format=json \
  --fail-on=high \
  --security-scan=false
```

The same non-zero status can also report invalid input or a partial analyzer
failure, so agents should inspect stderr instead of treating every failure as a
quality-gate violation.

## Trust boundary

- The scan target is local and explicitly selected by the caller.
- Static scanning does not require SaaS credentials or organization context.
- Trivy may inspect dependencies and secrets when `--security-scan=true`.
- `--coverage` parses existing artifacts; the CLI does not execute repository tests.
- SaaS persistence, billing, integrations, notifications, and hosted execution remain outside the CLI scanner.

## Planned MCP boundary

The planned first MCP integration belongs in this open-source repository and
will wrap the same neutral scanner API used by the CLI and TUI. Its intended
first boundary is local, read-only, and stdio-based. Configuration examples
will be published only after the server and tool contract exist, so agents do
not depend on a speculative command or schema.

Until then, use the command reference above and do not configure a
`debtdrone` MCP server in an agent client.

See [System architecture](../architecture/) and
[Scanner ownership](../ownership/) for the reusable scanner and SaaS boundary.

