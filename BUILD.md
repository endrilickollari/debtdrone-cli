# Build & Release Guide

This document explains how to build, test, and release DebtDrone CLI.

## Prerequisites

- **Go 1.21+** - For local development
- **Docker** - For cross-platform builds (CGO support)
- **Make** - For running build commands

## Architecture

DebtDrone CLI uses **CGO** for Tree-sitter integration, which requires:
- Native C/C++ compilation
- Cross-compilation toolchains for different platforms
- Docker-based builds for consistency

## Quick Start

### Local Development Build

```bash
# Build for your current platform
make build

# Run tests
make test

# Clean artifacts
make clean
```

The binary will be created in `dist/debtdrone`.

### Test Cross-Platform Build

```bash
# Create snapshot release (all platforms)
make snapshot

# Check artifacts
ls -la dist/
```

This uses Docker to simulate the full CI/CD pipeline locally.

## Build System Components

### 1. GoReleaser (`.goreleaser.yaml`)

Handles cross-platform compilation and release creation.

**Key Configuration:**
- **Entry Point**: `./cmd/debtdrone`
- **Binary Name**: `debtdrone`
- **CGO**: Enabled with platform-specific compilers
- **Platforms**: Linux/Darwin × AMD64/ARM64
- **Archive Format**: `debtdrone_Darwin_arm64.tar.gz`

**Cross-Compilers:**
- macOS AMD64: `o64-clang`
- macOS ARM64: `oa64-clang`
- Linux AMD64: `x86_64-linux-gnu-gcc`
- Linux ARM64: `aarch64-linux-gnu-gcc`

### 2. Makefile

Provides convenient build commands:

| Command | Description |
|---------|-------------|
| `make build` | Build locally (current platform only) |
| `make test` | Run test suite |
| `make clean` | Remove build artifacts |
| `make snapshot` | Full cross-platform build (no release) |
| `make install` | Install binary to `/usr/local/bin` |
| `make uninstall` | Remove installed binary |

### 3. GitHub Actions (`.github/workflows/release.yml`)

Automates releases when you push a version tag.

**Trigger**: `git push origin v0.1.0`

**Process:**
1. Checkout code with full history
2. Login to GitHub Container Registry
3. Run GoReleaser in Docker
4. Build binaries for all platforms
5. Create GitHub Release
6. Upload artifacts

## Release Process

### Step 1: Test Locally

```bash
# Ensure code is clean
git status

# Run tests
make test

# Test cross-compilation
make snapshot

# Verify artifacts
ls dist/
```

### Step 2: Create Version Tag

```bash
# Determine version (follow semantic versioning)
VERSION=v0.2.0

# Create annotated tag
git tag -a $VERSION -m "Release $VERSION"

# Push tag to GitHub
git push origin $VERSION
```

### Step 3: Monitor GitHub Actions

1. Go to: https://github.com/endrilickollari/debtdrone-cli/actions
2. Watch the "Release" workflow
3. Wait for completion (~5-10 minutes)

### Step 4: Verify Release

1. Go to: https://github.com/endrilickollari/debtdrone-cli/releases
2. Check that all platform binaries are present:
   - `debtdrone_Linux_x86_64.tar.gz`
   - `debtdrone_Linux_arm64.tar.gz`
   - `debtdrone_Darwin_x86_64.tar.gz`
   - `debtdrone_Darwin_arm64.tar.gz`
   - `checksums.txt`

### Step 5: Test Installation

```bash
# Test the install script
curl -sL https://raw.githubusercontent.com/endrilickollari/debtdrone-cli/main/install.sh | bash

# Verify installation
debtdrone --version
```

## Archive Naming Convention

**Critical**: The installation script expects this exact format:

```
{ProjectName}_{OS}_{Arch}.tar.gz
```

Examples:
- `debtdrone_Linux_x86_64.tar.gz`
- `debtdrone_Darwin_arm64.tar.gz`

The `.goreleaser.yaml` configuration ensures this format is maintained.

## Troubleshooting

### Build Fails Locally

```bash
# Check Go version
go version  # Should be 1.21+

# Update dependencies
go mod tidy
go mod download

# Try building manually
go build -o dist/debtdrone ./cmd/debtdrone
```

### Docker Build Fails

```bash
# Check Docker is running
docker ps

# Pull the build image manually
docker pull ghcr.io/goreleaser/goreleaser-cross:v1.23.2

# Check disk space
df -h
```

### GitHub Action Fails

**Common Issues:**

1. **Missing GITHUB_TOKEN**: This is automatic, but check workflow permissions
2. **CGO Errors**: Ensure `.goreleaser.yaml` has correct compiler paths
3. **Tag Format**: Must match `v*` pattern (e.g., `v0.1.0`)

**Debug:**
```bash
# Test GoReleaser config locally
docker run --rm -v $PWD:/code -w /code \
  ghcr.io/goreleaser/goreleaser-cross:v1.23.2 \
  check
```

### Archive Name Mismatch

If `install.sh` can't find binaries:

1. Check GitHub Release assets
2. Verify naming matches pattern
3. Update `.goreleaser.yaml` if needed
4. Re-run release

## CGO and Tree-sitter

Tree-sitter requires native compilation, which is why we use CGO.

**Implications:**
- Build times are longer (~5-10 min)
- Must use Docker for cross-compilation
- Cannot use `go install` directly
- Binaries are platform-specific

**Benefits:**
- Zero false positives (true AST parsing)
- Multi-language support
- Fast runtime performance

## Version Management

We follow [Semantic Versioning](https://semver.org/):

- `v0.1.0` - Initial release
- `v0.2.0` - New features (minor)
- `v0.2.1` - Bug fixes (patch)
- `v1.0.0` - Stable API (major)

## Pre-release Testing

Before tagging a release:

```bash
# 1. Run full test suite
make test

# 2. Build snapshot
make snapshot

# 3. Test each platform binary
./dist/debtdrone_linux_amd64/debtdrone --version
./dist/debtdrone_darwin_arm64/debtdrone --version

# 4. Verify install script logic
./install.sh
```

## Rollback

If a release has issues:

```bash
# Delete the tag locally
git tag -d v0.2.0

# Delete the tag on GitHub
git push origin :refs/tags/v0.2.0

# Delete the GitHub Release manually
# (Go to Releases page and delete)

# Fix the issue, then re-release
git tag -a v0.2.0 -m "Release v0.2.0 (fixed)"
git push origin v0.2.0
```

## Support

- **Build Issues**: Open issue on GitHub
- **Private Repo Sync**: Internal documentation only

---

**Last Updated**: 2025-12-05
