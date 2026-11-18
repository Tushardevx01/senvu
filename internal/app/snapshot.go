package app

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/senvu/senvu/internal/config"
	"github.com/senvu/senvu/internal/integrity"
	"github.com/senvu/senvu/internal/scanner"
	"github.com/senvu/senvu/internal/snapshot"
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

	var store storage.Store
	if config.WorkspaceExists(absRoot) {
		dbPath := getDBPath(absRoot)
		s, err := storage.Open(dbPath)
		if err != nil {
			return nil, fmt.Errorf("snapshot: opening storage: %w", err)
		}
		defer s.Close()
		store = s
	}

	svc := snapshot.NewService(ss.scanner, ss.hasher, store)
	return svc.Create(absRoot)
}

// ListSnapshots returns summaries of all snapshots stored for the given
// workspace path.
func (ss *SnapshotService) ListSnapshots(workspacePath string) ([]SnapshotSummary, error) {
	absRoot, err := filepath.Abs(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("snapshot: resolving workspace path: %w", err)
	}

	if !config.WorkspaceExists(absRoot) {
		return nil, fmt.Errorf("workspace not initialized at %s (run 'senvu init' first)", absRoot)
	}

	dbPath := getDBPath(absRoot)
	store, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot: opening storage: %w", err)
	}
	defer store.Close()

	svc := snapshot.NewService(ss.scanner, ss.hasher, store)
	summaries, err := svc.List()
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

	if !config.WorkspaceExists(absRoot) {
		return nil, fmt.Errorf("workspace not initialized at %s (run 'senvu init' first)", absRoot)
	}

	dbPath := getDBPath(absRoot)
	store, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot: opening storage: %w", err)
	}
	defer store.Close()

	svc := snapshot.NewService(ss.scanner, ss.hasher, store)
	return svc.Show(snapshotID)
}

// ExportSnapshot writes a snapshot manifest to the given output path as JSON.
func (ss *SnapshotService) ExportSnapshot(workspacePath, snapshotID, outputPath string) error {
	absRoot, err := filepath.Abs(workspacePath)
	if err != nil {
		return fmt.Errorf("snapshot: resolving workspace path: %w", err)
	}

	if !config.WorkspaceExists(absRoot) {
		return fmt.Errorf("workspace not initialized at %s (run 'senvu init' first)", absRoot)
	}

	dbPath := getDBPath(absRoot)
	store, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("snapshot: opening storage: %w", err)
	}
	defer store.Close()

	svc := snapshot.NewService(ss.scanner, ss.hasher, store)
	return svc.Export(snapshotID, outputPath)
}
