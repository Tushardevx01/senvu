package compare

import (
	"testing"
	"time"

	"github.com/senvu/senvu/pkg/manifest"
)

var now = time.Now()

func entry(path, hash string, size int64) manifest.FileEntry {
	return manifest.FileEntry{
		RelPath: path,
		Hash:    hash,
		Size:    size,
		ModTime: now,
	}
}

func TestDetectAddedFiles(t *testing.T) {
	source := []manifest.FileEntry{
		entry("a.txt", "hash1", 100),
	}
	dest := []manifest.FileEntry{
		entry("a.txt", "hash1", 100),
		entry("b.txt", "hash2", 200),
	}

	result := Compare(source, dest)

	if len(result.Added) != 1 {
		t.Fatalf("expected 1 added file, got %d", len(result.Added))
	}
	if result.Added[0].RelPath != "b.txt" {
		t.Errorf("expected added file b.txt, got %s", result.Added[0].RelPath)
	}
	if result.Added[0].DestHash != "hash2" {
		t.Errorf("expected dest hash hash2, got %s", result.Added[0].DestHash)
	}
}

func TestDetectRemovedFiles(t *testing.T) {
	source := []manifest.FileEntry{
		entry("a.txt", "hash1", 100),
		entry("b.txt", "hash2", 200),
	}
	dest := []manifest.FileEntry{
		entry("a.txt", "hash1", 100),
	}

	result := Compare(source, dest)

	if len(result.Removed) != 1 {
		t.Fatalf("expected 1 removed file, got %d", len(result.Removed))
	}
	if result.Removed[0].RelPath != "b.txt" {
		t.Errorf("expected removed file b.txt, got %s", result.Removed[0].RelPath)
	}
	if result.Removed[0].SourceHash != "hash2" {
		t.Errorf("expected source hash hash2, got %s", result.Removed[0].SourceHash)
	}
}

func TestDetectModifiedFiles(t *testing.T) {
	source := []manifest.FileEntry{
		entry("a.txt", "hash1", 100),
	}
	dest := []manifest.FileEntry{
		entry("a.txt", "hash_changed", 150),
	}

	result := Compare(source, dest)

	if len(result.Modified) != 1 {
		t.Fatalf("expected 1 modified file, got %d", len(result.Modified))
	}
	if result.Modified[0].RelPath != "a.txt" {
		t.Errorf("expected modified file a.txt, got %s", result.Modified[0].RelPath)
	}
	if result.Modified[0].SourceHash != "hash1" {
		t.Errorf("expected source hash hash1, got %s", result.Modified[0].SourceHash)
	}
	if result.Modified[0].DestHash != "hash_changed" {
		t.Errorf("expected dest hash hash_changed, got %s", result.Modified[0].DestHash)
	}
}

func TestDetectUnchangedFiles(t *testing.T) {
	source := []manifest.FileEntry{
		entry("a.txt", "hash1", 100),
		entry("b.txt", "hash2", 200),
	}
	dest := []manifest.FileEntry{
		entry("a.txt", "hash1", 100),
		entry("b.txt", "hash2", 200),
	}

	result := Compare(source, dest)

	if len(result.Unchanged) != 2 {
		t.Fatalf("expected 2 unchanged files, got %d", len(result.Unchanged))
	}
	if len(result.Added) != 0 || len(result.Removed) != 0 || len(result.Modified) != 0 {
		t.Errorf("expected no other changes; added=%d removed=%d modified=%d",
			len(result.Added), len(result.Removed), len(result.Modified))
	}
}

func TestRenameDetection(t *testing.T) {
	source := []manifest.FileEntry{
		entry("old_name.txt", "hash1", 100),
	}
	dest := []manifest.FileEntry{
		entry("new_name.txt", "hash1", 100),
	}

	result := Compare(source, dest)

	if len(result.Renamed) != 1 {
		t.Fatalf("expected 1 renamed file, got %d", len(result.Renamed))
	}
	if result.Renamed[0].RelPath != "new_name.txt" {
		t.Errorf("expected new path new_name.txt, got %s", result.Renamed[0].RelPath)
	}
	if result.Renamed[0].OldPath != "old_name.txt" {
		t.Errorf("expected old path old_name.txt, got %s", result.Renamed[0].OldPath)
	}
	if len(result.Added) != 0 {
		t.Errorf("expected 0 added files, got %d", len(result.Added))
	}
	if len(result.Removed) != 0 {
		t.Errorf("expected 0 removed files, got %d", len(result.Removed))
	}
}

