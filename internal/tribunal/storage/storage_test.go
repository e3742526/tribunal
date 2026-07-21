package storage

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func TestWorkspaceRejectsStateInsideDocuments(t *testing.T) {
	doc := t.TempDir()
	store, err := New(filepath.Join(doc, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Workspace("0123456789abcdef01234567", doc); err == nil {
		t.Fatal("expected state-root rejection")
	}
}

func TestTransitionJournalPrecedesSnapshot(t *testing.T) {
	runDir := t.TempDir()
	state := domain.RunState{SchemaVersion: 1, RunID: "run", WorkspaceID: "0123456789abcdef01234567", Phase: domain.PhaseInit, Status: "running", UpdatedAt: time.Unix(1, 0).UTC()}
	if err := Transition(runDir, state); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "events.jsonl")); err != nil {
		t.Fatal(err)
	}
	var loaded domain.RunState
	if err := ReadJSON(filepath.Join(runDir, "state.json"), &loaded); err != nil || loaded.Phase != domain.PhaseInit {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}

func TestCreateRunUsesULIDAndPrivateDirectory(t *testing.T) {
	root, doc := t.TempDir(), t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	store.Clock = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	store.Entropy = bytes.NewReader(make([]byte, 32))
	workspace, err := store.Workspace("0123456789abcdef01234567", doc)
	if err != nil {
		t.Fatal(err)
	}
	runID, runDir, err := store.CreateRun(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(runID) != 26 {
		t.Fatalf("run id = %q", runID)
	}
	info, err := os.Stat(runDir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("mode=%v err=%v", info.Mode(), err)
	}
}

func TestLockContentionHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.lock")
	first, err := AcquireLock(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := AcquireLock(ctx, path, nil); err == nil {
		t.Fatal("expected cancelled lock acquisition")
	}
}

func TestMultiprocessLockContention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.lock")
	command := exec.Command(os.Args[0], "-test.run=^TestLockHelper$")
	command.Env = append(os.Environ(), "TRIBUNAL_LOCK_HELPER=1", "TRIBUNAL_LOCK_PATH="+path)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	}()
	if scanner := bufio.NewScanner(stdout); !scanner.Scan() || scanner.Text() != "locked" {
		t.Fatal("helper did not acquire lock")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := AcquireLock(ctx, path, nil); err == nil {
		t.Fatal("second process lock unexpectedly succeeded")
	}
}

func TestLockHelper(t *testing.T) {
	if os.Getenv("TRIBUNAL_LOCK_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	lock, err := AcquireLock(context.Background(), os.Getenv("TRIBUNAL_LOCK_PATH"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	fmt.Println("locked")
	time.Sleep(2 * time.Second)
}
