// Package config provides workspace configuration loading, saving, and validation
// for Senvu workspaces. Configuration is stored as JSON in .senvu/config.json.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// SenvuDir is the name of the hidden directory used by Senvu.
	SenvuDir = ".senvu"
	// ConfigFile is the name of the configuration file within the Senvu directory.
	ConfigFile = "config.json"
	// DBDir is the subdirectory used for the SQLite database.
	DBDir = "db"
)

// WorkspaceConfig holds the configuration for a Senvu workspace.
type WorkspaceConfig struct {
	// RootPath is the absolute path to the workspace root directory.
	RootPath string `json:"root_path"`
	// DBPath is the path to the SQLite database directory, relative to the Senvu dir.
	DBPath string `json:"db_path"`
	// CreatedAt is the timestamp when the workspace was initialized.
	CreatedAt time.Time `json:"created_at"`
	// SenvuVersion is the version of Senvu that created this workspace.
	SenvuVersion string `json:"senvu_version"`
}

// DefaultConfig returns a WorkspaceConfig with sensible defaults for the given root path.
func DefaultConfig(rootPath string, version string) WorkspaceConfig {
	return WorkspaceConfig{
		RootPath:     rootPath,
		DBPath:       DBDir,
		CreatedAt:    time.Now().UTC(),
		SenvuVersion: version,
	}
}

// SenvuDirPath returns the absolute path to the .senvu directory for the given workspace root.
func SenvuDirPath(rootPath string) string {
	return filepath.Join(rootPath, SenvuDir)
}

// ConfigFilePath returns the absolute path to the config.json file for the given workspace root.
func ConfigFilePath(rootPath string) string {
	return filepath.Join(rootPath, SenvuDir, ConfigFile)
}

// DBDirPath returns the absolute path to the database directory for the given workspace root.
func DBDirPath(rootPath string) string {
	return filepath.Join(rootPath, SenvuDir, DBDir)
}

// Load reads and parses the workspace configuration from .senvu/config.json
// in the given root path.
func Load(rootPath string) (*WorkspaceConfig, error) {
	cfgPath := ConfigFilePath(rootPath)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load config from %s: %w", cfgPath, err)
	}

	var cfg WorkspaceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", cfgPath, err)
	}

	return &cfg, nil
}

// Save writes the workspace configuration to .senvu/config.json in the workspace root.
// The parent directory must already exist.
func Save(cfg *WorkspaceConfig) error {
	cfgPath := ConfigFilePath(cfg.RootPath)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return fmt.Errorf("write config to %s: %w", cfgPath, err)
	}

	return nil
}

// WorkspaceExists checks whether a Senvu workspace has been initialized
// at the given root path by looking for the .senvu directory.
func WorkspaceExists(rootPath string) bool {
	info, err := os.Stat(SenvuDirPath(rootPath))
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ValidateWorkspace checks that the workspace root directory exists and is readable.
func ValidateWorkspace(rootPath string) error {
	info, err := os.Stat(rootPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("workspace root does not exist: %s", rootPath)
		}
		return fmt.Errorf("check workspace root %s: %w", rootPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace root is not a directory: %s", rootPath)
	}

	// Verify readability by attempting to list the directory.
	f, err := os.Open(rootPath)
	if err != nil {
		return fmt.Errorf("workspace root is not readable: %w", err)
	}
	f.Close()

	return nil
}
