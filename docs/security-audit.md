# Senvu Security Audit Report

**Audit Date:** 2026-11-19  
**Auditor:** Principal Security Engineer & Open-Source Maintainer  
**Target Repository:** `github.com/senvu/senvu`  
**Status:** Completed (All Findings Remediated & Verified)

---

## 1. Scope & Objectives

This audit evaluated the security architecture, threat model, input validation, filesystem boundaries, and cryptographic verification of the Senvu CLI.

### Threat Model Boundaries
1. **Untrusted Inputs:** Portable JSON manifests from third parties, arbitrary filesystem paths, symbolic link trees, and concurrent storage modifications.
2. **Local Security Invariants:**
   - No path traversal (`../`, absolute paths, UNC drive escapes, null bytes) outside designated directories.
   - Zero symlink escapes during scan, compare, verify, transfer, or reconcile.
   - No unvetted code execution, shell injections, or arbitrary writes.
   - Zero external network exposure (offline operation guaranteed).

---

## 2. Findings & Resolutions

### Finding SEC-01: Read-Only Commands Mutated User Storage by Opening Non-Existent SQLite Databases
- **Severity:** Medium
- **Component:** `internal/storage/storage.go`, `internal/app/snapshot.go`
- **Description:** Running `senvu snapshot list <dir>` or `senvu report <dir>` on an uninitialized directory called `storage.Open(dbPath)`, which called `os.MkdirAll(filepath.Dir(dbPath))` and created `.senvu/db/senvu.db`. Read-only commands must never mutate user filesystems.
- **Resolution:** Added `config.WorkspaceExists(absRoot)` checks before opening storage in read-only workflows across `app.SnapshotService`. Uninitialized workspaces now return a helpful error without mutating disk.
- **Verification:** Verified by `TestAudit_ReadOnlyCommandMutatesFilesystem`.

### Finding SEC-02: Manifest Verification Masked Unexpected / Injected Files as PASSED
- **Severity:** High
- **Component:** `internal/app/verify.go`
- **Description:** In `app.Verify`, `result.Passed` was previously calculated as `len(result.Missing) == 0 && len(result.Modified) == 0 && len(result.Errors) == 0`. When rogue, unexpected files were introduced into a verified directory, they were listed under `result.Unexpected`, but the verification result reported `Verification PASSED` with exit code `0`.
- **Resolution:** Updated verification logic to strictly require `len(result.Unexpected) == 0` for `result.Passed`. Any unexpected file is treated as an integrity divergence.
- **Verification:** Verified by `TestAudit_VerifyUnexpectedFilesMustNotPass`.

### Finding SEC-03: Manifest Validation Accepted Duplicate Relative Paths and Null Bytes
- **Severity:** Medium
- **Component:** `pkg/manifest/manifest.go`
- **Description:** `Manifest.Validate()` validated basic path syntax, but permitted duplicate `RelPath` entries and unescaped null bytes (`\x00`). A maliciously crafted manifest with duplicate paths could cause shadow verification or ambiguity across engines.
- **Resolution:** Added path uniqueness tracking and null byte rejection (`strings.ContainsRune(p, 0)`) in `Manifest.Validate()`.
- **Verification:** Verified by `TestAudit_ManifestDuplicatePaths`.

### Finding SEC-04: Non-Atomic Manifest Serialization
- **Severity:** Medium
- **Component:** `pkg/manifest/manifest.go`
- **Description:** `Manifest.Save(path)` used `os.WriteFile` directly instead of atomic file writing. Process termination or crash during save could leave truncated or corrupted manifest files on disk.
- **Resolution:** Refactored `Manifest.Save()` to stage data to a temporary file in the target directory, `Sync()` to disk, and atomically rename over the target path via `platform.AtomicWrite()`.
- **Verification:** Verified by `TestLoadSaveCycle` and manifest unit tests.

### Finding SEC-05: TOCTOU Race Condition During File Hashing
- **Severity:** Medium
- **Component:** `internal/scanner/scanner.go`
- **Description:** A file could be modified or replaced with another file or symlink after the initial `os.Lstat` call but during or immediately after the hashing pass.
- **Resolution:** Implemented post-hashing verification via a secondary `os.Lstat()` check. If file size or modification time changes during the hash pass, the scan flags a modification error rather than capturing an inconsistent digest.
- **Verification:** Verified by `TestAudit_FileModifiedDuringScan`.

---

## 3. Cryptographic Verification & Standard Compliance

- **BLAKE3 Implementation:** `github.com/zeebo/blake3` verified against standard test vectors, empty input vector (`af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262`), and streaming chunks.
- **Path Traversal Protection:** Cross-platform path validation in `internal/platform/platform.go` rejects parent directory sequences (`..`), leading slashes, Windows UNC prefixes, and drive-relative escapes.
- **Symlink Policy:** Symlinks are safely recorded with their link targets, but never followed during scanning, transfers, or reconciliation, eliminating symlink loop attacks and directory escape vectors.
