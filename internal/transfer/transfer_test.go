package transfer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/senvu/senvu/internal/integrity"
)

func TestTransferBasic(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	content := []byte("Hello, Senvu safe transfer!")
	relPath := "subdir/sample.txt"
	srcFile := filepath.Join(srcDir, relPath)
	if err := os.MkdirAll(filepath.Dir(srcFile), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	engine := NewEngine()
	opts := DefaultOptions()

	tf, err := engine.TransferFile(srcDir, dstDir, relPath, opts)
	if err != nil {
		t.Fatalf("TransferFile failed: %v", err)
	}

	if tf.RelPath != relPath {
		t.Errorf("expected relPath %s, got %s", relPath, tf.RelPath)
	}

	dstFile := filepath.Join(dstDir, relPath)
	dstData, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("failed reading transferred file: %v", err)
	}
	if string(dstData) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(dstData), string(content))
	}

	dstHash, _ := integrity.HashFile(dstFile)
	if dstHash != tf.Hash {
		t.Errorf("hash mismatch: got %s, want %s", dstHash, tf.Hash)
	}
}

func TestTransferConflictAbort(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	relPath := "conflict.txt"
	srcFile := filepath.Join(srcDir, relPath)
	dstFile := filepath.Join(dstDir, relPath)

	os.WriteFile(srcFile, []byte("source content"), 0644)
	os.WriteFile(dstFile, []byte("different destination content"), 0644)

	engine := NewEngine()
	opts := DefaultOptions() // ConflictPolicyAbort

	_, err := engine.TransferFile(srcDir, dstDir, relPath, opts)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}

	// Verify destination content was NOT modified.
	data, _ := os.ReadFile(dstFile)
	if string(data) != "different destination content" {
		t.Errorf("destination content was overwritten: %q", string(data))
	}
}

func TestTransferConflictPreserve(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	relPath := "doc.txt"
	srcFile := filepath.Join(srcDir, relPath)
	dstFile := filepath.Join(dstDir, relPath)

	os.WriteFile(srcFile, []byte("new source content"), 0644)
	os.WriteFile(dstFile, []byte("old dest content"), 0644)

	engine := NewEngine()
	opts := Options{
		ConflictPolicy:      ConflictPolicyPreserve,
		PreserveTimestamps:  true,
		PreservePermissions: true,
	}

	_, err := engine.TransferFile(srcDir, dstDir, relPath, opts)
	if err != nil {
		t.Fatalf("TransferFile failed with ConflictPolicyPreserve: %v", err)
	}

	// Verify destination has the new content.
	data, _ := os.ReadFile(dstFile)
	if string(data) != "new source content" {
		t.Errorf("dest content mismatch: got %q", string(data))
	}

	// Verify preserved conflict file exists.
	entries, err := os.ReadDir(dstDir)
	if err != nil {
		t.Fatal(err)
	}

	var foundBackup bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "doc.txt.conflict.") {
			foundBackup = true
			backupData, _ := os.ReadFile(filepath.Join(dstDir, e.Name()))
			if string(backupData) != "old dest content" {
				t.Errorf("preserved file content mismatch: %q", string(backupData))
			}
		}
	}
	if !foundBackup {
		t.Error("expected conflict backup file to exist")
	}
}

func TestTransferDryRun(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	relPath := "dryrun.txt"
	srcFile := filepath.Join(srcDir, relPath)
	dstFile := filepath.Join(dstDir, relPath)

	os.WriteFile(srcFile, []byte("dry run test"), 0644)

	engine := NewEngine()
	opts := Options{
		DryRun: true,
	}

	tf, err := engine.TransferFile(srcDir, dstDir, relPath, opts)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if tf == nil {
		t.Fatal("expected non-nil TransferredFile in dry run")
	}

	// Ensure destination file was NOT created.
	if _, err := os.Stat(dstFile); !os.IsNotExist(err) {
		t.Errorf("destination file should not exist after dry run")
	}
}

func TestTransferDirectory(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("file a"), 0644)
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("file b"), 0644)

	engine := NewEngine()
	opts := DefaultOptions()

	result, err := engine.TransferDirectory(srcDir, dstDir, opts)
	if err != nil {
		t.Fatalf("TransferDirectory failed: %v", err)
	}

	if len(result.Transferred) != 2 {
		t.Errorf("expected 2 transferred files, got %d", len(result.Transferred))
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}

	// Run again -> both should be skipped as identical!
	result2, err := engine.TransferDirectory(srcDir, dstDir, opts)
	if err != nil {
		t.Fatalf("second TransferDirectory failed: %v", err)
	}
	if len(result2.Transferred) != 0 {
		t.Errorf("expected 0 transferred on second run, got %d", len(result2.Transferred))
	}
	if len(result2.Skipped) != 2 {
		t.Errorf("expected 2 skipped files on second run, got %d", len(result2.Skipped))
	}
}

func TestTransferPathTraversalRejected(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	engine := NewEngine()
	opts := DefaultOptions()

	_, err := engine.TransferFile(srcDir, dstDir, "../evil.txt", opts)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
}

func TestTransferMetadataPreservation(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	relPath := "meta.txt"
	srcFile := filepath.Join(srcDir, relPath)
	os.WriteFile(srcFile, []byte("metadata test"), 0755)

	customTime := time.Date(2023, 5, 12, 10, 30, 0, 0, time.UTC)
	os.Chtimes(srcFile, customTime, customTime)

	engine := NewEngine()
	opts := Options{
		PreserveTimestamps:  true,
		PreservePermissions: true,
	}

	_, err := engine.TransferFile(srcDir, dstDir, relPath, opts)
	if err != nil {
		t.Fatalf("transfer failed: %v", err)
	}

	dstFile := filepath.Join(dstDir, relPath)
	dstInfo, err := os.Stat(dstFile)
	if err != nil {
		t.Fatal(err)
	}

	// Check timestamp (within 2 seconds tolerance for filesystem resolution).
	diff := dstInfo.ModTime().Sub(customTime)
	if diff < -2*time.Second || diff > 2*time.Second {
		t.Errorf("timestamp not preserved: got %v, want %v", dstInfo.ModTime(), customTime)
	}
}
