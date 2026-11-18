// Package transfer provides safe, verifiable file transfer operations
// between source and destination directories. It enforces destination path safety,
// atomic writes, conflict detection, content hash verification, and metadata preservation.
package transfer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/senvu/senvu/internal/integrity"
	"github.com/senvu/senvu/internal/platform"
)

// ConflictPolicy defines how transfer handles an existing file with different content.
type ConflictPolicy int

const (
	// ConflictPolicyAbort refuses to overwrite and records a conflict error (safest default).
	ConflictPolicyAbort ConflictPolicy = iota
	// ConflictPolicyPreserve renames existing destination file to .conflict.<timestamp>.
	ConflictPolicyPreserve
	// ConflictPolicyOverwrite overwrites destination file only when explicitly requested.
	ConflictPolicyOverwrite
)

// Options configures a transfer operation.
type Options struct {
	// ConflictPolicy defines how conflicts are handled. Default: ConflictPolicyAbort.
	ConflictPolicy ConflictPolicy
	// DryRun previews the transfer without modifying any files.
	DryRun bool
	// PreserveTimestamps preserves source ModTime on destination. Default: true.
	PreserveTimestamps bool
	// PreservePermissions preserves source file mode. Default: true.
	PreservePermissions bool
	// OnProgress is an optional progress callback.
	OnProgress func(transferredBytes, totalBytes int64, currentFile string)
}

// DefaultOptions returns the safest default options.
func DefaultOptions() Options {
	return Options{
		ConflictPolicy:      ConflictPolicyAbort,
		DryRun:              false,
		PreserveTimestamps:  true,
		PreservePermissions: true,
	}
}

// TransferredFile records metadata about a successfully transferred file.
type TransferredFile struct {
	RelPath string    `json:"rel_path"`
	Size    int64     `json:"size"`
	Hash    string    `json:"hash"`
	ModTime time.Time `json:"mod_time"`
}

// SkippedFile records a file that was skipped (e.g. already identical).
type SkippedFile struct {
	RelPath string `json:"rel_path"`
	Reason  string `json:"reason"`
}

// ConflictFile records a conflicting file at the destination.
type ConflictFile struct {
	RelPath       string `json:"rel_path"`
	SourceHash    string `json:"source_hash"`
	DestHash      string `json:"dest_hash"`
	ActionTaken   string `json:"action_taken"`
	PreservedPath string `json:"preserved_path,omitempty"`
}

// TransferError records a file-specific failure.
type TransferError struct {
	RelPath string `json:"rel_path"`
	Err     string `json:"error"`
}

func (e TransferError) Error() string {
	return fmt.Sprintf("%s: %s", e.RelPath, e.Err)
}

// Result summarizes the outcome of a transfer operation.
type Result struct {
	SourceRoot            string            `json:"source_root"`
	DestRoot              string            `json:"dest_root"`
	Transferred           []TransferredFile `json:"transferred"`
	Skipped               []SkippedFile     `json:"skipped"`
	Conflicts             []ConflictFile    `json:"conflicts"`
	Errors                []TransferError   `json:"errors"`
	TotalBytesTransferred int64             `json:"total_bytes_transferred"`
	DryRun                bool              `json:"dry_run"`
}

// Engine performs transfers with safety guarantees.
type Engine struct {
	hasher integrity.Hasher
}

// NewEngine creates a new transfer Engine.
func NewEngine() *Engine {
	return &Engine{
		hasher: integrity.NewBlake3Hasher(),
	}
}

