// Package main is the entry point for the Senvu CLI.
// Senvu is a local-first digital integrity & portability toolkit.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/senvu/senvu/internal/app"
	"github.com/senvu/senvu/internal/output"
	"github.com/senvu/senvu/pkg/manifest"
	"github.com/spf13/cobra"
)

const (
	version = "0.1.0"
	tagline = "Keep every copy trustworthy."
)

var (
	flagVerbose bool
	flagQuiet   bool
	flagJSON    bool
	flagConfig  string
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "senvu",
		Short:         tagline,
		Long:          fmt.Sprintf("Senvu v%s — %s\n\nA local-first digital integrity & portability toolkit.", version, tagline),
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	// Global flags.
	root.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose output")
	root.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "Suppress all non-error output")
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output results as JSON")
	root.PersistentFlags().StringVar(&flagConfig, "config", "", "Path to config file")

	// Register subcommands.
	root.AddCommand(
		initCmd(),
		snapshotCmd(),
		compareCmd(),
		verifyCmd(),
		transferCmd(),
		reconcileCmd(),
		reportCmd(),
	)

	return root
}

// newOutput creates an output handler from the current global flags.
func newOutput() *output.Handler {
	return output.New(output.Options{
		JSONMode: flagJSON,
		Quiet:    flagQuiet,
		Verbose:  flagVerbose,
	})
}

// --- init command ---

func initCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init [directory]",
		Short: "Initialize a new Senvu workspace",
		Long:  "Initialize a new Senvu workspace in the specified directory (defaults to current directory).",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := newOutput()
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}

			return app.Init(app.InitOptions{
				TargetDir: targetDir,
				Force:     force,
				Version:   version,
			}, out)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force re-initialization of existing workspace")
	return cmd
}

// --- snapshot command (with subcommands) ---

func snapshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Manage content snapshots",
		Long:  "Create, list, and inspect content snapshots of a directory or workspace.",
	}

	cmd.AddCommand(
		snapshotCreateCmd(),
		snapshotListCmd(),
		snapshotShowCmd(),
	)

	return cmd
}

func snapshotCreateCmd() *cobra.Command {
	var exportPath string

	cmd := &cobra.Command{
		Use:   "create [directory]",
		Short: "Create a new snapshot of the directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := newOutput()
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}

			svc := app.NewSnapshotService()
			m, err := svc.CreateSnapshot(targetDir)
			if err != nil {
				return err
			}

			if exportPath != "" {
				if err := m.Save(exportPath); err != nil {
					return fmt.Errorf("exporting manifest: %w", err)
				}
				out.Log(output.LevelInfo, fmt.Sprintf("Exported manifest to %s", exportPath))
			}

			if flagJSON {
				data, _ := json.MarshalIndent(m, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			out.Success(fmt.Sprintf("Created snapshot %s", m.SnapshotID))
			out.Printf("  Files:     %d\n", m.TotalFiles)
			out.Printf("  Total Size: %d bytes\n", m.TotalSize)
			out.Printf("  Algorithm:  %s\n", m.HashAlgorithm)
			out.Printf("  Source:     %s\n", m.SourcePath)
			if exportPath != "" {
				out.Printf("  Exported:   %s\n", exportPath)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&exportPath, "export", "e", "", "Export manifest to specified JSON file")
	return cmd
}

func snapshotListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [directory]",
		Short: "List all snapshots in workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := newOutput()
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}

			svc := app.NewSnapshotService()
			summaries, err := svc.ListSnapshots(targetDir)
			if err != nil {
				return err
			}

			if flagJSON {
				data, _ := json.MarshalIndent(summaries, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			rep, err := app.GenerateReport(summaries, app.ReportFormatText)
			if err != nil {
				return err
			}
			out.Printf("%s", rep)
			return nil
		},
	}
}

