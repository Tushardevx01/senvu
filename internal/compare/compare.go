// Package compare provides a comparison engine for detecting differences
// between two sets of file entries based on content hashes.
package compare

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/senvu/senvu/pkg/manifest"
)

// FileChange represents a single file difference between source and destination.
type FileChange struct {
	// RelPath is the relative path of the file.
	RelPath string
	// SourceHash is the content hash from the source set (empty for added files).
	SourceHash string
	// DestHash is the content hash from the destination set (empty for removed files).
	DestHash string
	// SourceSize is the file size in the source set.
	SourceSize int64
	// DestSize is the file size in the destination set.
	DestSize int64
	// SourceModTime is the modification time in the source set.
	SourceModTime time.Time
	// DestModTime is the modification time in the destination set.
	DestModTime time.Time
	// OldPath is the previous path for renamed files.
	OldPath string
}

// CompareResult holds the full comparison outcome between two sets of file entries.
type CompareResult struct {
	Added     []FileChange
	Removed   []FileChange
	Modified  []FileChange
	Unchanged []FileChange
	Renamed   []FileChange
}

// Compare compares two sets of file entries and returns a detailed result
// categorising each file as added, removed, modified, unchanged, or renamed.
//
// Rename detection is conservative: a rename is reported only when exactly one
// removed file and exactly one added file share the same content hash, making
// the mapping unambiguous.
func Compare(sourceFiles, destFiles []manifest.FileEntry) *CompareResult {
	result := &CompareResult{}

	sourceMap := make(map[string]manifest.FileEntry, len(sourceFiles))
	for _, f := range sourceFiles {
		sourceMap[f.RelPath] = f
	}

	destMap := make(map[string]manifest.FileEntry, len(destFiles))
	for _, f := range destFiles {
		destMap[f.RelPath] = f
	}

	// Files present in both source and destination.
	for path, sf := range sourceMap {
		if df, ok := destMap[path]; ok {
			if sf.Hash == df.Hash {
				result.Unchanged = append(result.Unchanged, FileChange{
					RelPath:       path,
					SourceHash:    sf.Hash,
					DestHash:      df.Hash,
					SourceSize:    sf.Size,
					DestSize:      df.Size,
					SourceModTime: sf.ModTime,
					DestModTime:   df.ModTime,
				})
			} else {
				result.Modified = append(result.Modified, FileChange{
					RelPath:       path,
					SourceHash:    sf.Hash,
					DestHash:      df.Hash,
					SourceSize:    sf.Size,
					DestSize:      df.Size,
					SourceModTime: sf.ModTime,
					DestModTime:   df.ModTime,
				})
			}
		}
	}

	// Collect candidates: files only in source (potential removes) and only
	// in dest (potential adds).
	var removedCandidates []manifest.FileEntry
	for path, sf := range sourceMap {
		if _, ok := destMap[path]; !ok {
			removedCandidates = append(removedCandidates, sf)
		}
	}

	var addedCandidates []manifest.FileEntry
	for path, df := range destMap {
		if _, ok := sourceMap[path]; !ok {
			addedCandidates = append(addedCandidates, df)
		}
	}

	// Build hash → file lists for rename detection.
	removedByHash := make(map[string][]manifest.FileEntry)
	for _, f := range removedCandidates {
		removedByHash[f.Hash] = append(removedByHash[f.Hash], f)
	}

	addedByHash := make(map[string][]manifest.FileEntry)
	for _, f := range addedCandidates {
		addedByHash[f.Hash] = append(addedByHash[f.Hash], f)
	}

	// Detect unambiguous renames: exactly one source and one dest share a hash.
	renamedSourcePaths := make(map[string]bool)
	renamedDestPaths := make(map[string]bool)

	for hash, removed := range removedByHash {
		added, ok := addedByHash[hash]
		if !ok {
			continue
		}
		if len(removed) == 1 && len(added) == 1 {
			result.Renamed = append(result.Renamed, FileChange{
				RelPath:       added[0].RelPath,
				SourceHash:    removed[0].Hash,
				DestHash:      added[0].Hash,
				SourceSize:    removed[0].Size,
				DestSize:      added[0].Size,
				SourceModTime: removed[0].ModTime,
				DestModTime:   added[0].ModTime,
				OldPath:       removed[0].RelPath,
			})
			renamedSourcePaths[removed[0].RelPath] = true
			renamedDestPaths[added[0].RelPath] = true
		}
	}

	// Remaining removed files (not part of a rename).
	for _, f := range removedCandidates {
		if renamedSourcePaths[f.RelPath] {
			continue
		}
		result.Removed = append(result.Removed, FileChange{
			RelPath:       f.RelPath,
			SourceHash:    f.Hash,
			SourceSize:    f.Size,
			SourceModTime: f.ModTime,
		})
	}

	// Remaining added files (not part of a rename).
	for _, f := range addedCandidates {
		if renamedDestPaths[f.RelPath] {
			continue
		}
		result.Added = append(result.Added, FileChange{
			RelPath:     f.RelPath,
			DestHash:    f.Hash,
			DestSize:    f.Size,
			DestModTime: f.ModTime,
		})
	}

	sort.Slice(result.Added, func(i, j int) bool { return result.Added[i].RelPath < result.Added[j].RelPath })
	sort.Slice(result.Removed, func(i, j int) bool { return result.Removed[i].RelPath < result.Removed[j].RelPath })
	sort.Slice(result.Modified, func(i, j int) bool { return result.Modified[i].RelPath < result.Modified[j].RelPath })
	sort.Slice(result.Unchanged, func(i, j int) bool { return result.Unchanged[i].RelPath < result.Unchanged[j].RelPath })
	sort.Slice(result.Renamed, func(i, j int) bool { return result.Renamed[i].RelPath < result.Renamed[j].RelPath })

	return result
}

// Summary returns a human-readable summary of the comparison result.
func (r *CompareResult) Summary() string {
	var b strings.Builder
	total := len(r.Added) + len(r.Removed) + len(r.Modified) + len(r.Unchanged) + len(r.Renamed)

	b.WriteString(fmt.Sprintf("Comparison: %d files examined\n", total))
	b.WriteString(fmt.Sprintf("  Added:     %d\n", len(r.Added)))
	b.WriteString(fmt.Sprintf("  Removed:   %d\n", len(r.Removed)))
	b.WriteString(fmt.Sprintf("  Modified:  %d\n", len(r.Modified)))
	b.WriteString(fmt.Sprintf("  Unchanged: %d\n", len(r.Unchanged)))
	b.WriteString(fmt.Sprintf("  Renamed:   %d\n", len(r.Renamed)))

	if len(r.Added) == 0 && len(r.Removed) == 0 && len(r.Modified) == 0 && len(r.Renamed) == 0 {
		b.WriteString("\nDirectories are identical.\n")
	} else {
		b.WriteString("\nChanges detected.\n")
	}

	return b.String()
}
