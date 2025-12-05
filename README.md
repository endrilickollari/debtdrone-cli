# DebtDrone CLI

**Stop counting lines. Start fixing debt.**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)](https://go.dev/)
[![GitHub Release](https://img.shields.io/github/v/release/endrilickollari/debtdrone-cli?label=Latest)](https://github.com/endrilickollari/debtdrone-cli/releases)

<!--> ![Demo GIF](https://via.placeholder.com/800x400.png?text=Demo+Coming+Soon)
>
> *Scanning a complex repo in <200ms*-->

---

## Why DebtDrone?

Traditional linters check **style**. DebtDrone analyzes **architecture**.

Most code quality tools rely on regex pattern matching—they're fast but fragile. A complex function signature with nested generics? False positive. A callback wrapped in middleware? Missed entirely.

**DebtDrone uses Abstract Syntax Trees (AST)** via Tree-sitter, the same technology that powers GitHub's code navigation. It understands your code the way a compiler does: parsing structure, not strings. This means:

- ✅ **Zero false positives** on complex type signatures
- ✅ **Context-aware analysis** of function complexity, not just line counts
- ✅ **Multi-language support** with the same level of accuracy

If your linter is guessing, you're not measuring debt—you're measuring noise.

---

## 🎯 Key Features

- **🌳 True AST Analysis**  
  Deep parsing for Go, Python, JavaScript/TypeScript, Java, and Rust. Not regex. Not heuristics.

- **🔒 Security Built-In**  
  Detects hardcoded secrets, API keys, and CVEs via integrated Trivy scanning.

- **🏠 Privacy First**  
  Runs 100% locally. In-memory processing. Zero database. Your source code never leaves your machine unless you explicitly enable cloud features.

- **⚙️ CI/CD Ready**  
  Returns exit code `1` on critical issues. Break builds on high debt. Enforce standards automatically.

- **⚡ Blazing Fast**  
  Written in Go. Scans large repositories in milliseconds, not minutes.

---

## 📦 Installation

### One-Line Install (Recommended)

```bash
curl -sL https://raw.githubusercontent.com/endrilickollari/debtdrone-cli/main/install.sh | bash
```

### Homebrew

```bash
brew tap endrilickollari/debtdrone
brew install debtdrone
```

### Go Install

```bash
go install github.com/endrilickollari/debtdrone/backend/cmd/cli@latest
```

### Docker

```bash
docker run -v $(pwd):/app debtdrone/cli scan .
```

---

## 🚀 Usage

### Basic Scan

```bash
debtdrone scan .
```

Analyzes the current directory and outputs a summary of technical debt and security issues.

### CI/CD Pipeline

```bash
debtdrone scan . --fail-on critical
```

Exits with code `1` if any **critical** issues are found. Perfect for GitHub Actions, GitLab CI, or Jenkins.

### JSON Output for Reporting

```bash
debtdrone scan . --output json > report.json
```

Generates machine-readable output for dashboards, SLAs, or integration with other tools.

---

## 🤖 AI-Powered Fixes (Cloud) — Coming Soon

**DebtDrone CLI finds the issues. DebtDrone Cloud fixes them.**

We're building a dashboard version that will enable AI-powered refactoring and team collaboration features. Stay tuned for the launch.

---

## 📄 License

DebtDrone CLI is distributed under the **MIT License**. Free to use, modify, and distribute.

See [LICENSE](https://github.com/endrilickollari/debtdrone-cli/blob/main/LICENSE) for full details.

---

## 🤝 Contributing

This repository serves as the **public distribution channel** for DebtDrone CLI. The source code is proprietary, but we welcome:

- 🐛 Bug reports
- 💡 Feature requests
- 📖 Documentation improvements

Open an issue or discussion to get started.

---

## 🔗 Links

**Coming Soon** – Website, documentation, and community channels will be available at launch.

---

**Built with ❤️ by developers who are tired of false positives.**
