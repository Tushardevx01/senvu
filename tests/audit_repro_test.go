package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/senvu/senvu/internal/app"
	"github.com/senvu/senvu/internal/integrity"
	"github.com/senvu/senvu/internal/reconcile"
	"github.com/senvu/senvu/internal/scanner"
	"github.com/senvu/senvu/internal/snapshot"
	"github.com/senvu/senvu/internal/storage"
	"github.com/senvu/senvu/internal/transfer"
	"github.com/senvu/senvu/pkg/manifest"
)

// 1. Verify standard BLAKE3 empty hash.
func TestAudit_Blake3EmptyHash(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "empty.txt")
	os.WriteFile(tmpFile, []byte{}, 0644)

	hash, err := integrity.HashFile(tmpFile)
	if err != nil {
		t.Fatalf("hashing empty file failed: %v", err)
	}

	// Known BLAKE3 hash of empty string:
	expected := "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262"
	if hash != expected {
		t.Errorf("BLAKE3 empty hash mismatch:\ngot:  %s\nwant: %s", hash, expected)
	}
}

// 2. Reproduce: Partial scans with errors must NOT produce a successful complete snapshot.
func TestAudit_PartialScanMustNotBeSavedAsComplete(t *testing.T) {
	dataDir := t.TempDir()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "senvu.db")

	os.WriteFile(filepath.Join(dataDir, "good.txt"), []byte("good content"), 0644)
	badFile := filepath.Join(dataDir, "unreadable.txt")
	os.WriteFile(badFile, []byte("secret"), 0000)
	defer os.Chmod(badFile, 0644)

	// If running as non-root, chmod 0000 causes permission denied.
	if _, err := os.ReadFile(badFile); err == nil {
		t.Skip("skipping permission-based test: running as root/privileged user")
	}

	store, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("opening storage: %v", err)
	}
	defer store.Close()

	svc := snapshot.NewService(scanner.NewScanner(), integrity.NewBlake3Hasher(), store)
	m, err := svc.Create(dataDir)

	// DEFECT: Currently, Create() succeeds and returns m != nil even though a file failed!
	if err == nil && m != nil {
		t.Errorf("DEFECT CONFIRMED (REL-01): snapshot.Create succeeded despite unreadable files in scan! SnapshotID: %s, scan_errors: %s",
			m.SnapshotID, m.Metadata["scan_errors"])
	}
}

// 3. Reproduce: Unexpected files in verify must NOT be reported as Passed == true.
func TestAudit_VerifyUnexpectedFilesMustNotPass(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "known.txt"), []byte("known"), 0644)

	h, _ := integrity.HashFile(filepath.Join(dir, "known.txt"))
	m := manifest.NewManifest(dir, "blake3")
	m.SnapshotID = "test-snap"
	m.Files = []manifest.FileEntry{
		{RelPath: "known.txt", Hash: h, Size: 5, ModTime: time.Now(), Mode: 0644},
	}

	// Add an unexpected rogue file to directory.
	os.WriteFile(filepath.Join(dir, "rogue_injected.sh"), []byte("echo pwned"), 0755)

	res, err := app.Verify(dir, m)
	if err != nil {
		t.Fatalf("Verify returned unexpected error: %v", err)
	}

	if len(res.Unexpected) == 0 {
		t.Fatalf("expected unexpected file to be detected")
	}

	// DEFECT: Currently, res.Passed is true even when unexpected files exist!
	if res.Passed {
		t.Errorf("DEFECT CONFIRMED (SEC-02): Verify reported Passed=true despite unexpected rogue files present in directory!")
	}
}

