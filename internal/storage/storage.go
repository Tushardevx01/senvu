// Package storage provides a SQLite-backed persistence layer for snapshots
// and workspace metadata. It uses modernc.org/sqlite for a pure-Go, CGO-free
// implementation with transaction support and schema migration.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/senvu/senvu/pkg/manifest"

	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 1

// SnapshotSummary provides a brief overview of a stored snapshot.
type SnapshotSummary struct {
	// SnapshotID is the unique identifier of the snapshot.
	SnapshotID string
	// CreatedAt is the time the snapshot was created.
	CreatedAt time.Time
	// SourcePath is the relative path that was scanned.
	SourcePath string
	// TotalFiles is the number of files in the snapshot.
	TotalFiles int
	// TotalSize is the total size of all files in bytes.
	TotalSize int64
}

// Store defines the interface for snapshot persistence operations.
type Store interface {
	// Close closes the underlying database connection.
	Close() error
	// SaveSnapshot persists a manifest as a new snapshot.
	SaveSnapshot(m *manifest.Manifest) error
	// ListSnapshots returns summaries of all stored snapshots.
	ListSnapshots() ([]SnapshotSummary, error)
	// GetSnapshot retrieves a full manifest by snapshot ID.
	GetSnapshot(id string) (*manifest.Manifest, error)
	// GetSnapshotFiles returns the file entries for a snapshot.
	GetSnapshotFiles(id string) ([]manifest.FileEntry, error)
}

// SQLiteStore implements Store using a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// Open opens or creates a SQLite database at dbPath, runs schema migrations,
// and returns a ready-to-use SQLiteStore.
func Open(dbPath string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrency.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting journal mode: %w", err)
	}

	// Enable foreign keys.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema migration: %w", err)
	}

	return store, nil
}

// NewStore is an alias for Open.
func NewStore(dbPath string) (*SQLiteStore, error) {
	return Open(dbPath)
}

// migrate applies schema migrations up to currentSchemaVersion.
func (s *SQLiteStore) migrate() error {
	// Create migration tracking table.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	var version int
	err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	if version < 1 {
		if err := s.migrateV1(); err != nil {
			return fmt.Errorf("migration v1: %w", err)
		}
	}

	return nil
}

// migrateV1 creates the initial schema.
func (s *SQLiteStore) migrateV1() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS workspaces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			root_path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			senvu_version TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS snapshots (
			snapshot_id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			senvu_version TEXT NOT NULL,
			created_at TEXT NOT NULL,
			source_path TEXT NOT NULL,
			hash_algorithm TEXT NOT NULL,
			total_size INTEGER NOT NULL,
			total_files INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS snapshot_files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id TEXT NOT NULL,
			rel_path TEXT NOT NULL,
			size INTEGER NOT NULL,
			mod_time TEXT NOT NULL,
			hash TEXT NOT NULL,
			mode INTEGER NOT NULL,
			FOREIGN KEY (snapshot_id) REFERENCES snapshots(snapshot_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_snapshot_files_snapshot_id 
			ON snapshot_files(snapshot_id)`,
		`CREATE TABLE IF NOT EXISTS operations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			operation_type TEXT NOT NULL,
			snapshot_id TEXT,
			started_at TEXT NOT NULL,
			completed_at TEXT,
			status TEXT NOT NULL,
			details TEXT,
			FOREIGN KEY (snapshot_id) REFERENCES snapshots(snapshot_id)
		)`,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?)`,
	}

	for i, stmt := range statements {
		if i == len(statements)-1 {
			// The INSERT statement needs a parameter.
			if _, err := tx.Exec(stmt, time.Now().UTC().Format(time.RFC3339)); err != nil {
				return fmt.Errorf("executing migration statement %d: %w", i, err)
			}
		} else {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("executing migration statement %d: %w", i, err)
			}
		}
	}

	return tx.Commit()
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// SaveSnapshot persists the given manifest as a snapshot in the database.
// The entire operation is wrapped in a transaction.
func (s *SQLiteStore) SaveSnapshot(m *manifest.Manifest) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO snapshots (snapshot_id, schema_version, senvu_version, created_at,
			source_path, hash_algorithm, total_size, total_files)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.SnapshotID,
		m.SchemaVersion,
		m.SenvuVersion,
		m.CreatedAt.UTC().Format(time.RFC3339Nano),
		m.SourcePath,
		m.HashAlgorithm,
		m.TotalSize,
		m.TotalFiles,
	)
	if err != nil {
		return fmt.Errorf("inserting snapshot: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO snapshot_files (snapshot_id, rel_path, size, mod_time, hash, mode)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing file insert: %w", err)
	}
	defer stmt.Close()

	for i, f := range m.Files {
		_, err := stmt.Exec(
			m.SnapshotID,
			f.RelPath,
			f.Size,
			f.ModTime.UTC().Format(time.RFC3339Nano),
			f.Hash,
			uint32(f.Mode),
		)
		if err != nil {
			return fmt.Errorf("inserting file %d (%q): %w", i, f.RelPath, err)
		}
	}

	return tx.Commit()
}

