package tagteam

import (
	"os"
	"path/filepath"
	"testing"
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