func snapshotShowCmd() *cobra.Command {
	var workspaceDir string

	cmd := &cobra.Command{
		Use:   "show <snapshot-id>",
		Short: "Show details of a specific snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := newOutput()
			snapshotID := args[0]
			targetDir := "."
			if workspaceDir != "" {
				targetDir = workspaceDir
			}

			svc := app.NewSnapshotService()
			m, err := svc.ShowSnapshot(targetDir, snapshotID)
			if err != nil {
				return err
			}

			if flagJSON {
				data, _ := json.MarshalIndent(m, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			rep, err := app.GenerateReport(m, app.ReportFormatText)
			if err != nil {
				return err
			}
			out.Printf("%s", rep)
			return nil
		},
	}

	cmd.Flags().StringVarP(&workspaceDir, "workspace", "w", ".", "Workspace directory containing database")
	return cmd
}

// --- compare command ---

func compareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compare <source> <destination>",
		Short: "Compare two directories using content hashes",
		Long:  "Compare two directories to detect added, removed, modified, unchanged, and renamed files using BLAKE3 hashes.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := newOutput()
			sourcePath := args[0]
			destPath := args[1]

			result, err := app.CompareDirectories(sourcePath, destPath)
			if err != nil {
				return err
			}

			if flagJSON {
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
			} else {
				rep, _ := app.GenerateReport(result, app.ReportFormatText)
				out.Printf("%s", rep)
			}

			// Exit code 1 if differences found, 0 if identical.
			if len(result.Added) > 0 || len(result.Removed) > 0 || len(result.Modified) > 0 || len(result.Renamed) > 0 {
				os.Exit(1)
			}
			return nil
		},
	}
}

// --- verify command ---

