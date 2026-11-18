# Changelog

All notable changes to **Senvu** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.1.0] - 2026-11-19

### Initial Release

Senvu is a local-first digital integrity and portability toolkit designed to keep every copy of your data trustworthy.

#### Added
- **Core Engine:**
  - Concurrent directory scanner with a fixed worker pool (`runtime.NumCPU()`).
  - Cryptographic streaming BLAKE3 content hashing with TOCTOU detection.
  - Safe, verifiable file transfer engine with in-stream hashing and atomic file placement.
  - Conservative rename detection based on unique content hash matching.
  - Two-way reconciliation engine with conflict detection and automated non-destructive archiving.
- **Portability & Manifests:**
  - Portable, versioned JSON manifest schema (`pkg/manifest`).
  - Strict manifest validation: path traversal rejection, absolute path rejection, null-byte rejection, and duplicate entry detection.
  - Atomic serialization for manifests and reports via temporary staging files.
- **Storage Layer:**
  - Pure-Go SQLite persistence via `modernc.org/sqlite` (CGO-free).
  - Write-Ahead Logging (`WAL`), 10-second busy timeout, and automatic schema migration.
  - Complete separation between workspace storage and read-only evaluation.
- **CLI Commands:**
  - `senvu init`: Workspace and database initialization.
  - `senvu snapshot create | list | show`: Verified directory snapshots and manifest export.
  - `senvu compare`: Content-aware diff between directory trees or snapshots.
  - `senvu verify`: Manifest and snapshot integrity verification.
  - `senvu transfer`: Safe one-way directory transfers with conflict preservation.
  - `senvu reconcile`: Safe two-way directory synchronization with dry-run support.
  - `senvu report`: Human-readable and JSON status reports.
- **Safety & Platform Hardening:**
  - Cross-platform support for Linux, macOS (Intel & ARM64), and Windows (including UNC and extended path handling).
  - Rejection of path traversal (`../`) and non-dereferencing symlink recording.
  - Unique conflict archive generation (`.conflict.<timestamp>.<nonce>`) preventing backup collisions.
  - Strict verification exit codes: `0` (pass), `1` (divergence/unexpected files), `2` (system/scan error).
