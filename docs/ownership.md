# Scanner ownership and contribution boundary

This repository owns the open-source DebtDrone CLI and the reusable scanning
engine consumed by the paid SaaS product. The public Go module is
`github.com/endrilickollari/debtdrone-cli/v2`; the supported consumer boundary
is its `scanner` packages.

## What belongs here

- the public scanner API, options, progress events, failures, and report types;
- complexity, line-count, security, coverage, and repository-structure logic;
- file filtering and full, incremental, and explicit no-change semantics;
- cancellation, bounded concurrency, deterministic results, and neutral
  isolated-execution interfaces;
- CLI/TUI presentation, local configuration/history, installers, and releases;
- scanner unit, golden, compatibility, and external-consumer tests.

## What stays in the SaaS

The `debtdrone` SaaS repository owns hosted workers and queues, database/Redis
persistence, issue reconciliation, authentication, organizations, plans,
billing, repository credentials, dashboards, notifications, Jira, Trello,
GitHub, AI features, and the React application. Those concerns must not be
introduced into this module.

## Cross-repository workflow

1. Implement reusable scanner behavior here first.
2. Preserve the public contract and add tests that exercise both the scanner
   package and CLI consumer where relevant.
3. Merge without creating a release unless a maintainer explicitly approves a
   version.
4. Publish through the test-gated Release workflow described in
   [Versioning and releases](versioning.md).
5. Let release automation open a SaaS dependency-update pull request for the
   exact immutable version.
6. Put only adaptation, persistence, and hosted product behavior in that SaaS
   pull request.

For local coordinated work, a Go workspace may point the SaaS at a sibling
checkout. Local workspaces and `replace` directives are development tools and
must not be committed as the production dependency.

Do not copy scanner files between repositories and do not recreate the retired
`publish_cli.py` synchronization path. If a feature cannot be expressed without
SaaS types, split the neutral scanner capability from its SaaS adapter.
