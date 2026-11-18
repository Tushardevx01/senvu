package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/senvu/senvu/internal/compare"
	"github.com/senvu/senvu/internal/reconcile"
	"github.com/senvu/senvu/internal/transfer"
	"github.com/senvu/senvu/pkg/manifest"
)

// ReportFormat specifies the output format for reports.
type ReportFormat string

const (
	// ReportFormatText outputs a human-readable text format.
	ReportFormatText ReportFormat = "text"
	// ReportFormatJSON outputs structured JSON.
	ReportFormatJSON ReportFormat = "json"
)

// GenerateReport formats the given data into the specified report format.
func GenerateReport(data interface{}, format ReportFormat) (string, error) {
	switch format {
	case ReportFormatJSON:
		return generateJSON(data)
	case ReportFormatText:
		return generateText(data)
	default:
		return "", fmt.Errorf("report: unsupported format %q", format)
	}
}

func generateJSON(data interface{}) (string, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("report: JSON marshalling failed: %w", err)
	}
	return string(b), nil
}

func generateText(data interface{}) (string, error) {
	switch v := data.(type) {
	case *compare.CompareResult:
		return formatCompareResult(v), nil
	case *VerifyResult:
		return formatVerifyResult(v), nil
	case *SnapshotSummary:
		return formatSnapshotSummary(v), nil
	case []SnapshotSummary:
		return formatSnapshotList(v), nil
	case *manifest.Manifest:
		return formatManifest(v), nil
	case *transfer.Result:
		return formatTransferResult(v), nil
	case *reconcile.Plan:
		return v.Summary(), nil
	case *reconcile.ExecutionResult:
		return formatReconcileExecution(v), nil
	default:
		return fmt.Sprintf("%v", data), nil
	}
}

func formatCompareResult(r *compare.CompareResult) string {
	var b strings.Builder

	b.WriteString(r.Summary())

	if len(r.Added) > 0 {
		b.WriteString("\n── Added ──\n")
		for _, f := range r.Added {
			b.WriteString(fmt.Sprintf("  + %s (%d bytes)\n", f.RelPath, f.DestSize))
		}
	}

	if len(r.Removed) > 0 {
		b.WriteString("\n── Removed ──\n")
		for _, f := range r.Removed {
			b.WriteString(fmt.Sprintf("  - %s (%d bytes)\n", f.RelPath, f.SourceSize))
		}
	}

	if len(r.Modified) > 0 {
		b.WriteString("\n── Modified ──\n")
		for _, f := range r.Modified {
			b.WriteString(fmt.Sprintf("  ~ %s\n", f.RelPath))
			b.WriteString(fmt.Sprintf("    hash: %s → %s\n", truncHash(f.SourceHash), truncHash(f.DestHash)))
			b.WriteString(fmt.Sprintf("    size: %d → %d bytes\n", f.SourceSize, f.DestSize))
		}
	}

	if len(r.Renamed) > 0 {
		b.WriteString("\n── Renamed ──\n")
		for _, f := range r.Renamed {
			b.WriteString(fmt.Sprintf("  → %s (was %s)\n", f.RelPath, f.OldPath))
		}
	}

	return b.String()
}

func formatVerifyResult(r *VerifyResult) string {
	var b strings.Builder

	if r.Passed {
		b.WriteString("Verification PASSED\n")
	} else {
		b.WriteString("Verification FAILED\n")
	}

	b.WriteString(fmt.Sprintf("  Verified:   %d files\n", len(r.Verified)))
	b.WriteString(fmt.Sprintf("  Missing:    %d files\n", len(r.Missing)))
	b.WriteString(fmt.Sprintf("  Modified:   %d files\n", len(r.Modified)))
	b.WriteString(fmt.Sprintf("  Unexpected: %d files\n", len(r.Unexpected)))
	b.WriteString(fmt.Sprintf("  Errors:     %d\n", len(r.Errors)))

	if len(r.Missing) > 0 {
		b.WriteString("\n── Missing Files ──\n")
		for _, path := range r.Missing {
			b.WriteString(fmt.Sprintf("  ! %s\n", path))
		}
	}

	if len(r.Modified) > 0 {
		b.WriteString("\n── Modified Files ──\n")
		for _, f := range r.Modified {
			b.WriteString(fmt.Sprintf("  ~ %s\n", f.RelPath))
			b.WriteString(fmt.Sprintf("    expected: %s, got: %s\n", truncHash(f.SourceHash), truncHash(f.DestHash)))
		}
	}

	if len(r.Unexpected) > 0 {
		b.WriteString("\n── Unexpected Files (not in manifest) ──\n")
		for _, path := range r.Unexpected {
			b.WriteString(fmt.Sprintf("  ? %s\n", path))
		}
	}

	if len(r.Errors) > 0 {
		b.WriteString("\n── Errors ──\n")
		for _, err := range r.Errors {
			b.WriteString(fmt.Sprintf("  ✕ %s: %s\n", err.RelPath, err.Err))
		}
	}

	return b.String()
}

