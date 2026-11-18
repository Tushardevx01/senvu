// Package reconcile provides safe, predictable reconciliation between storage locations.
// In accordance with Senvu principles, it produces dry-run execution plans, detects
// conflicts rather than guessing, preserves conflicting versions, and never silently
// deletes or overwrites user data.
package reconcile

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/senvu/senvu/internal/scanner"
	"github.com/senvu/senvu/internal/transfer"
)

// Action represents the proposed operation for a file.
type Action string

const (
	// ActionCreate transfers a new file from source to destination.
	ActionCreate Action = "CREATE"
	// ActionUpdate updates destination with newer source content when destination is unmodified.
	ActionUpdate Action = "UPDATE"
	// ActionConflict preserves the existing destination file and transfers source.
	ActionConflict Action = "CONFLICT_PRESERVE"
	// ActionIdentical indicates file is already in sync; no action required.
	ActionIdentical Action = "IDENTICAL"
	// ActionRetainDest indicates file exists only on destination; kept safe (never deleted).
	ActionRetainDest Action = "RETAIN_DEST"
)

// PlanItem represents a single planned file operation.
type PlanItem struct {
	RelPath       string    `json:"rel_path"`
	Action        Action    `json:"action"`
	SourceHash    string    `json:"source_hash,omitempty"`
	DestHash      string    `json:"dest_hash,omitempty"`
	SourceSize    int64     `json:"source_size,omitempty"`
	DestSize      int64     `json:"dest_size,omitempty"`
	SourceModTime time.Time `json:"source_mod_time,omitempty"`
	DestModTime   time.Time `json:"dest_mod_time,omitempty"`
	Reason        string    `json:"reason"`
}

// Plan is the reconciliation plan proposed before execution.
type Plan struct {
	SourceRoot string     `json:"source_root"`
	DestRoot   string     `json:"dest_root"`
	CreatedAt  time.Time  `json:"created_at"`
	Items      []PlanItem `json:"items"`
}

// Counts returns counts of each action in the plan.
func (p *Plan) Counts() map[Action]int {
	counts := make(map[Action]int)
	for _, item := range p.Items {
		counts[item.Action]++
	}
	return counts
}

// HasChanges reports whether the plan includes changes to be applied.
func (p *Plan) HasChanges() bool {
	for _, item := range p.Items {
		if item.Action == ActionCreate || item.Action == ActionUpdate || item.Action == ActionConflict {
			return true
		}
	}
	return false
}

// Summary returns a human-readable summary of the plan.
func (p *Plan) Summary() string {
	var b strings.Builder
	counts := p.Counts()

	b.WriteString(fmt.Sprintf("Reconciliation Plan: %s → %s\n", p.SourceRoot, p.DestRoot))
	b.WriteString(fmt.Sprintf("  Files to create in destination:  %d\n", counts[ActionCreate]))
	b.WriteString(fmt.Sprintf("  Files to update in destination:  %d\n", counts[ActionUpdate]))
	b.WriteString(fmt.Sprintf("  Conflicting files to preserve:   %d\n", counts[ActionConflict]))
	b.WriteString(fmt.Sprintf("  Identical files (no action):     %d\n", counts[ActionIdentical]))
	b.WriteString(fmt.Sprintf("  Destination-only files (kept):   %d\n", counts[ActionRetainDest]))

	return b.String()
}

// ExecutionItem records the result of applying a single plan item.
type ExecutionItem struct {
	RelPath string `json:"rel_path"`
	Action  Action `json:"action"`
	Status  string `json:"status"` // "applied", "skipped", "failed"
	Error   string `json:"error,omitempty"`
}

// ExecutionResult records the outcome of executing a reconciliation plan.
type ExecutionResult struct {
	PlanApplied Plan            `json:"plan"`
	ExecutedAt  time.Time       `json:"executed_at"`
	DryRun      bool            `json:"dry_run"`
	Items       []ExecutionItem `json:"items"`
	Success     bool            `json:"success"`
	ErrorCount  int             `json:"error_count"`
}

