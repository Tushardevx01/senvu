# Senvu Architecture

This document describes the architectural design, core abstractions, and security boundaries of Senvu.

## Overview

Senvu is structured as a **modular monolithic CLI tool** in Go. It enforces strict separation of concerns across layers while avoiding unnecessary distributed microservices or bloated dependencies.

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

## Architectural Layers

### 1. CLI Layer (`cmd/senvu`)
- Built using **Cobra**.
- Responsible for parsing command line arguments, flags, environment variables, and exit codes.
- Global flags: `--json`, `--quiet`, `--verbose`, `--config`.
- Never pollutes terminal with unwanted banners or ASCII art.
- Exit codes:
  - `0`: Success / Verification Passed / Copies Identical
  - `1`: Differences detected / Verification Failed / Conflict Encountered
  - `2`: Operational or system error

### 2. Application Layer (`internal/app`)
- Use-case orchestrators for all CLI operations (`Init`, `CreateSnapshot`, `ListSnapshots`, `ShowSnapshot`, `CompareDirectories`, `Verify`, `Transfer`, `Reconcile`, `GenerateReport`).
- Coordinates scanner, hasher, comparator, transfer, reconciliation, and storage engines.
- Formats structured outputs for human-readable display and automation.

### 3. Core Engine

#### Scanner (`internal/scanner`)
- Recursively walks a directory tree using `filepath.WalkDir`.
- Skips `.senvu/` workspace metadata.
- Implements bounded concurrency hashing using worker pools sized to `runtime.NumCPU()`.
- Captures per-file errors without aborting the entire scan.
- Detects symlinks without traversing outside boundaries.
- Sorts results deterministically by relative path.

#### Integrity & Hasher (`internal/integrity`)
- Interface `Hasher`:
  ```go
  type Hasher interface {
      Hash(reader io.Reader) (string, error)
      Algorithm() string
  }
  ```
- Default implementation: **BLAKE3** via `github.com/zeebo/blake3`.
- Streams data in 32 KB / 64 KB buffers, never loading entire files into memory.

#### Compare Engine (`internal/compare`)
- Performs content-based comparison between file sets.
- Detects:
  - Added files
  - Removed files
  - Modified files (content hash divergence)
  - Unchanged files (identical content hashes)
  - Renamed files (unambiguous 1:1 hash match across removals and additions)

#### Transfer Engine (`internal/transfer`)
- Provides safe, verified file transfers.
- Preserves file timestamps (`os.Chtimes`) and file modes (`os.Chmod`).
- Implements atomic staging: writes to `.senvu-transfer-*` in the target directory, verifies written content hash against source hash, and atomically renames.
- Fallback for cross-device moves (`EXDEV`).
- Conflict detection: by default (`ConflictPolicyAbort`), refuses to overwrite divergent files.

#### Reconciliation Engine (`internal/reconcile`)
- Generates transparent, deterministic execution plans.
- Always supports `--dry-run` to preview actions before touching disk.
- Never silently deletes user data: destination-only files are marked `RETAIN_DEST`.
- Preserves conflicting files: divergent destination files are safely renamed with `.conflict.<timestamp>` before applying source copies.

### 4. Storage Layer (`internal/storage`, `pkg/manifest`)
- **SQLite Store (`internal/storage`)**:
  - Implemented using `modernc.org/sqlite` (pure Go, CGO-free, cross-platform).
  - WAL journal mode for concurrent read access.
  - Foreign key constraints enabled.
  - Versioned schema migrations table.
- **Portable Manifest (`pkg/manifest`)**:
  - Standalone JSON document format.
  - Path traversal and absolute path validation.
  - Machine-independent relative paths.

### 5. Platform Layer (`internal/platform`)
- Path canonicalization and traversal checks (`SafePath`).
- Atomic file writes (`AtomicWrite`).
- Symlink detection (`IsSymlink`).

## Security Boundaries & Threat Model

1. **Untrusted Manifests**:
   - Portable manifests loaded from external sources are treated as untrusted.
   - All paths are validated against `..` traversals, absolute prefixes, and drive-letter escapes.
2. **Local Isolation**:
   - Zero network connectivity. Senvu contains no telemetry, remote reporting, or external API calls.
3. **Data Loss Prevention**:
   - Non-destructive by default.
   - Atomic rename and temporary file cleaning prevent incomplete or corrupted file writes.