func verifyCmd() *cobra.Command {
	var (
		manifestPath string
		snapshotID   string
		workspaceDir string
	)

	cmd := &cobra.Command{
		Use:   "verify [directory]",
		Short: "Verify integrity of files against trusted manifest or snapshot",
		Long:  "Verify that directory contents match a snapshot or portable manifest, checking for modifications, corruption, missing, or unexpected files.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := newOutput()
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}

			var m *manifest.Manifest
			var err error

			if manifestPath != "" {
				m, err = manifest.Load(manifestPath)
				if err != nil {
					return fmt.Errorf("loading manifest %q: %w", manifestPath, err)
				}
			} else if snapshotID != "" {
				svc := app.NewSnapshotService()
				m, err = svc.ShowSnapshot(workspaceDir, snapshotID)
				if err != nil {
					return fmt.Errorf("loading snapshot %q: %w", snapshotID, err)
				}
			} else {
				// Default: load latest snapshot from workspace.
				svc := app.NewSnapshotService()
				list, lErr := svc.ListSnapshots(targetDir)
				if lErr != nil || len(list) == 0 {
					return fmt.Errorf("no manifest provided (--manifest) and no snapshots found in workspace (run 'senvu snapshot create' first)")
				}
				m, err = svc.ShowSnapshot(targetDir, list[0].ID)
				if err != nil {
					return fmt.Errorf("loading latest snapshot: %w", err)
				}
			}

			result, err := app.Verify(targetDir, m)
			if err != nil {
				return err
			}

			if flagJSON {
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
			} else {
				rep, _ := app.GenerateReport(result, app.ReportFormatText)
				out.Printf("%s", rep)
			}

			if !result.Passed {
				os.Exit(result.ExitCode())
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", "", "Path to portable JSON manifest to verify against")
	cmd.Flags().StringVarP(&snapshotID, "snapshot", "s", "", "Snapshot ID from workspace database to verify against")
	cmd.Flags().StringVarP(&workspaceDir, "workspace", "w", ".", "Workspace directory containing database")
	return cmd
}

// --- transfer command ---

func transferCmd() *cobra.Command {
	var (
		dryRun    bool
		overwrite bool
	)

	cmd := &cobra.Command{
		Use:   "transfer <source> <destination>",
		Short: "Transfer files safely with verification",
		Long:  "Safely transfer files between locations with temporary files, atomic placement, conflict detection, and BLAKE3 verification.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := newOutput()
			source := args[0]
			destination := args[1]

			result, err := app.Transfer(app.TransferOptions{
				SourcePath: source,
				DestPath:   destination,
				DryRun:     dryRun,
				Overwrite:  overwrite,
			}, out)
			if err != nil {
				return err
			}

			if flagJSON {
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
			} else {
				rep, _ := app.GenerateReport(result, app.ReportFormatText)
				out.Printf("%s", rep)
			}

			if len(result.Errors) > 0 {
				os.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Preview transfer without modifying destination")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Allow overwriting conflicting files at destination")
	return cmd
}

// --- reconcile command ---

func reconcileCmd() *cobra.Command {
	var (
		dryRun bool
		yes    bool
	)

	cmd := &cobra.Command{
		Use:   "reconcile <source> <backup>",
		Short: "Safely reconcile differences between copies",
		Long:  "Compare both locations, detect conflicting modifications, present a dry-run execution plan, and preserve conflicting copies safely.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := newOutput()
			source := args[0]
			backup := args[1]

			result, err := app.Reconcile(app.ReconcileOptions{
				SourcePath:  source,
				DestPath:    backup,
				DryRun:      dryRun,
				AutoApprove: yes,
			}, out)
			if err != nil {
				return err
			}

			if flagJSON {
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
			} else {
				rep, _ := app.GenerateReport(result, app.ReportFormatText)
				out.Printf("%s", rep)
			}

			if !result.Success {
				os.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Show preview plan without applying changes")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Automatically approve and execute reconciliation plan")
	return cmd
}

// --- report command (with subcommands) ---

func reportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report [snapshot-id]",
		Short: "Generate integrity and workspace reports",
		Long:  "Generate and export structured integrity reports for snapshots, comparisons, and workspaces.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := newOutput()
			targetDir := "."
			svc := app.NewSnapshotService()

			if len(args) > 0 {
				// Show specific snapshot report.
				m, err := svc.ShowSnapshot(targetDir, args[0])
				if err != nil {
					return err
				}
				if flagJSON {
					data, _ := json.MarshalIndent(m, "", "  ")
					fmt.Println(string(data))
				} else {
					rep, _ := app.GenerateReport(m, app.ReportFormatText)
					out.Printf("%s", rep)
				}
				return nil
			}

			// Show workspace snapshots summary.
			summaries, err := svc.ListSnapshots(targetDir)
			if err != nil {
				return err
			}

			if flagJSON {
				data, _ := json.MarshalIndent(summaries, "", "  ")
				fmt.Println(string(data))
			} else {
				rep, _ := app.GenerateReport(summaries, app.ReportFormatText)
				out.Printf("%s", rep)
			}
			return nil
		},
	}

	cmd.AddCommand(reportExportCmd())
	return cmd
}

func reportExportCmd() *cobra.Command {
	var (
		formatStr  string
		outputPath string
		snapshotID string
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a report to a file or stdout",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := newOutput()
			format := app.ReportFormat(formatStr)
			if format != app.ReportFormatJSON && format != app.ReportFormatText {
				return fmt.Errorf("invalid format %q (allowed: json, text)", formatStr)
			}

			svc := app.NewSnapshotService()
			var data interface{}
			var err error

			if snapshotID != "" {
				data, err = svc.ShowSnapshot(".", snapshotID)
				if err != nil {
					return fmt.Errorf("loading snapshot %q: %w", snapshotID, err)
				}
			} else {
				data, err = svc.ListSnapshots(".")
				if err != nil {
					return fmt.Errorf("listing snapshots: %w", err)
				}
			}

			content, err := app.GenerateReport(data, format)
			if err != nil {
				return err
			}

			if outputPath != "" {
				if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
					return err
				}
				if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
					return fmt.Errorf("writing report to %s: %w", outputPath, err)
				}
				out.Success(fmt.Sprintf("Exported report to %s", outputPath))
			} else {
				fmt.Println(content)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&formatStr, "format", "json", "Output format (json or text)")
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "File path to save the report (defaults to stdout)")
	cmd.Flags().StringVarP(&snapshotID, "snapshot", "s", "", "Snapshot ID to export report for")
	return cmd
}
