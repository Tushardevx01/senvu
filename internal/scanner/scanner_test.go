package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/senvu/senvu/internal/integrity"
)

func newTestScanner() *Scanner {
	return New(integrity.NewBlake3Hasher())
}

func TestScanEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	s := newTestScanner()

	results, err := s.Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestScanDirectoryWithFiles(t *testing.T) {
	dir := t.TempDir()

	// Create some test files.
	files := map[string]string{
		"file1.txt":         "hello",
		"file2.txt":         "world",
		"subdir/nested.txt": "nested content",
	}
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	s := newTestScanner()
	results, err := s.Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify all files are found.
	found := make(map[string]bool)
	for _, r := range results {
		found[r.RelPath] = true
		if r.Hash == "" {
			t.Errorf("empty hash for %s", r.RelPath)
		}
		if r.Error != nil {
			t.Errorf("unexpected error for %s: %v", r.RelPath, r.Error)
		}
	}

	for name := range files {
		normalized := strings.ReplaceAll(name, string(filepath.Separator), "/")
		if !found[normalized] {
			t.Errorf("file %q not found in results", normalized)
		}
	}
}

func TestScanSkipsSenvuDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create a regular file and a file inside .senvu/.
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	senvuDir := filepath.Join(dir, ".senvu")
	if err := os.MkdirAll(senvuDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(senvuDir, "config.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	s := newTestScanner()
	results, err := s.Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, r := range results {
		if strings.HasPrefix(r.RelPath, ".senvu") {
			t.Errorf("should have skipped .senvu/ but found: %s", r.RelPath)
		}
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestScanHandlesPermissionErrors(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("permission test not reliable on Windows or as root")
	}

	dir := t.TempDir()

	// Create a readable file and an unreadable file.
	if err := os.WriteFile(filepath.Join(dir, "readable.txt"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(dir, "unreadable.txt")
	if err := os.WriteFile(unreadable, []byte("secret"), 0000); err != nil {
		t.Fatal(err)
	}

	s := newTestScanner()
	results, err := s.Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error (scan should not abort): %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Find the unreadable file result — it should have an error.
	var foundErr bool
	for _, r := range results {
		if r.RelPath == "unreadable.txt" {
			if r.Error == nil {
				t.Error("expected error for unreadable file")
			}
			foundErr = true
		}
	}
	if !foundErr {
		t.Error("unreadable.txt not found in results")
	}
}

func TestScanSymlinkDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test not reliable on Windows")
	}

	dir := t.TempDir()

	// Create a real file and a symlink to it.
	realFile := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(realFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(dir, "link.txt")
	if err := os.Symlink(realFile, symlink); err != nil {
		t.Fatal(err)
	}

	s := newTestScanner()
	results, err := s.Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var foundSymlink bool
	for _, r := range results {
		if r.RelPath == "link.txt" {
			if !r.IsSymlink {
				t.Error("expected link.txt to be detected as symlink")
			}
			foundSymlink = true
		}
	}
	if !foundSymlink {
		t.Error("symlink not found in results")
	}
}

func TestScanPathNormalization(t *testing.T) {
	dir := t.TempDir()

	// Create nested file.
	nested := filepath.Join(dir, "a", "b", "c.txt")
	if err := os.MkdirAll(filepath.Dir(nested), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("deep"), 0644); err != nil {
		t.Fatal(err)
	}

	s := newTestScanner()
	results, err := s.Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	expected := "a/b/c.txt"
	if results[0].RelPath != expected {
		t.Errorf("path not normalized: got %q, want %q", results[0].RelPath, expected)
	}
}

func TestScanProgress(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, strings.Repeat("f", i+1)+".txt")
		if err := os.WriteFile(name, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var count atomic.Int32
	s := New(integrity.NewBlake3Hasher(), WithProgress(func(processed int, path string) {
		count.Add(1)
	}))

	results, err := s.Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if int(count.Load()) != len(results) {
		t.Errorf("progress called %d times, expected %d", count.Load(), len(results))
	}
}
