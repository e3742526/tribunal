package tagteam

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestActivateRunWritesRunningPointerAtStatePath(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".tagteam"), 0o755); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(workdir, ".tagteam", "runs", "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	activateRun(workdir, "run-1", runDir, ModeSolo)

	active, err := readActiveRun(workdir)
	if err != nil {
		t.Fatalf("readActiveRun() error = %v", err)
	}
	if active.RunID != "run-1" {
		t.Fatalf("run_id = %q, want run-1", active.RunID)
	}
	if active.Status != "running" {
		t.Fatalf("status = %q, want running", active.Status)
	}
	if active.StatePath != filepath.Join(runDir, "state.json") {
		t.Fatalf("state_path = %q, want %q", active.StatePath, filepath.Join(runDir, "state.json"))
	}
	if active.FinalPath != filepath.Join(runDir, "final.json") {
		t.Fatalf("final_path = %q, want %q", active.FinalPath, filepath.Join(runDir, "final.json"))
	}
	if active.RunDir != runDir {
		t.Fatalf("run_dir = %q, want %q", active.RunDir, runDir)
	}
	if active.Mode != ModeSolo {
		t.Fatalf("mode = %q, want %q", active.Mode, ModeSolo)
	}
	if active.SchemaVersion != ArtifactSchemaVersion {
		t.Fatalf("schema_version = %d, want %d", active.SchemaVersion, ArtifactSchemaVersion)
	}
	if active.StartedAt.IsZero() || active.UpdatedAt.IsZero() {
		t.Fatalf("expected started_at/updated_at to be set: %#v", active)
	}
}

func TestDeactivateRunCompletedRemovesPointer(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".tagteam"), 0o755); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(workdir, ".tagteam", "runs", "run-1")
	activateRun(workdir, "run-1", runDir, ModeSolo)

	deactivateRun(workdir, "run-1", true)

	if _, err := os.Stat(activeRunPath(workdir)); !os.IsNotExist(err) {
		t.Fatalf("expected active.json to be removed after a completed run, stat err = %v", err)
	}
}

func TestDeactivateRunFailedMarksStatusFailed(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".tagteam"), 0o755); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(workdir, ".tagteam", "runs", "run-1")
	activateRun(workdir, "run-1", runDir, ModeSolo)

	deactivateRun(workdir, "run-1", false)

	active, err := readActiveRun(workdir)
	if err != nil {
		t.Fatalf("readActiveRun() error = %v", err)
	}
	if active.Status != "failed" {
		t.Fatalf("status = %q, want failed", active.Status)
	}
	if active.Status == "running" {
		t.Fatal("active pointer must not be left running after a failed run")
	}
}

func TestFinalizeAndClearActiveRunIgnoreStalePointer(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".tagteam"), 0o755); err != nil {
		t.Fatal(err)
	}
	activateRun(workdir, "newer-run", "irrelevant", ModeSolo)

	if err := finalizeActiveRun(workdir, "older-run", "failed"); err != nil {
		t.Fatalf("finalizeActiveRun() error = %v", err)
	}
	if err := clearActiveRun(workdir, "older-run"); err != nil {
		t.Fatalf("clearActiveRun() error = %v", err)
	}

	active, err := readActiveRun(workdir)
	if err != nil {
		t.Fatalf("expected newer run's pointer to remain: %v", err)
	}
	if active.RunID != "newer-run" || active.Status != "running" {
		t.Fatalf("stale run must not modify a newer run's pointer: %#v", active)
	}
}

func TestReadActiveRunMissingReturnsError(t *testing.T) {
	workdir := t.TempDir()
	if _, err := readActiveRun(workdir); err == nil {
		t.Fatal("expected error reading a missing active.json")
	}
}

func TestRunSolo_SuccessfulRunRemovesActivePointer(t *testing.T) {
	installFakeClaudeBinary(t)

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	mustWriteFile(t, filepath.Join(repo, "README.md"), "hello\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "init")

	app := NewApp(DefaultConfig())
	final, err := app.Run(context.Background(), RunOptions{
		Prompt:  "add a feature",
		Workdir: repo,
		Mode:    ModeSolo,
		Coder:   RoleTarget{Adapter: "claude"},
		Rounds:  1,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if final.RunID == "" {
		t.Fatal("expected a run id")
	}
	if _, err := os.Stat(activeRunPath(repo)); !os.IsNotExist(err) {
		t.Fatalf("expected active.json to be removed after a successful run, stat err = %v", err)
	}

	var latest LatestRun
	readJSONFile(t, filepath.Join(repo, ".tagteam", "latest.json"), &latest)
	if latest.RunID != final.RunID {
		t.Fatalf("latest run id = %q, want %q (existing latest/final behavior must still pass)", latest.RunID, final.RunID)
	}
}

func TestRunLoop_FailureAfterRunDirCreationDoesNotLeaveActiveRunning(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	mustWriteFile(t, filepath.Join(repo, "README.md"), "hello\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "init")

	app := NewApp(DefaultConfig())
	final, err := app.Run(context.Background(), RunOptions{
		Prompt:    "ship it",
		Workdir:   repo,
		Mode:      ModeAdversarial,
		Coder:     RoleTarget{Adapter: "missing-adapter"},
		Adversary: RoleTarget{Adapter: "claude"},
		Rounds:    1,
		Timeout:   10 * time.Second,
	})
	if err == nil {
		t.Fatal("expected unknown adapter error")
	}
	if final.RunDir == "" {
		t.Fatal("expected run dir in failed final")
	}

	active, readErr := readActiveRun(repo)
	if readErr != nil {
		t.Fatalf("expected active.json to still exist for postmortem: %v", readErr)
	}
	if active.Status == "running" {
		t.Fatal("active pointer must not be left running after a failed run")
	}
	if active.Status != "failed" {
		t.Fatalf("status = %q, want failed", active.Status)
	}
}
