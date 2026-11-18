package app

import (
	"fmt"

	"github.com/senvu/senvu/internal/output"
	"github.com/senvu/senvu/internal/transfer"
)

// TransferOptions configures a transfer command invocation.
type TransferOptions struct {
	SourcePath string
	DestPath   string
	DryRun     bool
	Overwrite  bool
}

// Transfer performs a safe transfer between directories.
func Transfer(opts TransferOptions, out *output.Handler) (*transfer.Result, error) {
	policy := transfer.ConflictPolicyAbort
	if opts.Overwrite {
		policy = transfer.ConflictPolicyOverwrite
	}

	engine := transfer.NewEngine()
	transferOpts := transfer.Options{
		ConflictPolicy:      policy,
		DryRun:              opts.DryRun,
		PreserveTimestamps:  true,
		PreservePermissions: true,
		OnProgress: func(transferredBytes, totalBytes int64, currentFile string) {
			if out != nil && totalBytes > 0 {
				out.Log(output.LevelDebug, fmt.Sprintf("Transferring %s (%d/%d bytes)", currentFile, transferredBytes, totalBytes))
			}
		},
	}

	result, err := engine.TransferDirectory(opts.SourcePath, opts.DestPath, transferOpts)
	if err != nil {
		return nil, fmt.Errorf("transfer failed: %w", err)
	}

	return result, nil
}
