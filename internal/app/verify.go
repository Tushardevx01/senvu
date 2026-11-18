package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/senvu/senvu/internal/compare"
	"github.com/senvu/senvu/internal/scanner"
	"github.com/senvu/senvu/pkg/manifest"
)

// VerifyExitPass indicates all files verified successfully.
const VerifyExitPass = 0

// VerifyExitFail indicates verification found differences.
const VerifyExitFail = 1

// VerifyExitError indicates an error prevented verification from completing.
const VerifyExitError = 2

// FileError represents an error encountered while processing a specific file.
type FileError struct {
	// RelPath is the relative path of the file that caused the error.
	RelPath string `json:"rel_path"`
	// Err is the underlying error string.
	Err string `json:"error"`
}

func (e FileError) Error() string {
	return fmt.Sprintf("%s: %s", e.RelPath, e.Err)
}

// VerifyResult holds the outcome of verifying files against a manifest.
type VerifyResult struct {
	// Verified lists files that matched the manifest.
	Verified []manifest.FileEntry `json:"verified"`
	// Missing lists relative paths of files in the manifest but not on disk.
	Missing []string `json:"missing"`
	// Modified lists files whose content hash differs from the manifest.
	Modified []compare.FileChange `json:"modified"`
	// Unexpected lists relative paths of files on disk but not in the manifest.
	Unexpected []string `json:"unexpected"`
	// Errors lists files that could not be verified due to errors.
	Errors []FileError `json:"errors"`
	// Passed is true only when there are zero missing, zero modified, and zero errors.
	Passed bool `json:"passed"`
}

// ExitCode returns the appropriate exit code for the verification result.
func (r *VerifyResult) ExitCode() int {
	if len(r.Errors) > 0 {
		return VerifyExitError
	}
	if r.Passed {
		return VerifyExitPass
	}
	return VerifyExitFail
}

// Verify checks the files under rootPath against the given manifest and
// reports any discrepancies. It uses the scanner to get the current state of
// the directory, then compares each file's hash against the manifest.
func Verify(rootPath string, m *manifest.Manifest) (*VerifyResult, error) {
	if m == nil {
		return nil, fmt.Errorf("verify: manifest is nil")
	}

	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("verify: cannot resolve root path: %w", err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("verify: cannot access root path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("verify: root path is not a directory: %s", absRoot)
	}

	// Scan the current state of the directory with concurrent hashing.
	s := scanner.NewScanner()
	scanResults, err := s.Scan(absRoot)
	if err != nil {
		return nil, fmt.Errorf("verify: scan failed: %w", err)
	}

	result := &VerifyResult{
		Verified:   make([]manifest.FileEntry, 0),
		Missing:    make([]string, 0),
		Modified:   make([]compare.FileChange, 0),
		Unexpected: make([]string, 0),
		Errors:     make([]FileError, 0),
	}

	// Build a map of files currently on disk.
	diskFiles := make(map[string]scanner.ScanResult, len(scanResults))
	for _, sr := range scanResults {
		if sr.Error != nil {
			result.Errors = append(result.Errors, FileError{
				RelPath: sr.RelPath,
				Err:     sr.Error.Error(),
			})
			continue
		}
		if sr.IsSymlink {
			continue // skip symlinks
		}
		diskFiles[sr.RelPath] = sr
	}

	// Build a set of manifest paths for unexpected file detection.
	manifestPaths := make(map[string]bool, len(m.Files))
	for _, fe := range m.Files {
		manifestPaths[fe.RelPath] = true
	}

	// Check each file in the manifest against what is on disk.
	for _, fe := range m.Files {
		sr, onDisk := diskFiles[fe.RelPath]
		if !onDisk {
			result.Missing = append(result.Missing, fe.RelPath)
			continue
		}

		if sr.Hash != fe.Hash {
			result.Modified = append(result.Modified, compare.FileChange{
				RelPath:       fe.RelPath,
				SourceHash:    fe.Hash,
				DestHash:      sr.Hash,
				SourceSize:    fe.Size,
				DestSize:      sr.Size,
				SourceModTime: fe.ModTime,
				DestModTime:   sr.ModTime,
			})
		} else {
			result.Verified = append(result.Verified, fe)
		}
	}

	// Detect unexpected files (on disk but not in manifest).
	for relPath := range diskFiles {
		if !manifestPaths[relPath] {
			result.Unexpected = append(result.Unexpected, relPath)
		}
	}

	sort.Strings(result.Missing)
	sort.Strings(result.Unexpected)

	// Passed is true only if zero missing, zero modified, and zero errors.
	result.Passed = len(result.Missing) == 0 && len(result.Modified) == 0 && len(result.Errors) == 0

	return result, nil
}