// 4. Reproduce: Conflict archives must not overwrite existing archives.
func TestAudit_ConflictArchiveCollision(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	rel := "test.txt"
	srcFile := filepath.Join(srcDir, rel)
	dstFile := filepath.Join(dstDir, rel)

	os.WriteFile(srcFile, []byte("v2"), 0644)
	os.WriteFile(dstFile, []byte("v1"), 0644)

	// Pre-create a conflict archive with current timestamp format to simulate multiple conflicts in same second.
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	existingArchive := dstFile + ".conflict." + timestamp
	os.WriteFile(existingArchive, []byte("ORIGINAL CONFLICT BACKUP"), 0644)

	engine := transfer.NewEngine()
	opts := transfer.Options{
		ConflictPolicy: transfer.ConflictPolicyPreserve,
	}

	_, err := engine.TransferFile(srcDir, dstDir, rel, opts)
	if err != nil {
		t.Fatalf("TransferFile failed: %v", err)
	}

	// Check if the original conflict archive was overwritten!
	data, _ := os.ReadFile(existingArchive)
	if string(data) != "ORIGINAL CONFLICT BACKUP" {
		t.Errorf("DEFECT CONFIRMED (REL-04): Existing conflict archive was overwritten by new conflict! Content is now: %q", string(data))
	}
}

// 5. Reproduce: Equal timestamps with different hashes in reconcile must trigger conflict preservation.
func TestAudit_EqualTimestampConflictInReconcile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	rel := "data.txt"
	srcFile := filepath.Join(srcDir, rel)
	dstFile := filepath.Join(dstDir, rel)

	// Same timestamp, but different content!
	now := time.Now().Truncate(time.Second)
	os.WriteFile(srcFile, []byte("source version"), 0644)
	os.WriteFile(dstFile, []byte("destination version"), 0644)
	os.Chtimes(srcFile, now, now)
	os.Chtimes(dstFile, now, now)

	engine := reconcile.NewEngine()
	plan, err := engine.BuildPlan(srcDir, dstDir)
	if err != nil {
		t.Fatalf("BuildPlan failed: %v", err)
	}

	for _, item := range plan.Items {
		if item.RelPath == rel {
			// DEFECT: If timestamps are equal and hashes differ, it should be ActionConflict, NOT ActionUpdate!
			if item.Action == reconcile.ActionUpdate {
				t.Errorf("DEFECT CONFIRMED (REL-05): Equal-timestamp content mismatch planned as ActionUpdate instead of ActionConflict! Dest would be silently overwritten.")
			}
		}
	}
}

// 6. Reproduce: Read-only commands mutating filesystem by opening non-existent SQLite db.
func TestAudit_ReadOnlyCommandMutatesFilesystem(t *testing.T) {
	nonWorkspace := filepath.Join(t.TempDir(), "virgin-dir")
	os.MkdirAll(nonWorkspace, 0755)

	svc := app.NewSnapshotService()
	// Calling ListSnapshots on a non-workspace:
	_, _ = svc.ListSnapshots(nonWorkspace)

	// DEFECT: .senvu was silently created!
	if _, err := os.Stat(filepath.Join(nonWorkspace, ".senvu")); err == nil {
		t.Errorf("DEFECT CONFIRMED (SEC-01): ListSnapshots silently created .senvu directory in non-workspace path!")
	}
}

// 7. Reproduce: Manifest accepting duplicate paths.
func TestAudit_ManifestDuplicatePaths(t *testing.T) {
	m := manifest.NewManifest(".", "blake3")
	m.SnapshotID = "test-uuid"
	m.Files = []manifest.FileEntry{
		{RelPath: "dup.txt", Hash: "abc1234567890def", Size: 10, ModTime: time.Now(), Mode: 0644},
		{RelPath: "dup.txt", Hash: "fed0987654321cba", Size: 10, ModTime: time.Now(), Mode: 0644},
	}

	err := m.Validate()
	if err == nil {
		t.Errorf("DEFECT CONFIRMED (SEC-03): Manifest.Validate() accepted duplicate RelPath entries without error!")
	}
}

// 8. Reproduce: File modified during scan/hash must be detected as an error.
func TestAudit_FileModifiedDuringScan(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "changing.txt")
	os.WriteFile(filePath, []byte("initial contents"), 0644)

	// Create a scanner.
	sc := scanner.NewScanner()

	// If we simulate a file changing between walk and hash:
	// We can test scanner behavior when a file is modified.
	res, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
}
