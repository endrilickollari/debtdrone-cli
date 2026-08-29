# Build and release guide

This guide covers local development, release validation, and the maintainer-only
release workflow for DebtDrone CLI v2.

## Prerequisites

- Go 1.25.1 or later;
- a C/C++ compiler for Tree-sitter's CGO dependencies;
- Make for the repository shortcuts;
- Docker only for cross-platform snapshot builds; and
- Node.js 22.12 or later for the documentation site.

## Local development

Build the CLI for the current platform:

```bash
make build
./dist/debtdrone --version
```

Run the complete Go test suite and static checks:

```bash
make test
go vet ./...
```

`make test` runs `go test ./...`, including command, scanner API, and internal
package tests. Use `make clean` to remove generated build and documentation
artifacts.

DebtDrone uses CGO for Tree-sitter parsing. A local `go build` or `go install`
therefore needs a working native compiler, even though the installed CLI is a
single executable.

## Documentation

Install the locked dependencies and build the production documentation site:

```bash
npm ci
npm run build
```

Use `npm run dev` for a local development server. The generated production site
is written to `docs-dist/`.

## Cross-platform release validation

GoReleaser builds Linux and macOS archives for AMD64 and ARM64, plus a Windows
AMD64 archive. Because these builds require platform-specific CGO toolchains,
the repository runs GoReleaser through its pinned cross-compilation container:

```bash
make snapshot
ls -la dist/
```

This creates local snapshot artifacts only. It does not create a tag, publish a
GitHub release, update Homebrew, or notify the SaaS repository.

The archive names come from `.goreleaser.yaml` and must remain compatible with
the installation scripts:

```text
debtdrone_Darwin_x86_64.tar.gz
debtdrone_Darwin_arm64.tar.gz
debtdrone_Linux_x86_64.tar.gz
debtdrone_Linux_arm64.tar.gz
debtdrone_Windows_x86_64.zip
checksums.txt
```

## Maintainer release process

Releases are created only by the manually dispatched **Release** GitHub Actions
workflow. Do not create, move, delete, or push release tags manually.

Before dispatching a release:

1. Confirm the intended changes are merged into `main` and required CI checks
   are green.
2. Choose a valid v2 semantic version, such as `v2.1.0` or `v2.1.0-rc.1`, using
   the [versioning policy](src/content/docs/versioning.md).
3. Run the **Release** workflow from `main` with that version.
4. Monitor validation, tag creation, publication, and the SaaS dependency-update
   dispatch.

The workflow validates the version, runs `go test ./...`, `go vet ./...`, a CLI
build, and a GoReleaser snapshot before it creates an annotated tag. It then
publishes GitHub and Homebrew artifacts from that immutable tag and requests a
versioned scanner update in the SaaS repository.

After publication, verify the release assets and test the public installer:

```bash
curl -sL https://raw.githubusercontent.com/endrilickollari/debtdrone-cli/main/installation_scripts/install.sh | bash
debtdrone --version
```

The reported version must match the released tag. Windows installation can be
verified with `installation_scripts/install.ps1` in PowerShell.

## Failed releases and rollback

- If validation fails before tag creation, fix the cause on a feature branch,
  merge it through the normal pull-request process, and dispatch a new release.
- If tag creation succeeded but publication failed, dispatch the same version
  with `retry_failed_publication` enabled. The workflow verifies and reuses the
  existing immutable tag.
- If publication succeeded but the SaaS notification failed, keep the release
  tag unchanged and run the SaaS **Scanner Dependency Update** workflow with the
  published version.
- If a published scanner has a defect, keep its tag immutable. Patch the issue
  and publish a new patch version. The SaaS remains pinned until its scanner
  dependency pull request passes and is merged, so it can stay on or return to
  the last known-good version independently.

See [Versioning and releases](src/content/docs/versioning.md) for version rules,
required secrets, and retry semantics.

## Troubleshooting

### Local CGO build fails

Confirm the Go version, compiler, and CGO setting:

```bash
go version
go env CGO_ENABLED CC CXX
go build ./cmd/debtdrone
```

Install Xcode Command Line Tools on macOS or the platform C/C++ build tools on
Linux before retrying.

### Snapshot build fails

Confirm Docker is running and that the pinned image in `Makefile` is available.
The GitHub workflow is authoritative for release validation; do not change tags
or bypass its checks to recover a local snapshot failure.

### Installer cannot find an artifact

Compare the GitHub release asset names with `.goreleaser.yaml` and the platform
mapping in `installation_scripts/install.sh` or `install.ps1`. Correct the build
or installer in a new release; never replace a successfully published tag.