// TransferFile transfers a single file relative to sourceRoot into destRoot.
func (e *Engine) TransferFile(sourceRoot, destRoot, relPath string, opts Options) (*TransferredFile, error) {
	// Validate paths against path traversal.
	absSourceRoot, err := filepath.Abs(sourceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving source root: %w", err)
	}
	absDestRoot, err := filepath.Abs(destRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving destination root: %w", err)
	}

	srcPath, err := platform.SafePath(absSourceRoot, relPath)
	if err != nil {
		return nil, fmt.Errorf("invalid source path %q: %w", relPath, err)
	}
	destPath, err := platform.SafePath(absDestRoot, relPath)
	if err != nil {
		return nil, fmt.Errorf("invalid destination path %q: %w", relPath, err)
	}

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("stat source file: %w", err)
	}
	if !srcInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("source is not a regular file: %s", srcPath)
	}

	srcHash, err := integrity.HashFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("hashing source file: %w", err)
	}

	// Check if destination exists.
	_, destErr := os.Stat(destPath)
	if destErr == nil {
		// Destination exists. Check if content is already identical.
		destHash, err := integrity.HashFile(destPath)
		if err == nil && destHash == srcHash {
			// Already identical, nothing to do.
			return &TransferredFile{
				RelPath: relPath,
				Size:    srcInfo.Size(),
				Hash:    srcHash,
				ModTime: srcInfo.ModTime(),
			}, nil
		}

		// Content differs -> Conflict!
		switch opts.ConflictPolicy {
		case ConflictPolicyAbort:
			return nil, fmt.Errorf("conflict detected at %q: destination exists with different content (aborting)", relPath)
		case ConflictPolicyPreserve:
			if !opts.DryRun {
				// Preserve existing file by renaming.
				timestamp := time.Now().UTC().Format("20060102T150405Z")
				preservedPath := destPath + ".conflict." + timestamp
				if err := os.Rename(destPath, preservedPath); err != nil {
					return nil, fmt.Errorf("preserving conflicting file: %w", err)
				}
			}
		case ConflictPolicyOverwrite:
			// Overwrite allowed by caller.
		}
	} else if !os.IsNotExist(destErr) {
		return nil, fmt.Errorf("stat destination file: %w", destErr)
	}

	if opts.DryRun {
		return &TransferredFile{
			RelPath: relPath,
			Size:    srcInfo.Size(),
			Hash:    srcHash,
			ModTime: srcInfo.ModTime(),
		}, nil
	}

	// Ensure destination directory exists.
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return nil, fmt.Errorf("creating destination directory: %w", err)
	}

	// Transfer using safe temporary file in destination directory.
	tmpFile, err := os.CreateTemp(filepath.Dir(destPath), ".senvu-transfer-*")
	if err != nil {
		return nil, fmt.Errorf("creating temporary transfer file: %w", err)
	}
	tmpName := tmpFile.Name()
	cleanTmp := true
	defer func() {
		if cleanTmp {
			tmpFile.Close()
			os.Remove(tmpName)
		}
	}()

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("opening source file: %w", err)
	}
	defer srcFile.Close()

	buf := make([]byte, 64*1024)
	var written int64
	for {
		nr, rErr := srcFile.Read(buf)
		if nr > 0 {
			nw, wErr := tmpFile.Write(buf[:nr])
			if wErr != nil {
				return nil, fmt.Errorf("writing to temporary file: %w", wErr)
			}
			written += int64(nw)
			if opts.OnProgress != nil {
				opts.OnProgress(written, srcInfo.Size(), relPath)
			}
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			return nil, fmt.Errorf("reading source file: %w", rErr)
		}
	}

	if err := tmpFile.Sync(); err != nil {
		return nil, fmt.Errorf("syncing temporary file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("closing temporary file: %w", err)
	}

	// Verify written file hash BEFORE moving into place.
	writtenHash, err := integrity.HashFile(tmpName)
	if err != nil {
		return nil, fmt.Errorf("verifying written file hash: %w", err)
	}
	if writtenHash != srcHash {
		return nil, fmt.Errorf("integrity verification failed: source hash %s != written hash %s", srcHash, writtenHash)
	}

	// Preserve permissions.
	if opts.PreservePermissions {
		if err := os.Chmod(tmpName, srcInfo.Mode()); err != nil {
			// Non-fatal on some filesystems, but log/record.
		}
	}

	// Preserve timestamps.
	if opts.PreserveTimestamps {
		_ = os.Chtimes(tmpName, srcInfo.ModTime(), srcInfo.ModTime())
	}

	// Move into destination path.
	if err := os.Rename(tmpName, destPath); err != nil {
		// Fallback for cross-device moves if temp directory was on another mount.
		if isCrossDeviceError(err) {
			if copyErr := copyFile(tmpName, destPath, srcInfo.Mode()); copyErr != nil {
				return nil, fmt.Errorf("cross-device fallback copy: %w", copyErr)
			}
			os.Remove(tmpName)
		} else {
			return nil, fmt.Errorf("renaming temp file to destination: %w", err)
		}
	}

	cleanTmp = false // Successfully placed.

	// Re-apply timestamps to final destination after rename.
	if opts.PreserveTimestamps {
		_ = os.Chtimes(destPath, srcInfo.ModTime(), srcInfo.ModTime())
	}

	return &TransferredFile{
		RelPath: relPath,
		Size:    srcInfo.Size(),
		Hash:    srcHash,
		ModTime: srcInfo.ModTime(),
	}, nil
}