func formatSnapshotSummary(s *SnapshotSummary) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Snapshot: %s\n", s.ID))
	b.WriteString(fmt.Sprintf("  Created:  %s\n", s.CreatedAt.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("  Files:    %d\n", s.FileCount))
	b.WriteString(fmt.Sprintf("  Size:     %d bytes\n", s.TotalSize))
	b.WriteString(fmt.Sprintf("  Root:     %s\n", s.SourcePath))
	return b.String()
}

func formatSnapshotList(summaries []SnapshotSummary) string {
	if len(summaries) == 0 {
		return "No snapshots found.\n"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Snapshots (%d):\n", len(summaries)))
	b.WriteString(fmt.Sprintf("  %-38s  %-20s  %-8s  %s\n", "ID", "Created", "Files", "Size"))
	b.WriteString(fmt.Sprintf("  %-38s  %-20s  %-8s  %s\n", strings.Repeat("─", 38), strings.Repeat("─", 20), strings.Repeat("─", 8), strings.Repeat("─", 10)))

	for _, s := range summaries {
		b.WriteString(fmt.Sprintf("  %-38s  %-20s  %-8d  %d bytes\n",
			s.ID,
			s.CreatedAt.Format("2006-01-02 15:04:05"),
			s.FileCount,
			s.TotalSize,
		))
	}

	return b.String()
}

func formatManifest(m *manifest.Manifest) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Snapshot ID:    %s\n", m.SnapshotID))
	b.WriteString(fmt.Sprintf("Created:        %s\n", m.CreatedAt.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("Senvu Version:  %s\n", m.SenvuVersion))
	b.WriteString(fmt.Sprintf("Schema Version: %d\n", m.SchemaVersion))
	b.WriteString(fmt.Sprintf("Hash Algorithm: %s\n", m.HashAlgorithm))
	b.WriteString(fmt.Sprintf("Source Root:    %s\n", m.SourcePath))
	b.WriteString(fmt.Sprintf("Total Files:    %d\n", m.TotalFiles))
	b.WriteString(fmt.Sprintf("Total Size:     %d bytes\n", m.TotalSize))

	if len(m.Files) > 0 {
		b.WriteString("\nFiles:\n")
		for _, f := range m.Files {
			b.WriteString(fmt.Sprintf("  %s (%d bytes, hash: %s)\n", f.RelPath, f.Size, truncHash(f.Hash)))
		}
	}
	return b.String()
}

func formatTransferResult(r *transfer.Result) string {
	var b strings.Builder
	if r.DryRun {
		b.WriteString("Transfer PREVIEW (dry-run):\n")
	} else {
		b.WriteString("Transfer Summary:\n")
	}
	b.WriteString(fmt.Sprintf("  Source:      %s\n", r.SourceRoot))
	b.WriteString(fmt.Sprintf("  Destination: %s\n", r.DestRoot))
	b.WriteString(fmt.Sprintf("  Transferred: %d files (%d bytes)\n", len(r.Transferred), r.TotalBytesTransferred))
	b.WriteString(fmt.Sprintf("  Skipped:     %d files\n", len(r.Skipped)))
	b.WriteString(fmt.Sprintf("  Conflicts:   %d files\n", len(r.Conflicts)))
	b.WriteString(fmt.Sprintf("  Errors:      %d\n", len(r.Errors)))

	if len(r.Conflicts) > 0 {
		b.WriteString("\n── Conflicts ──\n")
		for _, c := range r.Conflicts {
			b.WriteString(fmt.Sprintf("  ! %s [%s]\n", c.RelPath, c.ActionTaken))
		}
	}

	if len(r.Errors) > 0 {
		b.WriteString("\n── Errors ──\n")
		for _, err := range r.Errors {
			b.WriteString(fmt.Sprintf("  ✕ %s: %s\n", err.RelPath, err.Err))
		}
	}

	return b.String()
}

func formatReconcileExecution(res *reconcile.ExecutionResult) string {
	var b strings.Builder
	if res.DryRun {
		b.WriteString("Reconciliation PREVIEW (dry-run):\n")
	} else {
		b.WriteString("Reconciliation Result:\n")
	}
	b.WriteString(fmt.Sprintf("  Status: %s\n", map[bool]string{true: "SUCCESS", false: "COMPLETED WITH ERRORS"}[res.Success]))
	b.WriteString(fmt.Sprintf("  Total Actions: %d\n", len(res.Items)))
	if res.ErrorCount > 0 {
		b.WriteString(fmt.Sprintf("  Errors: %d\n", res.ErrorCount))
	}

	for _, item := range res.Items {
		b.WriteString(fmt.Sprintf("  [%s] %s -> %s\n", item.Action, item.RelPath, item.Status))
		if item.Error != "" {
			b.WriteString(fmt.Sprintf("    error: %s\n", item.Error))
		}
	}

	return b.String()
}

// truncHash returns a truncated hash for display (first 12 chars).
func truncHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12] + "…"
	}
	return hash
}