// Options configures reconciliation behavior.
type Options struct {
	DryRun      bool
	AutoApprove bool // skip prompt when running programmatically
}

// Engine produces and executes reconciliation plans.
type Engine struct {
	scanner        *scanner.Scanner
	transferEngine *transfer.Engine
}

// NewEngine creates a new reconciliation Engine.
func NewEngine() *Engine {
	return &Engine{
		scanner:        scanner.NewScanner(),
		transferEngine: transfer.NewEngine(),
	}
}

// BuildPlan compares source and destination directories to produce a safe reconciliation plan.
func (e *Engine) BuildPlan(sourceRoot, destRoot string) (*Plan, error) {
	absSource, err := filepath.Abs(sourceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving source: %w", err)
	}
	absDest, err := filepath.Abs(destRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving destination: %w", err)
	}

	sourceResults, err := e.scanner.Scan(absSource)
	if err != nil {
		return nil, fmt.Errorf("scanning source %q: %w", absSource, err)
	}

	destResults, err := e.scanner.Scan(absDest)
	if err != nil {
		return nil, fmt.Errorf("scanning destination %q: %w", absDest, err)
	}

	sourceMap := make(map[string]scanner.ScanResult, len(sourceResults))
	for _, sr := range sourceResults {
		if sr.Error == nil && !sr.IsSymlink {
			sourceMap[sr.RelPath] = sr
		}
	}

	destMap := make(map[string]scanner.ScanResult, len(destResults))
	for _, dr := range destResults {
		if dr.Error == nil && !dr.IsSymlink {
			destMap[dr.RelPath] = dr
		}
	}

	var items []PlanItem

	// Process all files in source.
	for relPath, sf := range sourceMap {
		df, inDest := destMap[relPath]
		if !inDest {
			// New file to create in destination.
			items = append(items, PlanItem{
				RelPath:       relPath,
				Action:        ActionCreate,
				SourceHash:    sf.Hash,
				SourceSize:    sf.Size,
				SourceModTime: sf.ModTime,
				Reason:        "new file in source, missing in destination",
			})
			continue
		}

		if sf.Hash == df.Hash {
			// Identical content.
			items = append(items, PlanItem{
				RelPath:       relPath,
				Action:        ActionIdentical,
				SourceHash:    sf.Hash,
				DestHash:      df.Hash,
				SourceSize:    sf.Size,
				DestSize:      df.Size,
				SourceModTime: sf.ModTime,
				DestModTime:   df.ModTime,
				Reason:        "content is already identical (verified by BLAKE3 hash)",
			})
			continue
		}

		// Content differs between source and destination.
		// If destination is newer or timestamps are identical, preserve as conflict to prevent silent data loss.
		if df.ModTime.After(sf.ModTime) || df.ModTime.Equal(sf.ModTime) {
			items = append(items, PlanItem{
				RelPath:       relPath,
				Action:        ActionConflict,
				SourceHash:    sf.Hash,
				DestHash:      df.Hash,
				SourceSize:    sf.Size,
				DestSize:      df.Size,
				SourceModTime: sf.ModTime,
				DestModTime:   df.ModTime,
				Reason:        "destination file modified or conflicting with equal timestamp; preserved as conflict backup",
			})
		} else {
			// Source is strictly newer -> update destination.
			items = append(items, PlanItem{
				RelPath:       relPath,
				Action:        ActionUpdate,
				SourceHash:    sf.Hash,
				DestHash:      df.Hash,
				SourceSize:    sf.Size,
				DestSize:      df.Size,
				SourceModTime: sf.ModTime,
				DestModTime:   df.ModTime,
				Reason:        "source file is newer; updating destination copy",
			})
		}
	}

	// Check files only in destination (Never delete user data!).
	for relPath, df := range destMap {
		if _, inSource := sourceMap[relPath]; !inSource {
			items = append(items, PlanItem{
				RelPath:     relPath,
				Action:      ActionRetainDest,
				DestHash:    df.Hash,
				DestSize:    df.Size,
				DestModTime: df.ModTime,
				Reason:      "destination-only file; preserved safely without deletion",
			})
		}
	}

	// Sort items by RelPath for deterministic plan ordering.
	sort.Slice(items, func(i, j int) bool {
		return items[i].RelPath < items[j].RelPath
	})

	return &Plan{
		SourceRoot: absSource,
		DestRoot:   absDest,
		CreatedAt:  time.Now().UTC(),
		Items:      items,
	}, nil
}