// TransferDirectory transfers all files from sourceRoot to destRoot.
func (e *Engine) TransferDirectory(sourceRoot, destRoot string, opts Options) (*Result, error) {
	absSource, err := filepath.Abs(sourceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving source directory: %w", err)
	}
	absDest, err := filepath.Abs(destRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving destination directory: %w", err)
	}

	srcInfo, err := os.Stat(absSource)
	if err != nil {
		return nil, fmt.Errorf("source directory error: %w", err)
	}
	if !srcInfo.IsDir() {
		return nil, fmt.Errorf("source is not a directory: %s", absSource)
	}

	result := &Result{
		SourceRoot:  absSource,
		DestRoot:    absDest,
		Transferred: make([]TransferredFile, 0),
		Skipped:     make([]SkippedFile, 0),
		Conflicts:   make([]ConflictFile, 0),
		Errors:      make([]TransferError, 0),
		DryRun:      opts.DryRun,
	}

	// Scan source directory.
	err = filepath.Walk(absSource, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			rel, _ := filepath.Rel(absSource, path)
			result.Errors = append(result.Errors, TransferError{RelPath: rel, Err: walkErr.Error()})
			return nil
		}

		if info.IsDir() {
			if info.Name() == ".senvu" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip symlinks for safety.
		if info.Mode()&os.ModeSymlink != 0 {
			rel, _ := filepath.Rel(absSource, path)
			result.Skipped = append(result.Skipped, SkippedFile{RelPath: rel, Reason: "symlinks skipped for safety"})
			return nil
		}

		relPath, err := filepath.Rel(absSource, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)

		// Check if destination exists with same hash before transferring.
		destFilePath := filepath.Join(absDest, filepath.FromSlash(relPath))
		if _, dErr := os.Stat(destFilePath); dErr == nil {
			srcHash, sErr := integrity.HashFile(path)
			dstHash, dstErr := integrity.HashFile(destFilePath)
			if sErr == nil && dstErr == nil && srcHash == dstHash {
				result.Skipped = append(result.Skipped, SkippedFile{
					RelPath: relPath,
					Reason:  "identical content already at destination",
				})
				return nil
			}

			if opts.ConflictPolicy == ConflictPolicyAbort {
				result.Conflicts = append(result.Conflicts, ConflictFile{
					RelPath:     relPath,
					SourceHash:  srcHash,
					DestHash:    dstHash,
					ActionTaken: "aborted (conflict detected)",
				})
				return nil
			} else if opts.ConflictPolicy == ConflictPolicyPreserve {
				result.Conflicts = append(result.Conflicts, ConflictFile{
					RelPath:     relPath,
					SourceHash:  srcHash,
					DestHash:    dstHash,
					ActionTaken: "preserved existing copy",
				})
			}
		}

		tf, err := e.TransferFile(absSource, absDest, relPath, opts)
		if err != nil {
			result.Errors = append(result.Errors, TransferError{
				RelPath: relPath,
				Err:     err.Error(),
			})
			return nil
		}

		result.Transferred = append(result.Transferred, *tf)
		result.TotalBytesTransferred += tf.Size
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking source directory: %w", err)
	}

	return result, nil
}

func isCrossDeviceError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "cross-device") || strings.Contains(s, "exdev")
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
