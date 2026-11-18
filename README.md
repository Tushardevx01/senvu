# Senvu

**Keep every copy trustworthy.**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://github.com/senvu/senvu/actions/workflows/ci.yml/badge.svg)](https://github.com/senvu/senvu/actions/workflows/ci.yml)

---

## What is Senvu?

Senvu is a **local-first digital integrity and portability toolkit**. It scans storage directories, computes cryptographic BLAKE3 content hashes, produces portable versioned manifests, safely transfers files, and reliably reconciles differences across devices, external drives, and backup systems — ensuring every copy remains trustworthy and verifiable.

Senvu operates **fully offline**. There are no accounts, no subscriptions, no API keys, no hosted servers, and zero telemetry. Everything runs locally on your machine, against your files, under your complete control.

It is free and open-source software, released under the permissive [MIT License](LICENSE).

---

## Brand Identity & Visual Philosophy

- **Tagline:** Keep every copy trustworthy.
- **Brand Personality:** Minimal, not boring. Technical, not intimidating. Trustworthy, not corporate.
- **Palette:**
  - Deep Ink (`#172D35`): Foundational clarity and depth
  - Mint (`#83DCC3`): Verification success and data health
  - Mist (`#E8F0ED`): Neutral, quiet backgrounds
  - Steel (`#647985`): Structural indicators and metadata

---

## The Problem

People maintain copies of essential data across laptops, external hard drives, USB flash sticks, NAS arrays, and cloud storage folders.

Inevitably, copies drift and uncertainty accumulates:
- Did a file change intentionally, or is a drive suffering silent bitrot?
- Which backup location holds the true latest version?
- Was a copy operation interrupted mid-transfer?
- Will syncing or copying overwrite newer work or conflicting edits?
- Can two locations safely be reconciled without losing data?

Most utilities assume a single centralized authority or hide changes behind opaque sync heuristics. Senvu never guesses: it computes cryptographic content hashes, compares states honestly, previews actions before modifying disk, and keeps you in control.

---

## Non-Negotiable Principles

1. **Never silently delete user data** — Senvu will never remove files without explicit confirmation.
2. **Never silently overwrite conflicting files** — Divergent destination copies are surfaced and safely preserved.
3. **Never execute destructive operations without approval** — Dry-run previews are standard.
4. **Detect conflicts rather than guessing** — Timestamp-alone equality is rejected; content hashes determine truth.
5. **Reproducible integrity verification** — Powered by streaming BLAKE3 digests.
6. **Work fully offline** — Standalone binary with zero network dependencies.
7. **Portable manifests** — Versioned JSON format independent of local database internals.
8. **Safe handling of interrupted operations** — Atomic writes and staged temp files prevent partial corruption.
9. **Report failures honestly** — Never report a partially completed operation as successful.

---

## Installation

### Build from Source

```bash
git clone https://github.com/senvu/senvu.git
cd senvu
make build
# Binary is produced at ./bin/senvu
```

### Go Toolchain

```bash
go install github.com/senvu/senvu/cmd/senvu@latest
```

### Cross-Platform Binaries

