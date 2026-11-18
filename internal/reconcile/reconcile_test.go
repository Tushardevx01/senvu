package reconcile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReconcilePlan(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// 1. Identical file in both
	os.WriteFile(filepath.Join(srcDir, "identical.txt"), []byte("same content"), 0644)
	os.WriteFile(filepath.Join(dstDir, "identical.txt"), []byte("same content"), 0644)

	// 2. New file in source (missing in dest)
	os.WriteFile(filepath.Join(srcDir, "new_source.txt"), []byte("brand new"), 0644)

	// 3. File in dest only (should be retained, NEVER deleted)
	os.WriteFile(filepath.Join(dstDir, "dest_only.txt"), []byte("only here"), 0644)

	// 4. Source newer than dest (ActionUpdate)
	os.WriteFile(filepath.Join(srcDir, "updated.txt"), []byte("new version"), 0644)
	os.WriteFile(filepath.Join(dstDir, "updated.txt"), []byte("old version"), 0644)
	oldTime := time.Now().Add(-10 * time.Minute)
	newTime := time.Now()
	os.Chtimes(filepath.Join(dstDir, "updated.txt"), oldTime, oldTime)
	os.Chtimes(filepath.Join(srcDir, "updated.txt"), newTime, newTime)

	// 5. Dest newer than source (ActionConflict)
	os.WriteFile(filepath.Join(srcDir, "conflict.txt"), []byte("source version"), 0644)
	os.WriteFile(filepath.Join(dstDir, "conflict.txt"), []byte("dest newer version"), 0644)
	os.Chtimes(filepath.Join(srcDir, "conflict.txt"), oldTime, oldTime)
	os.Chtimes(filepath.Join(dstDir, "conflict.txt"), newTime, newTime)

	engine := NewEngine()
	plan, err := engine.BuildPlan(srcDir, dstDir)
	if err != nil {
		t.Fatalf("BuildPlan failed: %v", err)
	}

	counts := plan.Counts()
	if counts[ActionIdentical] != 1 {
		t.Errorf("expected 1 identical, got %d", counts[ActionIdentical])
	}
	if counts[ActionCreate] != 1 {
		t.Errorf("expected 1 create, got %d", counts[ActionCreate])
	}
	if counts[ActionRetainDest] != 1 {
		t.Errorf("expected 1 retain_dest, got %d", counts[ActionRetainDest])
	}
	if counts[ActionUpdate] != 1 {
		t.Errorf("expected 1 update, got %d", counts[ActionUpdate])
	}
	if counts[ActionConflict] != 1 {
		t.Errorf("expected 1 conflict, got %d", counts[ActionConflict])
	}

	if !plan.HasChanges() {
		t.Errorf("plan should have changes")
	}

	summary := plan.Summary()
	if !strings.Contains(summary, "Reconciliation Plan") {
		t.Errorf("summary missing title: %s", summary)
	}
}

func TestReconcileDryRun(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("dry run source"), 0644)

	engine := NewEngine()
	plan, err := engine.BuildPlan(srcDir, dstDir)
	if err != nil {
		t.Fatalf("BuildPlan failed: %v", err)
	}

	res, err := engine.Execute(plan, Options{DryRun: true})
	if err != nil {
		t.Fatalf("Execute dry run failed: %v", err)
	}

	if !res.DryRun {
		t.Errorf("expected result DryRun to be true")
	}

	// Ensure destination file was NOT created.
	if _, err := os.Stat(filepath.Join(dstDir, "file.txt")); !os.IsNotExist(err) {
		t.Errorf("file should not exist after dry run")
	}
}

func TestReconcileExecute(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// New file to create
	os.WriteFile(filepath.Join(srcDir, "new.txt"), []byte("new file content"), 0644)

	// Existing file in dest only
	os.WriteFile(filepath.Join(dstDir, "keeper.txt"), []byte("must not be deleted"), 0644)

	// Conflict file (dest is newer)
	os.WriteFile(filepath.Join(srcDir, "conflict.txt"), []byte("source version"), 0644)
	os.WriteFile(filepath.Join(dstDir, "conflict.txt"), []byte("dest version"), 0644)
	oldTime := time.Now().Add(-10 * time.Minute)
	newTime := time.Now()
	os.Chtimes(filepath.Join(srcDir, "conflict.txt"), oldTime, oldTime)
	os.Chtimes(filepath.Join(dstDir, "conflict.txt"), newTime, newTime)

	engine := NewEngine()
	plan, err := engine.BuildPlan(srcDir, dstDir)
	if err != nil {
		t.Fatalf("BuildPlan failed: %v", err)
	}

	res, err := engine.Execute(plan, Options{DryRun: false, AutoApprove: true})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !res.Success {
		t.Errorf("execution was not successful: %d errors", res.ErrorCount)
	}

	// 1. Verify new.txt created
	data, err := os.ReadFile(filepath.Join(dstDir, "new.txt"))
	if err != nil || string(data) != "new file content" {
		t.Errorf("new.txt not created properly: %v, content=%q", err, string(data))
	}

	// 2. Verify keeper.txt was NOT deleted (Never silently delete user data!)
	keeperData, err := os.ReadFile(filepath.Join(dstDir, "keeper.txt"))
	if err != nil || string(keeperData) != "must not be deleted" {
		t.Errorf("keeper.txt was deleted or modified: %v", err)
	}

	// 3. Verify conflict.txt was preserved and updated
	entries, _ := os.ReadDir(dstDir)
	var foundConflictBackup bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "conflict.txt.conflict.") {
			foundConflictBackup = true
		}
	}
	if !foundConflictBackup {
		t.Errorf("expected conflict backup to be preserved")
	}
}
