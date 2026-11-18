package integrity

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeebo/blake3"
)

func TestBlake3HasherInterfaceCompliance(t *testing.T) {
	var _ Hasher = (*Blake3Hasher)(nil)
}

func TestBlake3HasherAlgorithm(t *testing.T) {
	h := NewBlake3Hasher()
	if h.Algorithm() != "blake3" {
		t.Errorf("expected algorithm 'blake3', got %q", h.Algorithm())
	}
}

func TestBlake3HasherKnownContent(t *testing.T) {
	// Compute expected hash using the blake3 library directly on known content.
	content := []byte("hello, senvu")
	expected := func() string {
		h := blake3.New()
		h.Write(content)
		return hex.EncodeToString(h.Sum(nil))
	}()

	hasher := NewBlake3Hasher()
	got, err := hasher.Hash(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("hash mismatch: got %q, want %q", got, expected)
	}
}

func TestBlake3HasherEmptyInput(t *testing.T) {
	// Hash of empty input should match the blake3 library's empty hash.
	expected := func() string {
		h := blake3.New()
		return hex.EncodeToString(h.Sum(nil))
	}()

	hasher := NewBlake3Hasher()
	got, err := hasher.Hash(bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("empty hash mismatch: got %q, want %q", got, expected)
	}
}

func TestBlake3HasherLargeContent(t *testing.T) {
	// 1 MB of random data to verify streaming works correctly.
	data := make([]byte, 1<<20)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("failed to generate random data: %v", err)
	}

	expected := func() string {
		h := blake3.New()
		h.Write(data)
		return hex.EncodeToString(h.Sum(nil))
	}()

	hasher := NewBlake3Hasher()
	got, err := hasher.Hash(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("large content hash mismatch: got %q, want %q", got, expected)
	}
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("file content for hashing")
	path := filepath.Join(dir, "testfile.txt")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	hash, err := HashFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	// Verify it matches direct computation.
	expected := func() string {
		h := blake3.New()
		h.Write(content)
		return hex.EncodeToString(h.Sum(nil))
	}()
	if hash != expected {
		t.Errorf("HashFile mismatch: got %q, want %q", hash, expected)
	}
}

func TestHashFileNonexistent(t *testing.T) {
	_, err := HashFile("/nonexistent/path/file.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "opening file") {
		t.Errorf("expected 'opening file' in error, got: %v", err)
	}
}