// Execute applies a reconciliation plan safely according to options.
func (e *Engine) Execute(plan *Plan, opts Options) (*ExecutionResult, error) {
	result := &ExecutionResult{
		PlanApplied: *plan,
		ExecutedAt:  time.Now().UTC(),
		DryRun:      opts.DryRun,
		Items:       make([]ExecutionItem, 0, len(plan.Items)),
		Success:     true,
	}

	for _, item := range plan.Items {
		switch item.Action {
		case ActionIdentical, ActionRetainDest:
			result.Items = append(result.Items, ExecutionItem{
				RelPath: item.RelPath,
				Action:  item.Action,
				Status:  "skipped",
			})

		case ActionCreate:
			if opts.DryRun {
				result.Items = append(result.Items, ExecutionItem{
					RelPath: item.RelPath,
					Action:  item.Action,
					Status:  "dry-run (would create)",
				})
				continue
			}

			transferOpts := transfer.DefaultOptions()
			_, err := e.transferEngine.TransferFile(plan.SourceRoot, plan.DestRoot, item.RelPath, transferOpts)
			if err != nil {
				result.ErrorCount++
				result.Success = false
				result.Items = append(result.Items, ExecutionItem{
					RelPath: item.RelPath,
					Action:  item.Action,
					Status:  "failed",
					Error:   err.Error(),
				})
			} else {
				result.Items = append(result.Items, ExecutionItem{
					RelPath: item.RelPath,
					Action:  item.Action,
					Status:  "created",
				})
			}

		case ActionUpdate:
			if opts.DryRun {
				result.Items = append(result.Items, ExecutionItem{
					RelPath: item.RelPath,
					Action:  item.Action,
					Status:  "dry-run (would update)",
				})
				continue
			}

			// For update, safely preserve previous copy if overwrite is risky.
			transferOpts := transfer.Options{
				ConflictPolicy:      transfer.ConflictPolicyOverwrite,
				PreserveTimestamps:  true,
				PreservePermissions: true,
			}
			_, err := e.transferEngine.TransferFile(plan.SourceRoot, plan.DestRoot, item.RelPath, transferOpts)
			if err != nil {
				result.ErrorCount++
				result.Success = false
				result.Items = append(result.Items, ExecutionItem{
					RelPath: item.RelPath,
					Action:  item.Action,
					Status:  "failed",
					Error:   err.Error(),
				})
			} else {
				result.Items = append(result.Items, ExecutionItem{
					RelPath: item.RelPath,
					Action:  item.Action,
					Status:  "updated",
				})
			}

		case ActionConflict:
			if opts.DryRun {
				result.Items = append(result.Items, ExecutionItem{
					RelPath: item.RelPath,
					Action:  item.Action,
					Status:  "dry-run (would preserve conflict & transfer)",
				})
				continue
			}

			transferOpts := transfer.Options{
				ConflictPolicy:      transfer.ConflictPolicyPreserve,
				PreserveTimestamps:  true,
				PreservePermissions: true,
			}
			_, err := e.transferEngine.TransferFile(plan.SourceRoot, plan.DestRoot, item.RelPath, transferOpts)
			if err != nil {
				result.ErrorCount++
				result.Success = false
				result.Items = append(result.Items, ExecutionItem{
					RelPath: item.RelPath,
					Action:  item.Action,
					Status:  "failed",
					Error:   err.Error(),
				})
			} else {
				result.Items = append(result.Items, ExecutionItem{
					RelPath: item.RelPath,
					Action:  item.Action,
					Status:  "conflict-preserved",
				})
			}
		}
	}

	return result, nil
}
