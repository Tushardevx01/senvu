// Package platform provides cross-platform filesystem abstractions for Senvu.
// It includes safe path validation, atomic file writes, symlink detection,
// and directory management utilities.
package platform

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SafePath resolves and validates that the given path is within the specified
// root directory. It prevents path traversal attacks by resolving symlinks
// and ensuring the canonical path starts with the canonical root.
func SafePath(root, path string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root path %q: %w", root, err)
	}

	// Resolve the root to its real path (following symlinks).
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("eval symlinks for root %q: %w", absRoot, err)
	}

	absPath := filepath.Clean(filepath.Join(absRoot, path))

	// Lexical check first: path must be inside absRoot.
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("path %q is outside root %q", path, root)
	}

	// Walk up to find the closest existing ancestor directory.
	curr := absPath
	var suffixParts []string
	for {
		fi, err := os.Lstat(curr)
		if err == nil {
			// Found an existing node. If it's a symlink, eval it.
			realCurr, evalErr := filepath.EvalSymlinks(curr)
			if evalErr != nil {
				return "", fmt.Errorf("eval symlinks for %q: %w", curr, evalErr)
			}

			// Ensure realCurr is within realRoot.
			normRealRoot := normalizePathForComparison(realRoot)
			normRealCurr := normalizePathForComparison(realCurr)
			relReal, relErr := filepath.Rel(normRealRoot, normRealCurr)
			if relErr != nil || strings.HasPrefix(relReal, "..") || (relReal == ".." && normRealCurr != normRealRoot) {
				return "", fmt.Errorf("path %q resolves outside root %q", path, root)
			}

			// If it's a file but we still have suffix parts, that's invalid.
			if !fi.IsDir() && len(suffixParts) > 0 {
				return "", fmt.Errorf("path %q attempts to traverse through non-directory %q", path, curr)
			}

			// Reconstruct realPath.
			parts := append([]string{realCurr}, reverseSlice(suffixParts)...)
			realPath := filepath.Join(parts...)
			return realPath, nil
		}

		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect path %q: %w", curr, err)
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		suffixParts = append(suffixParts, filepath.Base(curr))
		curr = parent
	}

	return absPath, nil
}

func reverseSlice(s []string) []string {
	res := make([]string, len(s))
	for i, v := range s {
		res[len(s)-1-i] = v
	}
	return res
}

func normalizePathForComparison(p string) string {
	if strings.HasPrefix(p, `\\?\UNC\`) {
		p = `\\` + strings.TrimPrefix(p, `\\?\UNC\`)
	}
	p = strings.TrimPrefix(p, `\\?\`)
	if len(p) >= 2 && p[1] == ':' {
		p = strings.ToUpper(string(p[0])) + p[1:]
	}
	return filepath.Clean(p)
}

// AtomicWrite writes data to a file atomically by writing to a temporary
// file in the same directory and then renaming it to the target path.
// This prevents partial writes and data corruption.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".senvu-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()

	// Clean up on failure.
	success := false
	defer func() {
		if !success {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file %q: %w", tmpName, err)
	}

	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("chmod temp file %q: %w", tmpName, err)
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file %q: %w", tmpName, err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file %q: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		// On Windows or certain filesystems, remove target and retry if rename fails.
		_ = os.Remove(path)
		if retryErr := os.Rename(tmpName, path); retryErr != nil {
			return fmt.Errorf("rename %q to %q: %w", tmpName, path, err)
		}
	}

	success = true
	return nil
}

// IsSymlink reports whether the given path is a symbolic link.
func IsSymlink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, fmt.Errorf("lstat %q: %w", path, err)
	}
	return info.Mode()&os.ModeSymlink != 0, nil
}

// EnsureDir creates the specified directory and all necessary parents
// with the given permissions. It is safe to call on an existing directory.
func EnsureDir(path string, perm os.FileMode) error {
	if err := os.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("create directory %q: %w", path, err)
	}
	return nil
}

// FileInfo wraps os.FileInfo with additional convenience fields.
type FileInfo struct {
	// Path is the absolute path to the file.
	Path string
	// RelPath is the path relative to the workspace root, using forward slashes.
	RelPath string
	// Size is the file size in bytes.
	Size int64
	// Mode is the file permission mode.
	Mode fs.FileMode
	// IsDir indicates whether the entry is a directory.
	IsDir bool
	// IsLink indicates whether the entry is a symbolic link.
	IsLink bool
}

// NewFileInfo creates a FileInfo by statting the given path and computing
// its path relative to root.
func NewFileInfo(root, path string) (*FileInfo, error) {
	linfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("lstat %q: %w", path, err)
	}

	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return nil, fmt.Errorf("compute relative path of %q from %q: %w", path, root, err)
	}

	return &FileInfo{
		Path:    path,
		RelPath: NormalizePath(relPath),
		Size:    linfo.Size(),
		Mode:    linfo.Mode(),
		IsDir:   linfo.IsDir(),
		IsLink:  linfo.Mode()&os.ModeSymlink != 0,
	}, nil
}

// NormalizePath converts a filesystem path to use forward slashes,
// suitable for storage in manifests and cross-platform comparisons.
func NormalizePath(path string) string {
	return filepath.ToSlash(path)
}

// ValidateDestination ensures that the target path resolves to a location
// within the allowed root directory. It returns the cleaned absolute path
// or an error if the destination escapes the root.
func ValidateDestination(root, target string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root %q: %w", root, err)
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve target %q: %w", target, err)
	}

	// Clean both paths.
	cleanRoot := filepath.Clean(absRoot)
	cleanTarget := filepath.Clean(absTarget)

	if !strings.HasPrefix(cleanTarget, cleanRoot+string(filepath.Separator)) && cleanTarget != cleanRoot {
		return "", fmt.Errorf("target %q is outside allowed root %q", target, root)
	}

	return cleanTarget, nil
}
