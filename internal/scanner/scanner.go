// Package scanner provides recursive directory scanning with concurrent
// file hashing, symlink detection, and progress reporting.
package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/senvu/senvu/internal/integrity"
)

// ScanResult holds the metadata and hash for a single scanned file.
type ScanResult struct {
	// RelPath is the forward-slash-normalized path relative to the scan root.
	RelPath string
	// Size is the file size in bytes.
	Size int64
	// ModTime is the file's last modification time.
	ModTime time.Time
	// Hash is the hex-encoded BLAKE3 hash of the file content.
	Hash string
	// Mode is the file permission mode.
	Mode fs.FileMode
	// IsSymlink indicates whether the file is a symbolic link.
	IsSymlink bool
	// Error records any per-file error (e.g. permission denied).
	Error error
}

// ProgressFunc is called during scanning to report progress.
// processed is the number of files processed so far; path is the current file.
type ProgressFunc func(processed int, path string)

// Scanner recursively walks a directory tree, hashing each file with
// bounded concurrency.
type Scanner struct {
	hasher      integrity.Hasher
	concurrency int
	onProgress  ProgressFunc
}

// Option configures a Scanner.
type Option func(*Scanner)

// WithConcurrency sets the maximum number of concurrent hash workers.
func WithConcurrency(n int) Option {
	return func(s *Scanner) {
		if n > 0 {
			s.concurrency = n
		}
	}
}

// WithProgress sets a callback that is invoked for each processed file.
func WithProgress(fn ProgressFunc) Option {
	return func(s *Scanner) {
		s.onProgress = fn
	}
}

// New creates a new Scanner with the given options. If no concurrency is
// specified, runtime.NumCPU() is used.
func New(hasher integrity.Hasher, opts ...Option) *Scanner {
	s := &Scanner{
		hasher:      hasher,
		concurrency: runtime.NumCPU(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NewScanner creates a new Scanner using default Blake3 hasher.
func NewScanner(opts ...Option) *Scanner {
	return New(integrity.NewBlake3Hasher(), opts...)
}

// Scan recursively walks root, hashing every regular file with bounded
// concurrency. It skips the .senvu/ directory, records symlinks without
// following them, and captures per-file errors without aborting the scan.
func (s *Scanner) Scan(root string) ([]ScanResult, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving root path: %w", err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root is not a directory: %s", absRoot)
	}

	type workItem struct {
		absPath string
		relPath string
		info    fs.DirEntry
		linfo   os.FileInfo
	}

	var items []workItem

	// First pass: walk the directory tree to collect all items.
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Record permission errors etc. but continue walking.
			rel, _ := filepath.Rel(absRoot, path)
			rel = normalizePath(rel)
			items = append(items, workItem{
				absPath: path,
				relPath: rel,
				info:    d,
			})
			return nil
		}

		// Skip .senvu/ directory.
		if d.IsDir() && d.Name() == ".senvu" {
			return filepath.SkipDir
		}

		// Skip the root directory entry itself.
		if path == absRoot {
			return nil
		}

		// Skip directories (we only record files).
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return fmt.Errorf("computing relative path: %w", err)
		}
		rel = normalizePath(rel)

		// Use Lstat to detect symlinks.
		linfo, err := os.Lstat(path)
		if err != nil {
			items = append(items, workItem{absPath: path, relPath: rel, info: d})
			return nil
		}

		items = append(items, workItem{absPath: path, relPath: rel, info: d, linfo: linfo})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	// Second pass: hash files with bounded worker pool concurrency.
	results := make([]ScanResult, len(items))
	workerCount := s.concurrency
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	if workerCount > len(items) {
		workerCount = len(items)
	}

	type queueItem struct {
		idx  int
		item workItem
	}

	workCh := make(chan queueItem, workerCount*2)
	var wg sync.WaitGroup
	var mu sync.Mutex
	processed := 0

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for q := range workCh {
				idx := q.idx
				w := q.item

				result := ScanResult{
					RelPath: w.relPath,
				}

				// Handle items that had walk errors.
				if w.linfo == nil && w.info == nil {
					result.Error = fmt.Errorf("unable to stat file: %s", w.relPath)
					results[idx] = result
					s.reportProgress(&mu, &processed, w.relPath)
					continue
				}

				if w.linfo == nil {
					// Lstat failed; try to get what info we can.
					fi, err := os.Lstat(w.absPath)
					if err != nil {
						result.Error = fmt.Errorf("stat error: %w", err)
						results[idx] = result
						s.reportProgress(&mu, &processed, w.relPath)
						continue
					}
					w.linfo = fi
				}

				result.Size = w.linfo.Size()
				result.ModTime = w.linfo.ModTime()
				result.Mode = w.linfo.Mode()
				result.IsSymlink = w.linfo.Mode()&os.ModeSymlink != 0

				// Don't hash symlinks — record them but skip hashing.
				if result.IsSymlink {
					results[idx] = result
					s.reportProgress(&mu, &processed, w.relPath)
					continue
				}

				// Hash regular files.
				if w.linfo.Mode().IsRegular() {
					hash, err := integrity.HashFile(w.absPath)
					if err != nil {
						result.Error = fmt.Errorf("hashing file: %w", err)
					} else {
						// Detect file modifications/replacements during hashing.
						if postFi, postErr := os.Lstat(w.absPath); postErr == nil {
							if postFi.Size() != result.Size || !postFi.ModTime().Equal(result.ModTime) {
								result.Error = fmt.Errorf("file modified during hashing: %s", w.relPath)
							} else {
								result.Hash = hash
							}
						} else {
							result.Error = fmt.Errorf("file inaccessible after hashing: %w", postErr)
						}
					}
				}

				results[idx] = result
				s.reportProgress(&mu, &processed, w.relPath)
			}
		}()
	}

	for i, item := range items {
		workCh <- queueItem{idx: i, item: item}
	}
	close(workCh)
	wg.Wait()

	// Sort results by relative path for deterministic output.
	sort.Slice(results, func(i, j int) bool {
		return results[i].RelPath < results[j].RelPath
	})

	return results, nil
}

func (s *Scanner) reportProgress(mu *sync.Mutex, processed *int, path string) {
	if s.onProgress == nil {
		return
	}
	mu.Lock()
	*processed++
	count := *processed
	mu.Unlock()
	s.onProgress(count, path)
}

// normalizePath converts OS-specific path separators to forward slashes
// for portability.
func normalizePath(p string) string {
	return strings.ReplaceAll(p, string(filepath.Separator), "/")
}