// SaveManifest is an alias for SaveSnapshot.
func (s *SQLiteStore) SaveManifest(m *manifest.Manifest) error {
	return s.SaveSnapshot(m)
}

// LoadManifest is an alias for GetSnapshot.
func (s *SQLiteStore) LoadManifest(id string) (*manifest.Manifest, error) {
	return s.GetSnapshot(id)
}

// ListManifests is an alias for ListSnapshots.
func (s *SQLiteStore) ListManifests() ([]SnapshotSummary, error) {
	return s.ListSnapshots()
}

// ListSnapshots returns summaries of all stored snapshots, ordered by
// creation time (newest first).
func (s *SQLiteStore) ListSnapshots() ([]SnapshotSummary, error) {
	rows, err := s.db.Query(`
		SELECT snapshot_id, created_at, source_path, total_files, total_size
		FROM snapshots
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("querying snapshots: %w", err)
	}
	defer rows.Close()

	var summaries []SnapshotSummary
	for rows.Next() {
		var ss SnapshotSummary
		var createdStr string
		if err := rows.Scan(&ss.SnapshotID, &createdStr, &ss.SourcePath, &ss.TotalFiles, &ss.TotalSize); err != nil {
			return nil, fmt.Errorf("scanning snapshot row: %w", err)
		}
		ss.CreatedAt, err = time.Parse(time.RFC3339Nano, createdStr)
		if err != nil {
			return nil, fmt.Errorf("parsing created_at: %w", err)
		}
		summaries = append(summaries, ss)
	}

	return summaries, rows.Err()
}

// GetSnapshot retrieves a full manifest by snapshot ID, including all files.
func (s *SQLiteStore) GetSnapshot(id string) (*manifest.Manifest, error) {
	var m manifest.Manifest
	var createdStr string

	err := s.db.QueryRow(`
		SELECT snapshot_id, schema_version, senvu_version, created_at,
			source_path, hash_algorithm, total_size, total_files
		FROM snapshots
		WHERE snapshot_id = ?`, id).Scan(
		&m.SnapshotID,
		&m.SchemaVersion,
		&m.SenvuVersion,
		&createdStr,
		&m.SourcePath,
		&m.HashAlgorithm,
		&m.TotalSize,
		&m.TotalFiles,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("snapshot not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("querying snapshot: %w", err)
	}

	m.CreatedAt, err = time.Parse(time.RFC3339Nano, createdStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}

	files, err := s.GetSnapshotFiles(id)
	if err != nil {
		return nil, fmt.Errorf("loading snapshot files: %w", err)
	}
	m.Files = files

	return &m, nil
}

// GetSnapshotFiles returns all file entries for a given snapshot.
func (s *SQLiteStore) GetSnapshotFiles(id string) ([]manifest.FileEntry, error) {
	rows, err := s.db.Query(`
		SELECT rel_path, size, mod_time, hash, mode
		FROM snapshot_files
		WHERE snapshot_id = ?
		ORDER BY rel_path`, id)
	if err != nil {
		return nil, fmt.Errorf("querying snapshot files: %w", err)
	}
	defer rows.Close()

	var files []manifest.FileEntry
	for rows.Next() {
		var f manifest.FileEntry
		var modStr string
		var mode uint32
		if err := rows.Scan(&f.RelPath, &f.Size, &modStr, &f.Hash, &mode); err != nil {
			return nil, fmt.Errorf("scanning file row: %w", err)
		}
		f.Mode = os.FileMode(mode)
		f.ModTime, err = time.Parse(time.RFC3339Nano, modStr)
		if err != nil {
			return nil, fmt.Errorf("parsing mod_time: %w", err)
		}
		files = append(files, f)
	}

	return files, rows.Err()
}

// SchemaVersion returns the current applied schema version.
func (s *SQLiteStore) SchemaVersion() (int, error) {
	var version int
	err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("reading schema version: %w", err)
	}
	return version, nil
}
