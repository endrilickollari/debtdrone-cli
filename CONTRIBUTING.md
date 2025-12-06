# Contributing to DebtDrone CLI

Thank you for your interest in contributing to DebtDrone CLI! This guide will help you get started.

## 🚀 Quick Start

### Prerequisites

- **Go 1.21+** - [Download](https://go.dev/dl/)
- **Docker** - [Install](https://docs.docker.com/get-docker/)
- **Make** - Usually pre-installed on macOS/Linux

### Setup

```bash
# Clone the repository
git clone https://github.com/endrilickollari/debtdrone-cli.git
cd debtdrone-cli

# Download dependencies
go mod download

# Build the CLI
make build

# Run tests
make test
```

## 📋 Development Workflow

### Building

```bash
# Build for your platform
make build
./dist/debtdrone --help

# Cross-platform build (all platforms)
make snapshot
```

### Testing

```bash
# Run all tests
make test

# Run specific package tests
go test ./internal/analyzer/...

# Run with coverage
go test -cover ./internal/...
```

### Cleaning

```bash
# Remove build artifacts
make clean
```

## 🐛 Reporting Issues

We welcome bug reports and feature requests!

### Before Opening an Issue

1. **Search existing issues** to avoid duplicates
2. **Check the documentation** in README.md and BUILD.md
3. **Update to the latest version** to see if it's already fixed

### Issue Template

```markdown
**Description**: Clear description of the issue

**Steps to Reproduce**:
1. Run `debtdrone scan .`
2. See error...

**Expected Behavior**: What should happen

**Actual Behavior**: What actually happens

**Environment**:
- OS: macOS 14.0 / Ubuntu 22.04 / etc.
- Architecture: ARM64 / AMD64
- DebtDrone Version: `debtdrone --version`

**Additional Context**: Any relevant logs or screenshots
```

## 💡 Feature Requests

Have an idea? We'd love to hear it!

1. Open a new issue with the `enhancement` label
2. Describe the problem you're trying to solve
3. Explain your proposed solution
4. Include examples if possible

## 📝 Code Contributions

**Note**: The source code is maintained in a private repository. This public repository is for:
- Bug reports
- Feature requests
- Documentation improvements
- Installation script fixes

### What You Can Contribute

✅ **Documentation**: README, BUILD.md, this file
✅ **Installation Script**: `install.sh` improvements
✅ **Build Configuration**: `.goreleaser.yaml`, Makefile, GitHub Actions
✅ **Examples**: Usage examples, CI/CD integrations

❌ **Source Code**: CLI implementation is closed-source

### Pull Request Process

1. **Fork the repository**
2. **Create a feature branch**: `git checkout -b fix/install-script`
3. **Make your changes**
4. **Test thoroughly**
5. **Commit with clear messages**: `fix: Handle missing /usr/local/bin directory`
6. **Push and open a PR**

### Commit Message Format

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>: <description>

[optional body]
[optional footer]
```

**Types**:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `ci`: CI/CD changes
- `chore`: Maintenance tasks
- `test`: Test additions/changes

**Examples**:
```
feat: Add Windows support to install script
fix: Correct arm64 detection on Linux
docs: Update installation instructions
ci: Upgrade GoReleaser to v2
```

## 🔨 Build System

### Local Development

```bash
# Standard build
make build

# Install locally
make install

# Uninstall
make uninstall
```

### Testing Cross-Compilation

```bash
# Build for all platforms
make snapshot

# Check artifacts
ls -la dist/
```

### Understanding CGO

DebtDrone uses CGO for Tree-sitter integration:
- Requires C/C++ compilation
- Cross-compilation needs Docker
- Longer build times (~5-10 min)

See [BUILD.md](BUILD.md) for detailed information.

## 📦 Release Process

Only maintainers can create releases.

### Creating a Release

1. **Test everything**
   ```bash
   make test
   make snapshot
   ```

2. **Create and push tag**
   ```bash
   git tag -a v0.2.0 -m "Release v0.2.0"
   git push origin v0.2.0
   ```

3. **Monitor GitHub Actions**
   - Check workflow completion
   - Verify artifacts in Release

4. **Test installation**
   ```bash
   curl -sL https://raw.githubusercontent.com/endrilickollari/debtdrone-cli/main/install.sh | bash
   debtdrone --version
   ```

## 📖 Documentation

### README.md

Marketing-focused documentation:
- Hero section with demo
- Feature highlights
- Installation instructions
- Usage examples

### BUILD.md

Technical build documentation:
- Architecture overview
- Build system components
- Release process
- Troubleshooting

### This File (CONTRIBUTING.md)

Contributor guidelines and quick reference.

## 🤝 Code of Conduct

### Our Standards

- **Be respectful** and inclusive
- **Be constructive** in feedback
- **Be patient** with others
- **Focus on what's best** for the community

### Unacceptable Behavior

- Harassment or discrimination
- Trolling or inflammatory comments
- Personal or political attacks
- Publishing private information

## 🔒 Security

Found a security vulnerability? **Do not open a public issue.**

Email: security@debtdrone.io (Coming Soon)

For now, contact: endrilickollari@gmail.com

## 📞 Support

- **Issues**: https://github.com/endrilickollari/debtdrone-cli/issues
- **Discussions**: https://github.com/endrilickollari/debtdrone-cli/discussions

## ⚖️ License

By contributing, you agree that your contributions will be licensed under the MIT License.

See [LICENSE](LICENSE) for details.

## 🙏 Thank You

Your contributions make DebtDrone better for everyone. We appreciate your time and effort!

---

**Questions?** Open a discussion on GitHub or reach out via issues.
