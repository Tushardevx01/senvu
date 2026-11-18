# Contributing to Senvu

Thank you for your interest in contributing to Senvu! Every contribution — whether it's a bug report, a feature suggestion, documentation improvement, or code — helps make the project better.

## Getting Started

### Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- Git
- Make (optional, but recommended)

### Building

```bash
# Clone the repository
git clone https://github.com/senvu/senvu.git
cd senvu

# Build using Make
make build

# Or build directly with Go
go build -o bin/senvu ./cmd/senvu
```

### Running Tests

```bash
# Run all tests with race detection
make test

# Or directly
go test -race ./...
```

### Code Style

Senvu follows standard Go conventions:

- Run `gofmt -w .` before committing (or `make fmt`)
- Run `go vet ./...` to catch common issues (or `make lint`)
- All exported types and functions must have doc comments
- Use `fmt.Errorf("context: %w", err)` for error wrapping
- Keep functions focused and testable

## Pull Request Process

1. **Fork the repository** and create a feature branch from `main`
2. **Write your code** following the style guidelines above
3. **Add or update tests** for any new or changed functionality
4. **Run the full test suite** (`make test`) and ensure it passes
5. **Run the linter** (`make lint`) and fix any issues
6. **Write a clear commit message** following [Conventional Commits](https://www.conventionalcommits.org/):
   - `feat: add snapshot export command`
   - `fix: handle symlink loops in scanner`
   - `docs: update architecture diagram`
7. **Open a pull request** with a clear description of what you changed and why

### What Makes a Good PR

- **Focused**: one logical change per PR
- **Tested**: new features have tests, bug fixes include regression tests
- **Documented**: public APIs have doc comments, significant changes update relevant docs
- **Clean history**: squash fixup commits before requesting review

## Reporting Issues

### Bug Reports

When filing a bug report, please include:

- **What you were trying to do** (expected behavior)
- **What happened instead** (actual behavior)
- **Steps to reproduce** the issue
- **Environment details**: OS, Go version, Senvu version (`senvu --version`)
- **Relevant logs** or error messages (use `--verbose` for extra detail)

### Feature Requests

Feature requests are welcome. Please describe:

- **The problem** you're trying to solve
- **Your proposed solution** (if you have one)
- **Alternatives** you've considered

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## Questions?

If you're unsure about anything, open an issue and ask. We'd rather help you contribute than have you stuck.