Standalone static binaries for Linux, macOS (Apple Silicon & Intel), and Windows are published under [Releases](https://github.com/senvu/senvu/releases).

---

## Quick Start Workflow

```bash
# 1. Initialize a Senvu workspace
senvu init ./my-data

# 2. Create a snapshot and export a portable manifest
senvu snapshot create ./my-data --export baseline.json

# 3. List stored snapshots
senvu snapshot list ./my-data

# 4. Compare two directories using content hashes
senvu compare ./my-data ./backup

# 5. Verify integrity against a manifest or snapshot
senvu verify ./backup --manifest baseline.json

# 6. Safely transfer files with conflict detection
senvu transfer ./my-data ./backup

# 7. Preview and execute safe reconciliation
senvu reconcile ./my-data ./backup --dry-run
senvu reconcile ./my-data ./backup --yes

# 8. Export structured integrity reports
senvu report export --format json -o report.json
```

---

## Command Reference

| Command | Usage | Description |
|---|---|---|
| `init` | `senvu init [dir] [--force]` | Initialize a new Senvu workspace and local SQLite database |
| `snapshot create` | `senvu snapshot create [dir] [--export file.json]` | Recursively scan, hash (BLAKE3), and create an immutable snapshot |
| `snapshot list` | `senvu snapshot list [dir]` | List all snapshots recorded in the workspace index |
| `snapshot show` | `senvu snapshot show <id> [--workspace dir]` | Inspect detailed file entries and metadata for a snapshot |
| `compare` | `senvu compare <source> <dest>` | Compare two directories; detects added, removed, modified, unchanged, renamed |
| `verify` | `senvu verify [dir] [--manifest file.json]` | Verify files against a trusted manifest or snapshot (Exit: 0 pass, 1 diff, 2 err) |
| `transfer` | `senvu transfer <source> <dest> [--dry-run]` | Safely transfer files with atomic writes, metadata preservation, and conflict checks |
| `reconcile` | `senvu reconcile <source> <dest> [--dry-run]` | Generate a safe reconciliation plan; preserves conflicts as `.conflict.<timestamp>` |
| `report` | `senvu report [snapshot-id]` | Display workspace or snapshot status report |
| `report export` | `senvu report export [--format json\|text]` | Export structured report to standard output or file |

### Global Flags

- `--json`: Format command results as structured JSON (ideal for automation).
- `--verbose`, `-v`: Enable detailed diagnostic logging.
- `--quiet`, `-q`: Suppress informational messages, printing errors only.
- `--config`: Specify path to an alternate configuration file.

---

## Architecture

Senvu is designed as a cohesive modular engine:

```
┌────────────────────────────────────────────────────────┐
│                      CLI Layer                         │
│                    (cmd/senvu)                         │
├────────────────────────────────────────────────────────┤
│                  Application Layer                     │
│                   (internal/app)                       │
├────────────────────────────────────────────────────────┤
│                     Core Engine                        │
│  ┌──────────┬──────────┬──────────┬──────────┬───────┐ │
│  │ Scanner  │ Integrity│ Compare  │ Transfer │Reconc.│ │
│  └──────────┴──────────┴──────────┴──────────┴───────┘ │
├────────────────────────────────────────────────────────┤
│                    Storage Layer                       │
│           (internal/storage, pkg/manifest)             │
├────────────────────────────────────────────────────────┤
│                   Platform Layer                       │
│                 (internal/platform)                    │
└────────────────────────────────────────────────────────┘
```

For detailed component documentation, see [docs/architecture.md](docs/architecture.md).

---

## Safety Guarantees & Known Limitations

### Safety Guarantees
- **True Content Equality**: Files are verified using cryptographic BLAKE3 hashes, preventing false positives from identical timestamps or sizes alone.
- **Unambiguous Rename Detection**: Rename tracking only links files when exactly one removed file and one added file share the identical hash.
- **Staged File Operations**: File writes use staged temporary files in the target directory with sync before rename, avoiding in-place truncation during writes on supported local filesystems.
- **Path Traversal Protection**: All paths in manifests and commands are checked against `..` escapes, null bytes, and symlink redirects.
- **Data Preservation**: Reconciliation never deletes destination-only files and backs up conflicted copies before applying updates.

### Known Limitations
- **Not a Sync Daemon**: Senvu is an on-demand integrity and portability toolkit, not a background continuous synchronization daemon.
- **Filesystem Semantics**: Rename atomicity relies on local filesystem semantics (standard POSIX); network mounts or non-POSIX filesystems may not guarantee atomic replacement.
- **No Encryption Yet**: Manifests and SQLite databases are currently stored in unencrypted plaintext on disk (encryption is planned for Phase 5).
- **No Digital Signatures Yet**: Manifest digital signing (PKI / Ed25519) is planned for a future release.

---

## Roadmap

| Phase | Milestone | Status |
|---|---|---|
| **Phase 1** | Foundation: Go module, CLI framework, configuration, logging, platform safety | ✅ Complete |
| **Phase 2** | Integrity Engine: Concurrent scanner, BLAKE3 streaming hasher, manifests, SQLite store | ✅ Complete |
| **Phase 3** | Comparison Engine: Differences detection, rename tracking, verification, JSON reports | ✅ Complete |
| **Phase 4** | Safe Operations: Verified transfer, conflict preservation, dry-run reconciliation | ✅ Complete |
| **Phase 5** | Future Enhancements: Ed25519 manifest signing, optional client-side encryption | 🔲 Planned |

---

## Contributing

We welcome contributions from developers, researchers, and open-source advocates. Please see [CONTRIBUTING.md](CONTRIBUTING.md) and our [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for contribution guidelines.

---

## License

Senvu is open source software released under the [MIT License](LICENSE).  
Copyright © 2024–2026 Senvu Contributors.
