# Installation

DebtDrone ships as a single static binary with no runtime dependencies. Choose the method that best fits your workflow.

---

## Option 1 — `go install`

If you have Go 1.21 or later on your `PATH`, this is the fastest path to a working installation:

```bash
go install github.com/endrilickollari/debtdrone-cli/cmd/debtdrone@latest
```

The binary is placed in `$(go env GOPATH)/bin`. Ensure that directory is on your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Verify the installation:

```bash
debtdrone --version
```

---

## Option 2 — Pre-compiled Binaries

Pre-compiled static binaries are published to the [GitHub Releases](https://github.com/endrilickollari/debtdrone-cli/releases/latest) page for every tagged release. Download the archive for your platform, extract it, and place the binary somewhere on your `PATH`.

### Supported Platforms

| OS | Architecture | Archive |
|---|---|---|
| macOS | x86_64 (Intel) | `debtdrone_Darwin_x86_64.tar.gz` |
| macOS | arm64 (Apple Silicon) | `debtdrone_Darwin_arm64.tar.gz` |
| Linux | x86_64 | `debtdrone_Linux_x86_64.tar.gz` |
| Linux | arm64 | `debtdrone_Linux_arm64.tar.gz` |
| Windows | x86_64 | `debtdrone_Windows_x86_64.zip` |

!!! note "Windows ARM64"
    Windows ARM64 is not currently supported in the pre-compiled release artifacts. Windows ARM64 users should build from source (see below).

### macOS / Linux

```bash
# Replace <version> and <platform> with the values for your system
# Example: debtdrone_Darwin_arm64.tar.gz for Apple Silicon
curl -L https://github.com/endrilickollari/debtdrone-cli/releases/latest/download/debtdrone_Darwin_arm64.tar.gz \
  | tar -xz -C /usr/local/bin

debtdrone --version
```

### Windows (PowerShell)

```powershell
# Download and extract
Invoke-WebRequest -Uri "https://github.com/endrilickollari/debtdrone-cli/releases/latest/download/debtdrone_Windows_x86_64.zip" `
  -OutFile debtdrone.zip
Expand-Archive -Path debtdrone.zip -DestinationPath "$env:LOCALAPPDATA\debtdrone"

# Add to PATH for the current session
$env:PATH += ";$env:LOCALAPPDATA\debtdrone"
```

---

## Option 3 — Homebrew (macOS & Linux)

DebtDrone is published to a [Homebrew tap](https://github.com/endrilickollari/homebrew-tap):

```bash
brew tap endrilickollari/tap
brew install debtdrone
```

Upgrade to the latest release at any time:

```bash
brew upgrade debtdrone
```

---

## Option 4 — Build from Source

Clone the repository and build with the standard Go toolchain:

```bash
git clone https://github.com/endrilickollari/debtdrone-cli.git
cd debtdrone-cli
go build -o debtdrone ./cmd/debtdrone
sudo mv debtdrone /usr/local/bin/
```

!!! note "CGO requirement"
    The analysis engine uses [tree-sitter](https://tree-sitter.github.io/tree-sitter/) for multi-language syntax parsing, which requires CGO. Ensure a C compiler (`gcc` or `clang`) is present on your system before building from source.

---

## Built-in Auto-Updater

!!! tip "Install once, stay current automatically"
    DebtDrone ships with a built-in self-updater. Once installed by any of the methods above, you never need to manually upgrade again. Use the `/update` command in the TUI, or run `debtdrone update` from the command line to check for a new release, view the changelog, and apply the update in-place — no package manager required.

---

## Verifying the Installation

Run the following to confirm everything is working:

```bash
# Show version information
debtdrone --version

# Run a quick scan of the current directory
debtdrone scan . --format=text
```

If you see version output and a scan report, you are ready to go.
