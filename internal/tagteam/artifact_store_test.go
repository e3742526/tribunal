package tagteam

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStateLocatorPreparePreservesLegacyGitignore(t *testing.T) {
	workdir := t.TempDir()
	stateRoot := t.TempDir()
	runGit(t, workdir, "init")
	legacyRoot := filepath.Join(workdir, ".tagteam")
	if err := os.MkdirAll(legacyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ignorePath := filepath.Join(legacyRoot, ".gitignore")
	const contents = "*\n!.gitignore\n"
	if err := os.WriteFile(ignorePath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	locator, err := resolveStateLocator(workdir, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := locator.Prepare(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatalf("tracked-compatible legacy ignore was removed: %v", err)
	}
	if string(got) != contents {
		t.Fatalf("legacy ignore = %q, want %q", got, contents)
	}
	if !fileExists(locator.PointerPath) {
		t.Fatal("repository pointer was not written")
	}
}

func TestStateLocatorPrepareSelfIgnoresFreshPointer(t *testing.T) {
	workdir := t.TempDir()
	runGit(t, workdir, "init")
	runGit(t, workdir, "config", "user.email", "tagteam@example.com")
	runGit(t, workdir, "config", "user.name", "Tagteam Test")
	if err := os.WriteFile(filepath.Join(workdir, "README.md"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workdir, "add", "README.md")
	runGit(t, workdir, "commit", "-m", "baseline")
	locator, err := resolveStateLocator(workdir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := locator.Prepare(); err != nil {
		t.Fatal(err)
	}
	if status := strings.TrimSpace(runGit(t, workdir, "status", "--porcelain=v1", "--untracked-files=all")); status != "" {
		t.Fatalf("runtime pointer dirtied fresh repository: %q", status)
	}
	data, err := os.ReadFile(filepath.Join(workdir, ".tagteam", ".gitignore"))
	if err != nil || string(data) != "*\n" {
		t.Fatalf("runtime self-ignore = %q err=%v", data, err)
	}
}

func TestPreflightRepairsUnignoredRuntimePointer(t *testing.T) {
	workdir := t.TempDir()
	runGit(t, workdir, "init")
	runGit(t, workdir, "config", "user.email", "tagteam@example.com")
	runGit(t, workdir, "config", "user.name", "Tagteam Test")
	if err := os.WriteFile(filepath.Join(workdir, "README.md"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workdir, "add", "README.md")
	runGit(t, workdir, "commit", "-m", "baseline")
	pointer := filepath.Join(workdir, ".tagteam", repositoryPointerName)
	if err := os.MkdirAll(filepath.Dir(pointer), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pointer, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status := strings.TrimSpace(runGit(t, workdir, "status", "--porcelain=v1", "--untracked-files=all")); status == "" {
		t.Fatal("fixture pointer should initially be untracked")
	}
	if _, _, err := preflight(RunOptions{Workdir: workdir, GitSafety: "clean"}, "runtime-ignore"); err != nil {
		t.Fatalf("preflight did not repair runtime ignore: %v", err)
	}
	if status := strings.TrimSpace(runGit(t, workdir, "status", "--porcelain=v1", "--untracked-files=all")); status != "" {
		t.Fatalf("worktree remains dirty after runtime ignore repair: %q", status)
	}
}

func TestStateLocatorPrepareReconcilesDivergentLegacyPointers(t *testing.T) {
	workdir := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("TAGTEAM_STATE_ROOT", stateRoot)
	runGit(t, workdir, "init")
	locator, err := resolveStateLocator(workdir, stateRoot)
	if err != nil {
		t.Fatal(err)
	}

	legacyRunDir := filepath.Join(locator.LegacyRoot, "runs", "legacy-run")
	currentRunDir := filepath.Join(locator.RunsRoot, "current-run")
	for _, runDir := range []string{legacyRunDir, currentRunDir} {
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "final.json"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	older := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	newer := older.Add(48 * time.Hour)
	writeArtifactStoreTestJSON(t, filepath.Join(locator.LegacyRoot, "latest.json"), LatestRun{
		RunID:     "legacy-run",
		RunDir:    legacyRunDir,
		FinalPath: filepath.Join(legacyRunDir, "final.json"),
		UpdatedAt: older,
	})
	writeArtifactStoreTestJSON(t, filepath.Join(locator.RepoRoot, "latest.json"), LatestRun{
		RunID:     "current-run",
		RunDir:    currentRunDir,
		FinalPath: filepath.Join(currentRunDir, "final.json"),
		UpdatedAt: newer,
	})
	writeArtifactStoreTestJSON(t, filepath.Join(locator.LegacyRoot, "active.json"), ActiveRun{
		RunID:     "legacy-run",
		RunDir:    legacyRunDir,
		Status:    "failed",
		UpdatedAt: older,
	})
	writeArtifactStoreTestJSON(t, filepath.Join(locator.RepoRoot, "active.json"), ActiveRun{
		RunID:     "current-run",
		RunDir:    currentRunDir,
		Status:    "running",
		UpdatedAt: newer,
	})

	// Read-only commands should discover an existing external store before the
	// first successful migration writes repo.json.
	if got := statePathForWorkdir(workdir, "latest.json"); got != filepath.Join(locator.RepoRoot, "latest.json") {
		t.Fatalf("state path = %q, want external store", got)
	}
	if err := locator.Prepare(); err != nil {
		t.Fatal(err)
	}

	var latest LatestRun
	readJSONFile(t, filepath.Join(locator.RepoRoot, "latest.json"), &latest)
	if latest.RunID != "current-run" || latest.RunDir != currentRunDir || latest.FinalPath != filepath.Join(currentRunDir, "final.json") {
		t.Fatalf("reconciled latest pointer = %#v", latest)
	}
	var active ActiveRun
	readJSONFile(t, filepath.Join(locator.RepoRoot, "active.json"), &active)
	if active.RunID != "current-run" || active.Status != "running" || active.RunDir != currentRunDir {
		t.Fatalf("reconciled active pointer = %#v", active)
	}
	if !fileExists(filepath.Join(locator.RunsRoot, "legacy-run", "final.json")) {
		t.Fatal("legacy run was not merged into the external store")
	}
	for _, path := range []string{
		filepath.Join(locator.LegacyRoot, "runs"),
		filepath.Join(locator.LegacyRoot, "latest.json"),
		filepath.Join(locator.LegacyRoot, "active.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("legacy runtime path still exists after verified migration: %s", path)
		}
	}
}

func TestStateLocatorPrepareUsesNewerLegacyPointerAndNormalizesPaths(t *testing.T) {
	workdir := t.TempDir()
	stateRoot := t.TempDir()
	runGit(t, workdir, "init")
	locator, err := resolveStateLocator(workdir, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	legacyRunDir := filepath.Join(locator.LegacyRoot, "runs", "newer-legacy")
	currentRunDir := filepath.Join(locator.RunsRoot, "older-current")
	for _, runDir := range []string{legacyRunDir, currentRunDir} {
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "final.json"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	writeArtifactStoreTestJSON(t, filepath.Join(locator.RepoRoot, "latest.json"), LatestRun{RunID: "older-current", UpdatedAt: now.Add(-time.Hour)})
	writeArtifactStoreTestJSON(t, filepath.Join(locator.LegacyRoot, "latest.json"), LatestRun{RunID: "newer-legacy", RunDir: legacyRunDir, UpdatedAt: now})

	if err := locator.Prepare(); err != nil {
		t.Fatal(err)
	}
	var latest LatestRun
	readJSONFile(t, filepath.Join(locator.RepoRoot, "latest.json"), &latest)
	wantRunDir := filepath.Join(locator.RunsRoot, "newer-legacy")
	if latest.RunID != "newer-legacy" || latest.RunDir != wantRunDir || latest.FinalPath != filepath.Join(wantRunDir, "final.json") {
		t.Fatalf("newer legacy pointer was not normalized: %#v", latest)
	}
}

func TestStateLocatorPrepareRejectsConflictingRunArtifact(t *testing.T) {
	workdir := t.TempDir()
	stateRoot := t.TempDir()
	runGit(t, workdir, "init")
	locator, err := resolveStateLocator(workdir, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(locator.LegacyRoot, "runs", "same-run", "final.json")
	currentPath := filepath.Join(locator.RunsRoot, "same-run", "final.json")
	for path, contents := range map[string]string{legacyPath: "legacy\n", currentPath: "current\n"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := locator.Prepare(); err == nil {
		t.Fatal("expected conflicting same-run artifact to block migration")
	}
	if !fileExists(legacyPath) {
		t.Fatal("conflicting legacy artifact must remain available for manual recovery")
	}
}

func TestStateLocatorPreparePreservesLegacyStateUntilPointerPublished(t *testing.T) {
	workdir := t.TempDir()
	stateRoot := t.TempDir()
	runGit(t, workdir, "init")
	locator, err := resolveStateLocator(workdir, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	legacyRunDir := filepath.Join(locator.LegacyRoot, "runs", "legacy-run")
	if err := os.MkdirAll(legacyRunDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRunDir, "final.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyLatest := filepath.Join(locator.LegacyRoot, "latest.json")
	writeArtifactStoreTestJSON(t, legacyLatest, LatestRun{
		RunID:     "legacy-run",
		RunDir:    legacyRunDir,
		FinalPath: filepath.Join(legacyRunDir, "final.json"),
		UpdatedAt: time.Now().UTC(),
	})

	// A directory at the pointer path forces durable publication to fail after
	// the external copy has completed.
	if err := os.MkdirAll(locator.PointerPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := locator.Prepare(); err == nil {
		t.Fatal("expected repository pointer publication to fail")
	}
	if !fileExists(filepath.Join(legacyRunDir, "final.json")) || !fileExists(legacyLatest) {
		t.Fatal("legacy runtime state was removed before repository pointer publication")
	}
	if !fileExists(filepath.Join(locator.RunsRoot, "legacy-run", "final.json")) {
		t.Fatal("verified external copy was not retained for retry")
	}
	legacyReadPath := filepath.Join(workdir, ".tagteam", "latest.json")
	if got := statePathForWorkdir(workdir, "latest.json"); got != legacyReadPath {
		t.Fatalf("read path after interrupted migration = %q, want recoverable legacy pointer %q", got, legacyReadPath)
	}
}

func writeArtifactStoreTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
