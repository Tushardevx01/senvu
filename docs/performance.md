# Senvu Performance & Scalability Report

**Benchmark Date:** 2026-11-19  
**Platform:** Linux amd64, AMD Ryzen 7 7435HS (16 threads)  
**Go Version:** 1.27  

---

## 1. Scanner & Hasher Benchmarks (1,000 Files)

### Benchmark Configuration
- **Dataset:** 1,000 files across nested subdirectories with varied sizes (1 KB to 64 KB).
- **Hasher:** Cryptographic streaming BLAKE3.
- **Engine:** Fixed worker pool with `runtime.NumCPU()` workers (16 workers).
- **Command:** `go test -bench=BenchmarkScanner1000Files -benchmem ./tests`

### Execution Metrics
```text
BenchmarkScanner1000Files-16    	      48	  23005225 ns/op	34088812 B/op	   14211 allocs/op
```

- **Throughput:** ~43,500 files per second scanned, verified, and hashed.
- **Latency:** **23.00 ms** to scan and hash 1,000 files.
- **Memory Overhead:** 34 MB per run (including in-memory manifest data structures).
- **Concurrency Scaling:** Worker pool scales with available CPU threads rather than spawning $O(N)$ goroutines.

---

## 2. Transfer Engine Optimization

### In-Stream BLAKE3 Hashing
Prior to audit remediation, `senvu transfer` performed redundant read passes:
1. **Pass 1:** Full source file read to compute `srcHash`.
2. **Pass 2:** Full source file read to write to temporary file `.senvu-transfer-*`.
3. **Pass 3:** Full read of temporary file to verify integrity before rename.

### Post-Audit Pipeline
1. **Quick Rejection:** If destination file exists, file sizes are checked first. If file sizes differ, content cannot be identical; redundant destination hashing is bypassed.
2. **In-Stream Hashing:** For new files or differing files, copying from source to temporary file feeds directly into a streaming BLAKE3 hasher (`blake3.New()`) in real-time.
3. **On-Disk Verification:** After flushing and closing the temporary file, a single read verifies the on-disk bytes before atomic rename.

### Result
- Eliminates 1 full read pass over the source file on all file transfers.
- Disk I/O reduced by ~33% for single-file transfers.
- Destination files with different lengths avoid redundant reads completely during conflict evaluation.

---

## 3. SQLite Concurrency & Scaling

- **Journal Mode:** Write-Ahead Logging (`PRAGMA journal_mode=WAL`) enables concurrent readers alongside a writer.
- **Busy Timeout:** `PRAGMA busy_timeout=10000;` ensures commands wait up to 10 seconds for locks under contention rather than failing immediately.
- **Connection Cap:** Single writer connection (`db.SetMaxOpenConns(1)`) prevents SQLite thread locking contention.
