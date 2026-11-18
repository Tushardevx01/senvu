package app

import (
	"fmt"

	"github.com/senvu/senvu/internal/compare"
	"github.com/senvu/senvu/internal/scanner"
	"github.com/senvu/senvu/pkg/manifest"
)

// CompareDirectories scans two directories and returns a comparison of their
// file contents, using content hashes for accurate difference detection.
func CompareDirectories(sourcePath, destPath string) (*compare.CompareResult, error) {
	s := scanner.NewScanner()

	sourceResults, err := s.Scan(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("compare: scanning source %q: %w", sourcePath, err)
	}

	destResults, err := s.Scan(destPath)
	if err != nil {
		return nil, fmt.Errorf("compare: scanning dest %q: %w", destPath, err)
	}

	sourceFiles := scanResultsToFileEntries(sourceResults)
	destFiles := scanResultsToFileEntries(destResults)

	return compare.Compare(sourceFiles, destFiles), nil
}

// scanResultsToFileEntries converts scanner results into manifest file entries.
func scanResultsToFileEntries(results []scanner.ScanResult) []manifest.FileEntry {
	var entries []manifest.FileEntry
	for _, sr := range results {
		if sr.Error != nil || sr.IsSymlink {
			continue // skip files with scan errors or symlinks
		}

		entries = append(entries, manifest.FileEntry{
			RelPath: sr.RelPath,
			Hash:    sr.Hash,
			Size:    sr.Size,
			ModTime: sr.ModTime,
			Mode:    sr.Mode,
		})
	}
	return entries
}
