package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/senvu/senvu/internal/integrity"
	"github.com/senvu/senvu/internal/scanner"
	"github.com/senvu/senvu/internal/storage"
)

func TestSnapshotCreateAndRetrieve(t *testing.T) {
	dataDir := t.TempDir()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "senvu.db")

	os.WriteFile(filepath.Join(dataDir, "f1.txt"), []byte("data 1"), 0644)
	os.MkdirAll(filepath.Join(dataDir, "nested"), 0755)
	os.WriteFile(filepath.Join(dataDir, "nested", "f2.txt"), []byte("data 2"), 0644)

	store, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer store.Close()

	hasher := integrity.NewBlake3Hasher()
	sc := scanner.NewScanner()
	svc := NewService(sc, hasher, store)

	m, err := svc.Create(dataDir)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if m.SnapshotID == "" {
		t.Errorf("expected non-empty snapshot ID")
	}
	if len(m.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(m.Files))
	}
	if m.TotalSize != 12 { // "data 1" (6) + "data 2" (6)
		t.Errorf("expected total size 12, got %d", m.TotalSize)
	}

	// List snapshots.
	list, err := svc.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 snapshot in list, got %d", len(list))
	}
	if list[0].SnapshotID != m.SnapshotID {
		t.Errorf("expected snapshot ID %s, got %s", m.SnapshotID, list[0].SnapshotID)
	}

	// Show snapshot.
	retrieved, err := svc.Show(m.SnapshotID)
	if err != nil {
		t.Fatalf("Show failed: %v", err)
	}
	if retrieved.SnapshotID != m.SnapshotID {
		t.Errorf("retrieved ID mismatch")
	}
	if len(retrieved.Files) != 2 {
		t.Errorf("retrieved files count mismatch: got %d, want 2", len(retrieved.Files))
	}

	// Export snapshot.
	exportPath := filepath.Join(t.TempDir(), "exported-manifest.json")
	if err := svc.Export(m.SnapshotID, exportPath); err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if _, err := os.Stat(exportPath); err != nil {
		t.Errorf("exported file does not exist: %v", err)
	}
}
