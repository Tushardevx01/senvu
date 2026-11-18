package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/senvu/senvu/pkg/manifest"
)

func testDB(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	return store
}

func testManifest() *manifest.Manifest {
	return &manifest.Manifest{
		SchemaVersion: 1,
		SenvuVersion:  "0.1.0",
		SnapshotID:    "snap-test-001",
		CreatedAt:     time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
		SourcePath:    ".",
		HashAlgorithm: "blake3",
		TotalSize:     150,
		TotalFiles:    3,
		Files: []manifest.FileEntry{
			{RelPath: "README.md", Size: 50, ModTime: time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC), Hash: "aaa111", Mode: 0644},
			{RelPath: "src/main.go", Size: 60, ModTime: time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC), Hash: "bbb222", Mode: 0644},
			{RelPath: "src/util.go", Size: 40, ModTime: time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC), Hash: "ccc333", Mode: 0644},
		},
	}
}

func TestOpenCreateDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "new.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer store.Close()

	// Verify the file was created.
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file not created: %v", err)
	}
}

func TestSaveAndRetrieveSnapshot(t *testing.T) {
	store := testDB(t)
	m := testManifest()

	if err := store.SaveSnapshot(m); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	// Retrieve it.
	got, err := store.GetSnapshot(m.SnapshotID)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}

	if got.SnapshotID != m.SnapshotID {
		t.Errorf("SnapshotID mismatch: got %q, want %q", got.SnapshotID, m.SnapshotID)
	}
	if got.TotalFiles != m.TotalFiles {
		t.Errorf("TotalFiles mismatch: got %d, want %d", got.TotalFiles, m.TotalFiles)
	}
	if got.TotalSize != m.TotalSize {
		t.Errorf("TotalSize mismatch: got %d, want %d", got.TotalSize, m.TotalSize)
	}
	if len(got.Files) != len(m.Files) {
		t.Fatalf("Files count mismatch: got %d, want %d", len(got.Files), len(m.Files))
	}
	for i, f := range got.Files {
		if f.RelPath != m.Files[i].RelPath {
			t.Errorf("file %d RelPath mismatch: got %q, want %q", i, f.RelPath, m.Files[i].RelPath)
		}
		if f.Hash != m.Files[i].Hash {
			t.Errorf("file %d Hash mismatch: got %q, want %q", i, f.Hash, m.Files[i].Hash)
		}
	}
}

func TestListSnapshots(t *testing.T) {
	store := testDB(t)

	// Save two snapshots.
	m1 := testManifest()
	m1.SnapshotID = "snap-001"
	m1.CreatedAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	m2 := testManifest()
	m2.SnapshotID = "snap-002"
	m2.CreatedAt = time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

	if err := store.SaveSnapshot(m1); err != nil {
		t.Fatalf("SaveSnapshot m1 failed: %v", err)
	}
	if err := store.SaveSnapshot(m2); err != nil {
		t.Fatalf("SaveSnapshot m2 failed: %v", err)
	}

	summaries, err := store.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	// Newest first.
	if summaries[0].SnapshotID != "snap-002" {
		t.Errorf("expected newest first, got %q", summaries[0].SnapshotID)
	}
	if summaries[1].SnapshotID != "snap-001" {
		t.Errorf("expected oldest second, got %q", summaries[1].SnapshotID)
	}
}

func TestGetSnapshotNotFound(t *testing.T) {
	store := testDB(t)

	_, err := store.GetSnapshot("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent snapshot")
	}
}

func TestSchemaMigration(t *testing.T) {
	store := testDB(t)

	version, err := store.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}
	if version != 1 {
		t.Errorf("expected schema version 1, got %d", version)
	}

	// Opening the same database again should not fail (migration is idempotent).
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "idempotent.db")

	store1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first open failed: %v", err)
	}
	store1.Close()

	store2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second open failed: %v", err)
	}
	defer store2.Close()

	v, err := store2.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion on reopened db failed: %v", err)
	}
	if v != 1 {
		t.Errorf("schema version on reopened db: got %d, want 1", v)
	}
}

func TestGetSnapshotFiles(t *testing.T) {
	store := testDB(t)
	m := testManifest()

	if err := store.SaveSnapshot(m); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	files, err := store.GetSnapshotFiles(m.SnapshotID)
	if err != nil {
		t.Fatalf("GetSnapshotFiles failed: %v", err)
	}

	if len(files) != len(m.Files) {
		t.Fatalf("file count mismatch: got %d, want %d", len(files), len(m.Files))
	}

	// Files should be ordered by rel_path.
	for i := 1; i < len(files); i++ {
		if files[i].RelPath < files[i-1].RelPath {
			t.Errorf("files not sorted: %q < %q", files[i].RelPath, files[i-1].RelPath)
		}
	}
}
