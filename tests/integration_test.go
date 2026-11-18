package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/senvu/senvu/pkg/manifest"
)

var binPath string

func TestMain(m *testing.M) {
	// Find or build the binary for integration tests.
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	// Walk up to find repo root if running inside tests/.
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			panic("could not find repo root")
		}
		root = parent
	}

	binName := "senvu"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath = filepath.Join(root, "bin", binName)

	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/senvu")
	buildCmd.Dir = root
	// If a local .gopath exists in dev sandbox, append it to GOPATH.
	if _, statErr := os.Stat(filepath.Join(root, ".gopath")); statErr == nil {
		buildCmd.Env = append(os.Environ(), "GOPATH="+filepath.Join(root, ".gopath"))
	}
	if out, err := buildCmd.CombinedOutput(); err != nil {
		panic("failed building binary: " + string(out))
	}

	code := m.Run()
	os.Exit(code)
}

func runSenvu(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := stdout.String() + stderr.String()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			out += fmt.Sprintf("\nexec error: %v (binPath=%s)", err, binPath)
			exitCode = -1
		}
	}
	return out, exitCode
}

func TestCLI_Init(t *testing.T) {
	workspace := t.TempDir()

	out, code := runSenvu(t, "", "init", workspace)
	if code != 0 {
		t.Fatalf("init failed with code %d: %s", code, out)
	}

	// Verify .senvu directory structure.
	if _, err := os.Stat(filepath.Join(workspace, ".senvu", "config.json")); err != nil {
		t.Errorf("config.json not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".senvu", "db")); err != nil {
		t.Errorf(".senvu/db not found: %v", err)
	}

	// Refuse re-init without --force.
	_, code2 := runSenvu(t, "", "init", workspace)
	if code2 == 0 {
		t.Errorf("re-init without force should fail")
	}

	// Re-init with --force.
	_, code3 := runSenvu(t, "", "init", workspace, "--force")
	if code3 != 0 {
		t.Errorf("re-init with --force failed: code %d", code3)
	}
}

func TestCLI_SnapshotWorkflow(t *testing.T) {
	workspace := t.TempDir()

	// Initialize workspace.
	runSenvu(t, "", "init", workspace)

	// Create test files.
	os.WriteFile(filepath.Join(workspace, "hello.txt"), []byte("Hello world!"), 0644)
	os.MkdirAll(filepath.Join(workspace, "sub"), 0755)
	os.WriteFile(filepath.Join(workspace, "sub", "doc.md"), []byte("# Senvu Documentation"), 0644)

	// Create snapshot.
	exportPath := filepath.Join(t.TempDir(), "portable-manifest.json")
	out, code := runSenvu(t, workspace, "snapshot", "create", ".", "--export", exportPath)
	if code != 0 {
		t.Fatalf("snapshot create failed: %s", out)
	}

	if !strings.Contains(out, "Created snapshot") {
		t.Errorf("unexpected output: %s", out)
	}

	// Verify exported manifest.
	m, err := manifest.Load(exportPath)
	if err != nil {
		t.Fatalf("failed loading exported manifest: %v", err)
	}
	if m.TotalFiles != 2 {
		t.Errorf("expected 2 files in manifest, got %d", m.TotalFiles)
	}

	// List snapshots.
	listOut, listCode := runSenvu(t, workspace, "snapshot", "list", ".")
	if listCode != 0 {
		t.Fatalf("snapshot list failed: %s", listOut)
	}
	if !strings.Contains(listOut, m.SnapshotID) {
		t.Errorf("snapshot ID %s not found in list output: %s", m.SnapshotID, listOut)
	}

	// Show snapshot.
	showOut, showCode := runSenvu(t, workspace, "snapshot", "show", m.SnapshotID)
	if showCode != 0 {
		t.Fatalf("snapshot show failed: %s", showOut)
	}
	if !strings.Contains(showOut, "hello.txt") || !strings.Contains(showOut, "sub/doc.md") {
		t.Errorf("snapshot show missing files: %s", showOut)
	}
}

func TestCLI_Compare(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// 1. Identical file
	os.WriteFile(filepath.Join(src, "same.txt"), []byte("same"), 0644)
	os.WriteFile(filepath.Join(dst, "same.txt"), []byte("same"), 0644)

	// 2. Added in dst
	os.WriteFile(filepath.Join(dst, "added.txt"), []byte("added"), 0644)

	// 3. Removed (in src, not dst)
	os.WriteFile(filepath.Join(src, "removed.txt"), []byte("removed"), 0644)

	// 4. Modified
	os.WriteFile(filepath.Join(src, "mod.txt"), []byte("v1"), 0644)
	os.WriteFile(filepath.Join(dst, "mod.txt"), []byte("v2"), 0644)

	out, code := runSenvu(t, "", "compare", src, dst)
	// Code should be 1 since differences exist.
	if code != 1 {
		t.Errorf("expected exit code 1 for differences, got %d: %s", code, out)
	}

	if !strings.Contains(out, "added.txt") {
		t.Errorf("missing added.txt in compare output: %s", out)
	}
	if !strings.Contains(out, "removed.txt") {
		t.Errorf("missing removed.txt in compare output: %s", out)
	}
	if !strings.Contains(out, "mod.txt") {
		t.Errorf("missing mod.txt in compare output: %s", out)
	}

	// JSON output mode.
	jsonOut, codeJSON := runSenvu(t, "", "compare", src, dst, "--json")
	if codeJSON != 1 {
		t.Errorf("expected exit code 1 in json mode, got %d", codeJSON)
	}
	var cmpMap map[string]interface{}
	if err := json.Unmarshal([]byte(jsonOut), &cmpMap); err != nil {
		t.Errorf("invalid json output: %v\n%s", err, jsonOut)
	}
}

func TestCLI_Verify(t *testing.T) {
	dataDir := t.TempDir()
	manifestDir := t.TempDir()
	manifestFile := filepath.Join(manifestDir, "manifest.json")

	os.WriteFile(filepath.Join(dataDir, "good.txt"), []byte("perfect data"), 0644)

	// Create manifest using snapshot create.
	runSenvu(t, "", "init", dataDir)
	_, code := runSenvu(t, dataDir, "snapshot", "create", ".", "--export", manifestFile)
	if code != 0 {
		t.Fatalf("snapshot create failed")
	}

	// Verify should pass.
	vOut, vCode := runSenvu(t, dataDir, "verify", dataDir, "--manifest", manifestFile)
	if vCode != 0 {
		t.Fatalf("verify should pass, got code %d: %s", vCode, vOut)
	}
	if !strings.Contains(vOut, "Verification PASSED") {
		t.Errorf("expected PASSED in output: %s", vOut)
	}

	// Modify file -> verify should fail!
	os.WriteFile(filepath.Join(dataDir, "good.txt"), []byte("corrupted data!"), 0644)
	vOut2, vCode2 := runSenvu(t, dataDir, "verify", dataDir, "--manifest", manifestFile)
	if vCode2 != 1 {
		t.Errorf("verify modified file should exit with 1, got %d: %s", vCode2, vOut2)
	}
	if !strings.Contains(vOut2, "Verification FAILED") {
		t.Errorf("expected FAILED in output: %s", vOut2)
	}

	// Delete file -> verify detects missing file!
	os.Remove(filepath.Join(dataDir, "good.txt"))
	vOut3, vCode3 := runSenvu(t, dataDir, "verify", dataDir, "--manifest", manifestFile)
	if vCode3 != 1 {
		t.Errorf("verify missing file should exit with 1, got %d: %s", vCode3, vOut3)
	}
	if !strings.Contains(vOut3, "Missing Files") {
		t.Errorf("expected Missing Files in output: %s", vOut3)
	}
}

func TestCLI_TransferAndReconcile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	os.WriteFile(filepath.Join(src, "item.txt"), []byte("payload"), 0644)

	// 1. Dry run transfer.
	dryOut, dryCode := runSenvu(t, "", "transfer", src, dst, "--dry-run")
	if dryCode != 0 {
		t.Fatalf("transfer dry-run failed: %s", dryOut)
	}
	if _, err := os.Stat(filepath.Join(dst, "item.txt")); !os.IsNotExist(err) {
		t.Errorf("dry-run should not create files")
	}

	// 2. Real transfer.
	tOut, tCode := runSenvu(t, "", "transfer", src, dst)
	if tCode != 0 {
		t.Fatalf("transfer failed: %s", tOut)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "item.txt")); err != nil || string(data) != "payload" {
		t.Errorf("transferred file corrupted or missing: %v", err)
	}

	// 3. Reconcile dry run.
	rDryOut, rDryCode := runSenvu(t, "", "reconcile", src, dst, "--dry-run")
	if rDryCode != 0 {
		t.Fatalf("reconcile dry-run failed: %s", rDryOut)
	}
	if !strings.Contains(rDryOut, "Reconciliation PREVIEW") {
		t.Errorf("expected preview in output: %s", rDryOut)
	}

	// 4. Reconcile execution with conflict preservation.
	// Make destination newer with different content.
	oldTime := time.Now().Add(-10 * time.Minute)
	newTime := time.Now()
	os.Chtimes(filepath.Join(src, "item.txt"), oldTime, oldTime)
	os.WriteFile(filepath.Join(dst, "item.txt"), []byte("dest modification"), 0644)
	os.Chtimes(filepath.Join(dst, "item.txt"), newTime, newTime)

	rOut, rCode := runSenvu(t, "", "reconcile", src, dst, "--yes")
	if rCode != 0 {
		t.Fatalf("reconcile failed: %s", rOut)
	}

	// Verify conflict was preserved.
	entries, _ := os.ReadDir(dst)
	var foundBackup bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "item.txt.conflict.") {
			foundBackup = true
		}
	}
	if !foundBackup {
		t.Errorf("conflict file was not preserved during reconcile")
	}
}

func TestCLI_ReportExport(t *testing.T) {
	workspace := t.TempDir()
	runSenvu(t, "", "init", workspace)
	os.WriteFile(filepath.Join(workspace, "r.txt"), []byte("report data"), 0644)
	runSenvu(t, workspace, "snapshot", "create", ".")

	// Export text report.
	textReport, code := runSenvu(t, workspace, "report", "export", "--format", "text")
	if code != 0 {
		t.Fatalf("report export text failed: %s", textReport)
	}
	if !strings.Contains(textReport, "Snapshots") {
		t.Errorf("unexpected report content: %s", textReport)
	}

	// Export JSON report to file.
	outFile := filepath.Join(t.TempDir(), "out.json")
	_, code2 := runSenvu(t, workspace, "report", "export", "--format", "json", "-o", outFile)
	if code2 != 0 {
		t.Fatalf("report export json to file failed")
	}
	data, err := os.ReadFile(outFile)
	if err != nil || len(data) == 0 {
		t.Errorf("expected non-empty json file: %v", err)
	}
}
