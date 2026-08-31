---
title: Contributing
description: Build, test, document, and contribute to the open-source DebtDrone scanner and CLI.
---

DebtDrone CLI and its reusable scanner are open source. Before changing code,
read the repository's complete
[contribution guide](https://github.com/endrilickollari/debtdrone-cli/blob/main/CONTRIBUTING.md)
and the [scanner ownership boundary](../ownership/).

## Choose the correct repository

Contribute reusable scanning, analyzer, CLI, TUI, local configuration/history,
installer, release, and documentation behavior here.

SaaS workers, queues, persistence, organizations, billing, notifications,
hosted integrations, AI enrichment, and the web application belong in the
private SaaS repository. Do not copy scanner implementations between them.

## Prepare a local checkout

Prerequisites:

- Go 1.25.1 or later;
- a C compiler for CGO and tree-sitter;
- Node.js 22.12 or later for documentation work; and
- Docker only for cross-platform release snapshots or explicit isolated coverage work.

```bash
git clone https://github.com/endrilickollari/debtdrone-cli.git
cd debtdrone-cli
go mod download
go test ./...
go build -o dist/debtdrone ./cmd/debtdrone
```

## Work on documentation

Install exactly the dependency versions in `package-lock.json` and start the
Starlight development server:

```bash
npm ci
npm run dev
```

Before opening a pull request, produce the same static build used by CI:

```bash
npm run build
```

Documentation source lives in `src/content/docs`, assets imported by pages live
in `src/assets`, and static public files live in `public`. Navigation and site
metadata are defined in `astro.config.mjs`.

Validate examples against the current binary instead of copying old output:

```bash
go run ./cmd/debtdrone --help
go run ./cmd/debtdrone scan --help
```

### Maintain visual documentation

The documentation visual system mirrors the DebtDrone landing page: Geist for
interface text, Geist Mono for commands and labels, matte charcoal surfaces,
fine white hairlines, and lime only for active signals. Shared tokens and
responsive visual framing live in `src/styles/custom.css`.

Editable diagrams live in `src/assets/diagrams` as source SVG files. When a
workflow changes:

1. update the labels and paths in the relevant SVG;
2. keep its internal `<title>` and `<desc>` accurate;
3. update the Markdown image alt text and adjacent prose or caption;
4. verify labels remain readable at a 320-pixel viewport; and
5. run `npm run build` before opening a pull request.

TUI screenshots must show the current executable rather than a recreated
mockup. Capture them in a dark terminal with the repository font and color
profile, crop away unrelated desktop content, retain enough resolution for
zooming, and replace the existing PNG in `src/assets` without renaming it.
Always describe the view and the user action it demonstrates in the image alt
text and caption.

The public documentation intentionally uses the same dark-only theme as the
landing page. Review it with both light and dark operating-system preferences
to confirm that the forced theme remains consistent, then check desktop and
mobile widths plus keyboard focus and reduced-motion behavior.

## MkDocs migration map

END-78 retains every substantive page from the former `docs/` tree. This map
records the destination so later cleanup remains reviewable.

| Former MkDocs source | Starlight destination | Decision |
|---|---|---|
| `docs/index.md` | `src/content/docs/index.md` | Retained as the overview |
| `docs/installation.md` | `src/content/docs/installation.md` | Retained as a how-to guide |
| `docs/configuration.md` | `src/content/docs/configuration.md` | Retained and corrected for current limitations |
| `docs/tui-usage.md` | `src/content/docs/tui-usage.md` | Retained as a how-to guide |
| `docs/headless-usage.md` | `src/content/docs/headless-usage.md` | Retained as the CI/CD guide |
| `docs/scanner-coverage.md` | `src/content/docs/scanner-coverage.md` | Retained as scanner reference |
| `docs/architecture.md` | `src/content/docs/architecture.md` | Retained as a concept page |
| `docs/ownership.md` | `src/content/docs/ownership.md` | Retained as a concept page |
| `docs/versioning.md` | `src/content/docs/versioning.md` | Retained as project reference |
| `docs/assets/*` | `src/assets/*` or `public/assets/*` | Moved to Astro-managed or public assets |
| `mkdocs.yml` | `astro.config.mjs` | Replaced by Starlight configuration |

No substantive MkDocs page was deleted without a destination. New workflow
pages—quickstart, command reference, MCP and agents, troubleshooting, and this
contributing guide—were added directly in Starlight.

## Pull-request checks

Run checks proportional to the change:

```bash
go test ./...
go vet ./...
npm run build
```

Documentation-only changes still need the production documentation build.
Scanner changes require focused tests plus the full Go suite. Releases and tags
remain maintainer-only and run through the test-gated Release workflow.
