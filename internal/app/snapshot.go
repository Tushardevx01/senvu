package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/senvu/senvu/internal/config"
	"github.com/senvu/senvu/internal/integrity"
	"github.com/senvu/senvu/internal/platform"
	"github.com/senvu/senvu/internal/scanner"
	"github.com/senvu/senvu/internal/storage"
	"github.com/senvu/senvu/pkg/manifest"
)

// SnapshotSummary provides a brief overview of a stored snapshot.
type SnapshotSummary struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	FileCount  int       `json:"file_count"`
	TotalSize  int64     `json:"total_size"`
	SourcePath string    `json:"source_path"`
}

// SnapshotService orchestrates snapshot creation, listing, retrieval, and
// export by combining the scanner, hasher, and storage layers.
type SnapshotService struct {
	scanner *scanner.Scanner
	hasher  *integrity.Blake3Hasher
}

// NewSnapshotService creates a new SnapshotService.
func NewSnapshotService() *SnapshotService {
	return &SnapshotService{
		scanner: scanner.NewScanner(),
		hasher:  integrity.NewBlake3Hasher(),
	}
}

// getDBPath returns the sqlite database file path for a workspace.
func getDBPath(workspacePath string) string {
	return filepath.Join(config.DBDirPath(workspacePath), "senvu.db")
}

// CreateSnapshot scans the given root path and creates a new manifest snapshot.
func (ss *SnapshotService) CreateSnapshot(rootPath string) (*manifest.Manifest, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot: resolving root path: %w", err)
	}

	results, err := ss.scanner.Scan(absRoot)
	if err != nil {
		return nil, fmt.Errorf("snapshot: scanning directory: %w", err)
	}

	var files []manifest.FileEntry
	var scanErrors []string
	var totalSize int64

	for _, sr := range results {
		if sr.Error != nil {
			scanErrors = append(scanErrors, fmt.Sprintf("%s: %s", sr.RelPath, sr.Error))
			continue
		}
		if sr.IsSymlink {
			continue
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

	m := manifest.NewManifest(absRoot, ss.hasher.Algorithm())
	m.SnapshotID = uuid.New().String()
	m.Files = files
	m.TotalFiles = len(files)
	m.TotalSize = totalSize

	if len(scanErrors) > 0 {
		m.Metadata["scan_errors"] = strings.Join(scanErrors, "; ")
	}

	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("snapshot: validating manifest: %w", err)
	}

	// Store the snapshot if in an initialized workspace.
	if config.WorkspaceExists(absRoot) {
		dbPath := getDBPath(absRoot)
		store, err := storage.Open(dbPath)
		if err != nil {
			return nil, fmt.Errorf("snapshot: opening storage: %w", err)
		}
		defer store.Close()

		if err := store.SaveSnapshot(m); err != nil {
			return nil, fmt.Errorf("snapshot: saving manifest: %w", err)
		}
	}

	return m, nil
}

// ListSnapshots returns summaries of all snapshots stored for the given
// workspace path.
func (ss *SnapshotService) ListSnapshots(workspacePath string) ([]SnapshotSummary, error) {
	absRoot, err := filepath.Abs(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("snapshot: resolving workspace path: %w", err)
	}

	dbPath := getDBPath(absRoot)
	store, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot: opening storage: %w", err)
	}
	defer store.Close()

	summaries, err := store.ListSnapshots()
	if err != nil {
		return nil, fmt.Errorf("snapshot: listing manifests: %w", err)
	}

	results := make([]SnapshotSummary, 0, len(summaries))
	for _, s := range summaries {
		results = append(results, SnapshotSummary{
			ID:         s.SnapshotID,
			CreatedAt:  s.CreatedAt,
			FileCount:  s.TotalFiles,
			TotalSize:  s.TotalSize,
			SourcePath: s.SourcePath,
		})
	}

	return results, nil
}

// ShowSnapshot retrieves a specific snapshot by its ID.
func (ss *SnapshotService) ShowSnapshot(workspacePath, snapshotID string) (*manifest.Manifest, error) {
	absRoot, err := filepath.Abs(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("snapshot: resolving workspace path: %w", err)
	}

	dbPath := getDBPath(absRoot)
	store, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot: opening storage: %w", err)
	}
	defer store.Close()

	m, err := store.GetSnapshot(snapshotID)
	if err != nil {
		return nil, fmt.Errorf("snapshot: loading snapshot %q: %w", snapshotID, err)
	}

	return m, nil
}

// ExportSnapshot writes a snapshot manifest to the given output path as JSON.
func (ss *SnapshotService) ExportSnapshot(workspacePath, snapshotID, outputPath string) error {
	m, err := ss.ShowSnapshot(workspacePath, snapshotID)
	if err != nil {
		return fmt.Errorf("snapshot: export: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("snapshot: marshalling manifest: %w", err)
	}

	if err := platform.AtomicWrite(outputPath, data, 0644); err != nil {
		return fmt.Errorf("snapshot: writing export file: %w", err)
	}

	return nil
}
