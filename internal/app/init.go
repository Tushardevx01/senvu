// Package app implements the core application logic for Senvu commands.
package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/senvu/senvu/internal/config"
	"github.com/senvu/senvu/internal/output"
	"github.com/senvu/senvu/internal/platform"
)

// InitOptions configures the behavior of workspace initialization.
type InitOptions struct {
	// TargetDir is the directory to initialize as a Senvu workspace.
	TargetDir string
	// Force allows re-initialization of an existing workspace.
	Force bool
	// Version is the Senvu version string to record in the config.
	Version string
}

// Init initializes a new Senvu workspace in the specified directory.
// It creates the .senvu/ directory structure, writes the default
// configuration, and reports what was created.
func Init(opts InitOptions, out *output.Handler) error {
	// Resolve to absolute path.
	targetDir := opts.TargetDir
	if targetDir == "" {
		var err error
		targetDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
	}

	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("resolve target directory: %w", err)
	}
	targetDir = absTarget

	// Validate target directory exists and is readable.
	if err := config.ValidateWorkspace(targetDir); err != nil {
		return fmt.Errorf("validate target directory: %w", err)
	}

	// Check for existing workspace.
	senvuDir := config.SenvuDirPath(targetDir)
	if config.WorkspaceExists(targetDir) {
		if !opts.Force {
			return fmt.Errorf("workspace already initialized at %s (use --force to reinitialize)", targetDir)
		}
		if out != nil {
			out.Log(output.LevelWarn, "Reinitializing existing workspace (--force)")
		}
		if err := os.RemoveAll(senvuDir); err != nil {
			return fmt.Errorf("remove existing .senvu directory: %w", err)
		}
	}

	if out != nil {
		out.Log(output.LevelDebug, fmt.Sprintf("Initializing workspace at %s", targetDir))
	}

	// Create .senvu/ directory.
	if err := platform.EnsureDir(senvuDir, 0755); err != nil {
		return fmt.Errorf("create .senvu directory: %w", err)
	}
	if out != nil {
		out.Log(output.LevelDebug, fmt.Sprintf("Created %s", senvuDir))
	}

	// Create .senvu/db/ directory.
	dbDir := config.DBDirPath(targetDir)
	if err := platform.EnsureDir(dbDir, 0755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	if out != nil {
		out.Log(output.LevelDebug, fmt.Sprintf("Created %s", dbDir))
	}

	// Create and save default config.
	cfg := config.DefaultConfig(targetDir, opts.Version)
	if err := config.Save(&cfg); err != nil {
		return fmt.Errorf("save workspace config: %w", err)
	}
	if out != nil {
		out.Log(output.LevelDebug, fmt.Sprintf("Created %s", config.ConfigFilePath(targetDir)))

		// Report what was created.
		out.Success(fmt.Sprintf("Initialized Senvu workspace at %s", targetDir))
		out.Printf("  Created %s/", config.SenvuDir)
		out.Printf("  Created %s/%s/", config.SenvuDir, config.DBDir)
		out.Printf("  Created %s/%s", config.SenvuDir, config.ConfigFile)
	}

	return nil
}
