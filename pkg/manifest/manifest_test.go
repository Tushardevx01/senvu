package manifest

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validManifest() *Manifest {
	return &Manifest{
		SchemaVersion: 1,
		SenvuVersion:  "0.1.0",
		SnapshotID:    "test-uuid-1234",
		CreatedAt:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		SourcePath:    ".",
		HashAlgorithm: "blake3",
		TotalSize:     42,
		TotalFiles:    2,
		Files: []FileEntry{
			{
				RelPath: "file1.txt",
				Size:    20,
				ModTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Hash:    "abcdef1234567890",
				Mode:    0644,
			},
			{
				RelPath: "subdir/file2.txt",
				Size:    22,
				ModTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Hash:    "1234567890abcdef",
				Mode:    0644,
			},
		},
	}
}

func TestSerializationRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	m := validManifest()

	if err := m.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.SchemaVersion != m.SchemaVersion {
		t.Errorf("SchemaVersion mismatch: got %d, want %d", loaded.SchemaVersion, m.SchemaVersion)
	}
	if loaded.SnapshotID != m.SnapshotID {
		t.Errorf("SnapshotID mismatch: got %q, want %q", loaded.SnapshotID, m.SnapshotID)
	}
	if loaded.TotalFiles != m.TotalFiles {
		t.Errorf("TotalFiles mismatch: got %d, want %d", loaded.TotalFiles, m.TotalFiles)
	}
	if len(loaded.Files) != len(m.Files) {
		t.Fatalf("Files count mismatch: got %d, want %d", len(loaded.Files), len(m.Files))
	}
	for i, f := range loaded.Files {
		if f.RelPath != m.Files[i].RelPath {
			t.Errorf("file %d RelPath mismatch: got %q, want %q", i, f.RelPath, m.Files[i].RelPath)
		}
		if f.Hash != m.Files[i].Hash {
			t.Errorf("file %d Hash mismatch: got %q, want %q", i, f.Hash, m.Files[i].Hash)
		}
	}
}

func TestValidateRejectsPathTraversal(t *testing.T) {
	m := validManifest()
	m.Files[0].RelPath = "../evil/file.txt"

	err := m.Validate()
	if err == nil {
		t.Fatal("expected validation error for path traversal")
	}
	t.Logf("got expected error: %v", err)
}

func TestValidateRejectsAbsolutePaths(t *testing.T) {
	m := validManifest()
	m.Files[0].RelPath = "/etc/passwd"

	err := m.Validate()
	if err == nil {
		t.Fatal("expected validation error for absolute path")
	}
	t.Logf("got expected error: %v", err)
}

func TestValidateRejectsEmptyHashes(t *testing.T) {
	m := validManifest()
	m.Files[0].Hash = ""

	err := m.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty hash")
	}
	t.Logf("got expected error: %v", err)
}

func TestValidateSchemaVersion(t *testing.T) {
	tests := []struct {
		version int
		wantErr bool
	}{
		{0, true},
		{1, false},
		{2, true},
		{-1, true},
	}

	for _, tt := range tests {
		m := validManifest()
		m.SchemaVersion = tt.version
		err := m.Validate()
		if (err != nil) != tt.wantErr {
			t.Errorf("SchemaVersion=%d: wantErr=%v, got %v", tt.version, tt.wantErr, err)
		}
	}
}

func TestLoadSaveCycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-manifest.json")

	original := validManifest()
	if err := original.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("manifest file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("manifest file is empty")
	}

	// Load and re-save should produce valid manifest.
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	path2 := filepath.Join(dir, "resaved.json")
	if err := loaded.Save(path2); err != nil {
		t.Fatalf("re-Save failed: %v", err)
	}

	reloaded, err := Load(path2)
	if err != nil {
		t.Fatalf("re-Load failed: %v", err)
	}

	if reloaded.SnapshotID != original.SnapshotID {
		t.Errorf("SnapshotID changed after round-trip: got %q, want %q",
			reloaded.SnapshotID, original.SnapshotID)
	}
}

func TestValidateRejectsEmptySnapshotID(t *testing.T) {
	m := validManifest()
	m.SnapshotID = ""

	err := m.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty snapshot ID")
	}
}

func TestValidateRejectsEmbeddedTraversal(t *testing.T) {
	m := validManifest()
	m.Files[0].RelPath = "foo/../../bar"

	err := m.Validate()
	if err == nil {
		t.Fatal("expected validation error for embedded path traversal")
	}
}

func TestValidateUnicodePaths(t *testing.T) {
	m := validManifest()
	m.Files[0].RelPath = "日本語/ファイル.txt"

	err := m.Validate()
	if err != nil {
		t.Fatalf("unexpected error for unicode path: %v", err)
	}
}
