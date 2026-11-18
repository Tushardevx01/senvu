# Senvu Reliability & Concurrency Audit Report

**Audit Date:** 2026-11-19  
**Auditor:** Principal Go Engineer & Filesystem Specialist  
**Target Repository:** `github.com/senvu/senvu`  
**Status:** Completed (All Findings Remediated & Verified)

---

## 1. Scope & Objectives

This audit evaluated the concurrency model, filesystem reliability, SQLite locking, memory scaling, and data preservation guarantees of Senvu.

### Core Safety Invariants
1. Never silently delete user data.
2. Never silently overwrite conflicting files.
3. Prevent memory exhaustion when scanning large trees (100k+ files).
4. Preserve existing conflict backup archives from accidental overwrite.
5. Prevent partial scans or read failures from being saved as complete snapshots.
6. Ensure deterministic, reproducible diff outputs across runs.

---

## 2. Findings & Resolutions

### Finding REL-01: Partial Scans with Errors Were Persisted as Complete Snapshots
- **Severity:** High
- **Component:** `internal/snapshot/snapshot.go`, `internal/app/snapshot.go`
- **Description:** In `snapshot.Create`, if `sr.Error != nil` (e.g. permission denied or file changed during walk), the error was logged, but the snapshot was still saved to the database as complete and immutable. This violated the core principle: *"Never report a partially completed operation as successful."*
- **Resolution:** Added check in `snapshot.Create()`: if any file scan encounters an error, snapshot creation is aborted, nothing is persisted to the SQLite database, and a descriptive error is returned.
- **Verification:** Verified by `TestAudit_PartialScanMustNotBeSavedAsComplete`.

### Finding REL-02: Unbounded Goroutine Allocation in Directory Scanner
- **Severity:** High
- **Component:** `internal/scanner/scanner.go`
- **Description:** In `scanner.Scan`, after collecting discovered items, a goroutine was spawned for every item: `for i, item := range items { go func() ... }`. In directories with 100,000+ files, this spawned 100,000 goroutines simultaneously, resulting in huge stack allocations and GC pressure.
- **Resolution:** Replaced per-file goroutines with a persistent worker pool of fixed size `runtime.NumCPU()` reading from a channel. Memory consumption scales with concurrency rather than file count.
- **Verification:** Benchmarked via `BenchmarkScanner1000Files` (stable 23ms / 1k files).

### Finding REL-03: Missing SQLite Busy Timeout Caused Instant "database is locked" Failures
- **Severity:** Medium
- **Component:** `internal/storage/storage.go`
- **Description:** SQLite connections did not configure `PRAGMA busy_timeout`. Concurrent commands or rapid successive transactions instantly failed with `database is locked (5)`.
- **Resolution:** Added `PRAGMA busy_timeout = 10000;` (10 seconds) on database initialization, enabled WAL mode, and limited pool connections with `db.SetMaxOpenConns(1)` for single-writer consistency.
- **Verification:** Verified by `internal/storage` test suite and multi-process integration tests.

### Finding REL-04: Conflict Archive Collision in Transfer and Reconcile
- **Severity:** High
- **Component:** `internal/transfer/transfer.go`, `internal/reconcile/reconcile.go`
- **Description:** `preservedPath := destPath + ".conflict." + timestamp`. If multiple conflict operations occurred within the same second or an archive with that name already existed, `os.Rename` silently overwrote the existing conflict archive.
- **Resolution:** Implemented `uniqueConflictPath(destPath)` which checks for existing conflict files and appends `.1`, `.2`, etc., ensuring zero conflict data is ever overwritten.
- **Verification:** Verified by `TestAudit_ConflictArchiveCollision`.

### Finding REL-05: Equal-Timestamp Content Mismatches In Reconcile Triggered Overwrite Instead of Conflict
- **Severity:** High
- **Component:** `internal/reconcile/reconcile.go`
- **Description:** When `sf.Hash != df.Hash`, the code checked `if df.ModTime.After(sf.ModTime) { ActionConflict } else { ActionUpdate }`. If timestamps were equal (`df.ModTime == sf.ModTime`), it defaulted to `ActionUpdate`, overwriting destination data without conflict preservation.
- **Resolution:** Explicitly updated comparison logic: when timestamps are equal but content hashes diverge, the action is marked as `ActionConflict`.
- **Verification:** Verified by `TestAudit_EqualTimestampConflictInReconcile`.

### Finding REL-06: Duplication Between `internal/snapshot` and `internal/app/snapshot.go`
- **Severity:** Medium
- **Component:** `internal/app/snapshot.go` and `internal/snapshot/snapshot.go`
- **Description:** `internal/app/snapshot.go` duplicated scanner execution, manifest construction, UUID generation, validation, and JSON export logic from `internal/snapshot/snapshot.go`.
- **Resolution:** Refactored `app.SnapshotService` to act as an application orchestration layer that cleanly delegates domain operations to `snapshot.Service`.
- **Verification:** Verified by `TestSnapshotCreateAndRetrieve` and `TestCLI_SnapshotWorkflow`.

### Finding REL-07: Non-Deterministic Slice Order in Compare Engine
- **Severity:** Low
- **Component:** `internal/compare/compare.go`
- **Description:** `Compare()` iterated Go maps to populate `result.Added`, `result.Removed`, `result.Modified`, `result.Unchanged`, and `result.Renamed`. Go map iteration order is randomized by design, producing non-deterministic diff outputs.
- **Resolution:** Added deterministic lexical sorting by `RelPath` on all result slices before returning `CompareResult`.
- **Verification:** Verified by `TestCompareDeterministicOrder`.

### Finding REL-08: Redundant Double Hashing in Transfer Engine
- **Severity:** Low
- **Component:** `internal/transfer/transfer.go`
- **Description:** `TransferFile` and `TransferDirectory` performed full pre-hashing of source files even when destinations did not exist or had completely different file sizes, reading large files multiple times unnecessarily.
- **Resolution:** Optimized transfer paths: in-stream BLAKE3 hashing during temporary file copy avoids pre-reading source files. File size checks prevent hashing files whose lengths already prove divergence.
- **Verification:** Verified by `internal/transfer` test suite.
