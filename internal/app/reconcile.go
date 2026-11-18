package app

import (
	"fmt"

	"github.com/senvu/senvu/internal/output"
	"github.com/senvu/senvu/internal/reconcile"
)

// ReconcileOptions configures reconciliation execution.
type ReconcileOptions struct {
	SourcePath  string
	DestPath    string
	DryRun      bool
	AutoApprove bool
}

// Reconcile executes reconciliation between two storage locations.
func Reconcile(opts ReconcileOptions, out *output.Handler) (*reconcile.ExecutionResult, error) {
	engine := reconcile.NewEngine()

	plan, err := engine.BuildPlan(opts.SourcePath, opts.DestPath)
	if err != nil {
		return nil, fmt.Errorf("building reconciliation plan: %w", err)
	}

	if out != nil && !opts.AutoApprove && !opts.DryRun {
		// Log plan summary.
		out.Printf("%s", plan.Summary())
	}

	result, err := engine.Execute(plan, reconcile.Options{
		DryRun:      opts.DryRun,
		AutoApprove: opts.AutoApprove,
	})
	if err != nil {
		return nil, fmt.Errorf("reconciliation failed: %w", err)
	}

	return result, nil
}
