# Senvu Development Guide

This guide covers building, testing, and contributing to the Senvu codebase.

## Prerequisites

- **Go**: 1.22 or higher (recommended: Go 1.22+)
- **Make**: Standard GNU Make or compatible build utility
- **Git**

## Quick Start

```bash
# Clone the repository
git clone https://github.com/senvu/senvu.git
cd senvu

# Build the binary
make build

# Run the test suite with race detector
make test

# Run linter / static analysis
make lint

# Install locally
make install
```

The compiled binary will be placed in `bin/senvu`.

## Project Structure

```
senvu/
├── cmd/
│   └── senvu/              # CLI entry point (main.go)
├── internal/
│   ├── app/                # Orchestration layer for CLI commands
│   ├── scanner/            # Directory scanner with bounded concurrency
│   ├── integrity/          # BLAKE3 streaming hasher & Hasher interface
│   ├── snapshot/           # Snapshot creation, listing, export service
│   ├── compare/            # Content-hash comparison & rename detection
│   ├── transfer/           # Safe file transfer engine & conflict handling
│   ├── reconcile/          # Reconciliation engine with dry-run & preservation
│   ├── storage/            # SQLite storage with migrations (modernc.org/sqlite)
│   ├── config/             # Workspace configuration & validation
│   ├── output/             # Structured terminal & JSON output formatter
│   └── platform/           # Filesystem safety, atomic writes, symlinks
├── pkg/
│   └── manifest/           # Portable, versioned JSON manifest schema
├── docs/                   # Architecture and development documentation
├── tests/                  # Integration tests executing end-to-end scenarios
├── .github/                # CI and release workflows, issue templates
├── Makefile                # Build targets
├── go.mod
└── go.sum
```

## Running Tests

Senvu takes correctness and concurrency safety seriously. All tests should be run with the Go race detector enabled:

```bash
go test -v -race ./...
```

To run a specific package test:

```bash
go test -v -race ./internal/transfer/...
```

To run end-to-end integration tests:

```bash
go test -v -race ./tests/...
```

## Code Guidelines

- **Idiomatic Go**: Follow standard Go naming conventions and formatting (`gofmt -w .`).
- **Error Wrapping**: Wrap errors with context using `fmt.Errorf("context: %w", err)`.
- **Bounded Concurrency**: Never spawn unbounded goroutines for filesystem or hashing operations.
- **Safety First**: Destructive actions must require explicit flags (`--force`, `--yes`, `--overwrite`) and dry-run previews must be available.
- **No Network In Core**: Core engine components must remain fully offline.
