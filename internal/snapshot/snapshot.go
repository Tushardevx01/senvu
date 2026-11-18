// Package snapshot provides snapshot creation, inspection, listing,
// and export capabilities. Snapshots are immutable once created and
// represent verified point-in-time states of directory contents.
package snapshot

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/senvu/senvu/internal/integrity"
	"github.com/senvu/senvu/internal/platform"
	"github.com/senvu/senvu/internal/scanner"
	"github.com/senvu/senvu/internal/storage"
	"github.com/senvu/senvu/pkg/manifest"
)

// Service coordinates snapshot creation, storage, and retrieval.
type Service struct {
	scanner *scanner.Scanner
	hasher  integrity.Hasher
	store   storage.Store
}

// NewService creates a new snapshot Service.
func NewService(scanner *scanner.Scanner, hasher integrity.Hasher, store storage.Store) *Service {
	return &Service{
		scanner: scanner,
		hasher:  hasher,
		store:   store,
	}
}

// Create recursively scans rootPath, hashes all files, builds an immutable manifest,
// and persists it to the storage layer.
func (s *Service) Create(rootPath string) (*manifest.Manifest, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolving snapshot root: %w", err)
	}

	scanResults, err := s.scanner.Scan(absRoot)
	if err != nil {
		return nil, fmt.Errorf("scanning root %q: %w", absRoot, err)
	}

	var files []manifest.FileEntry
	var totalSize int64
	var scanErrors []string

	for _, sr := range scanResults {
		if sr.Error != nil {
			scanErrors = append(scanErrors, fmt.Sprintf("%s: %s", sr.RelPath, sr.Error))
			continue
		}
		if sr.IsSymlink {
			continue // symlinks are recorded but not hashed as file content
		}

		files = append(files, manifest.FileEntry{
			RelPath: sr.RelPath,
			Hash:    sr.Hash,
			Size:    sr.Size,
			ModTime: sr.ModTime,
			Mode:    sr.Mode,
		})
		totalSize += sr.Size
	}

	m := manifest.NewManifest(absRoot, s.hasher.Algorithm())
	m.SnapshotID = uuid.New().String()
	m.CreatedAt = time.Now().UTC()
	m.Files = files
	m.TotalFiles = len(files)
	m.TotalSize = totalSize

	if len(scanErrors) > 0 {
		return nil, fmt.Errorf("snapshot aborted: %d files encountered errors during scan (first error: %s)", len(scanErrors), scanErrors[0])
	}

	// Validate manifest before persisting.
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("validating manifest: %w", err)
	}

	// Persist to store if available.
	if s.store != nil {
		if err := s.store.SaveSnapshot(m); err != nil {
			return nil, fmt.Errorf("saving snapshot to database: %w", err)
		}
	}

	return m, nil
}

// List returns summaries of all snapshots stored.
func (s *Service) List() ([]storage.SnapshotSummary, error) {
	if s.store == nil {
		return nil, fmt.Errorf("storage store not configured")
	}
	return s.store.ListSnapshots()
}

// Show retrieves a snapshot by its ID.
func (s *Service) Show(snapshotID string) (*manifest.Manifest, error) {
	if s.store == nil {
		return nil, fmt.Errorf("storage store not configured")
	}
	return s.store.GetSnapshot(snapshotID)
}

// Export writes a snapshot manifest to an output file as JSON.
func (s *Service) Export(snapshotID string, outputPath string) error {
	m, err := s.Show(snapshotID)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	if err := platform.AtomicWrite(outputPath, data, 0644); err != nil {
		return fmt.Errorf("writing manifest export: %w", err)
	}

	return nil
}