func TestAmbiguousRenameNotReported(t *testing.T) {
	// Two removed files and one added file share the same hash.
	// This is ambiguous, so no rename should be reported.
	source := []manifest.FileEntry{
		entry("copy1.txt", "samehash", 100),
		entry("copy2.txt", "samehash", 100),
	}
	dest := []manifest.FileEntry{
		entry("renamed.txt", "samehash", 100),
	}

	result := Compare(source, dest)

	if len(result.Renamed) != 0 {
		t.Fatalf("expected 0 renames for ambiguous case, got %d", len(result.Renamed))
	}
	// The files should be classified as added/removed instead.
	if len(result.Added) != 1 {
		t.Errorf("expected 1 added file, got %d", len(result.Added))
	}
	if len(result.Removed) != 2 {
		t.Errorf("expected 2 removed files, got %d", len(result.Removed))
	}
}

func TestAmbiguousRenameMultipleAdded(t *testing.T) {
	// One removed file and two added files share the same hash.
	// This is ambiguous, so no rename should be reported.
	source := []manifest.FileEntry{
		entry("original.txt", "samehash", 100),
	}
	dest := []manifest.FileEntry{
		entry("copy_a.txt", "samehash", 100),
		entry("copy_b.txt", "samehash", 100),
	}

	result := Compare(source, dest)

	if len(result.Renamed) != 0 {
		t.Fatalf("expected 0 renames for ambiguous case, got %d", len(result.Renamed))
	}
	if len(result.Added) != 2 {
		t.Errorf("expected 2 added files, got %d", len(result.Added))
	}
	if len(result.Removed) != 1 {
		t.Errorf("expected 1 removed file, got %d", len(result.Removed))
	}
}

func TestEmptyDirectories(t *testing.T) {
	var source, dest []manifest.FileEntry

	result := Compare(source, dest)

	if len(result.Added) != 0 || len(result.Removed) != 0 || len(result.Modified) != 0 ||
		len(result.Unchanged) != 0 || len(result.Renamed) != 0 {
		t.Errorf("expected all empty for empty inputs; got added=%d removed=%d modified=%d unchanged=%d renamed=%d",
			len(result.Added), len(result.Removed), len(result.Modified),
			len(result.Unchanged), len(result.Renamed))
	}
}

func TestComparingIdenticalSets(t *testing.T) {
	files := []manifest.FileEntry{
		entry("a.txt", "hash1", 100),
		entry("b.txt", "hash2", 200),
		entry("dir/c.txt", "hash3", 300),
	}

	result := Compare(files, files)

	if len(result.Unchanged) != 3 {
		t.Fatalf("expected 3 unchanged files, got %d", len(result.Unchanged))
	}
	if len(result.Added) != 0 || len(result.Removed) != 0 || len(result.Modified) != 0 || len(result.Renamed) != 0 {
		t.Errorf("expected no changes for identical sets")
	}
}

func TestSummaryIdentical(t *testing.T) {
	files := []manifest.FileEntry{
		entry("a.txt", "hash1", 100),
	}
	result := Compare(files, files)
	summary := result.Summary()

	if !contains(summary, "identical") {
		t.Errorf("expected summary to contain 'identical', got: %s", summary)
	}
}

func TestSummaryChanges(t *testing.T) {
	source := []manifest.FileEntry{
		entry("a.txt", "hash1", 100),
	}
	dest := []manifest.FileEntry{
		entry("a.txt", "hash2", 100),
	}
	result := Compare(source, dest)
	summary := result.Summary()

	if !contains(summary, "Changes detected") {
		t.Errorf("expected summary to contain 'Changes detected', got: %s", summary)
	}
	if !contains(summary, "Modified:  1") {
		t.Errorf("expected summary to report 1 modified, got: %s", summary)
	}
}

func TestCompareDeterministicOrder(t *testing.T) {
	source := []manifest.FileEntry{
		entry("z.txt", "hashZ", 100),
		entry("a.txt", "hashA", 100),
		entry("m.txt", "hashM", 100),
	}
	dest := []manifest.FileEntry{
		entry("z.txt", "hashZ_diff", 100),
		entry("a.txt", "hashA", 100),
		entry("b.txt", "hashB", 100),
		entry("c.txt", "hashC", 100),
	}

	result := Compare(source, dest)

	// Modified should have z.txt
	if len(result.Modified) != 1 || result.Modified[0].RelPath != "z.txt" {
		t.Fatalf("expected modified [z.txt], got %+v", result.Modified)
	}

	// Removed should have m.txt
	if len(result.Removed) != 1 || result.Removed[0].RelPath != "m.txt" {
		t.Fatalf("expected removed [m.txt], got %+v", result.Removed)
	}

	// Added should be sorted: b.txt, c.txt
	if len(result.Added) != 2 || result.Added[0].RelPath != "b.txt" || result.Added[1].RelPath != "c.txt" {
		t.Fatalf("expected added [b.txt, c.txt], got %+v", result.Added)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
