package tagteam

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPersistRunStateJournalsPhaseTransitions(t *testing.T) {
	runDir := t.TempDir()
	states := []RunState{
		{RunID: "run-1", Status: "running", Phase: string(PhasePlanning)},
		{RunID: "run-1", Status: "running", Phase: string(PhaseImplementing)},
		{RunID: "run-1", Status: "running", Phase: string(PhaseTesting)},
		{RunID: "run-1", Status: "running", Phase: string(PhaseReviewing)},
	}
	for _, state := range states {
		if err := persistRunState(runDir, state); err != nil {
			t.Fatal(err)
		}
	}
	state, err := readRunState(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != runStateSchemaVersion || state.Phase != string(PhaseReviewing) || state.CompletedPhase != PhaseTesting {
		t.Fatalf("state = %#v", state)
	}
	events, err := os.ReadFile(filepath.Join(runDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(strings.TrimSpace(string(events)), "\n") + 1; got != len(states) {
		t.Fatalf("event count = %d, want %d\n%s", got, len(states), events)
	}
}

func TestResumeReturnsAlreadyPassedRunAfterVerification(t *testing.T) {
	repo, baseline := createResumeFixtureRepo(t)
	runID := "resume-passed"
	runDir, err := createRunDir(repo, "", runID)
	if err != nil {
		t.Fatal(err)
	}
	writeResumeFixture(t, runDir, runID, repo, baseline, RunStatusPassed)
	final, err := NewApp(DefaultConfig()).Resume(context.Background(), RunOptions{Workdir: repo}, runID)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if final.RunID != runID || final.Status != RunStatusPassed {
		t.Fatalf("final = %#v", final)
	}
}

func TestResumeQuarantinesBaselineMismatch(t *testing.T) {
	repo, baseline := createResumeFixtureRepo(t)
	runID := "resume-mismatch"
	runDir, err := createRunDir(repo, "", runID)
	if err != nil {
		t.Fatal(err)
	}
	writeResumeFixture(t, runDir, runID, repo, baseline, RunStatusRunning)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("new head\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "advance")
	final, err := NewApp(DefaultConfig()).Resume(context.Background(), RunOptions{Workdir: repo}, runID)
	if err == nil || final.Status != RunStatusQuarantined {
		t.Fatalf("final=%#v err=%v", final, err)
	}
	state, readErr := readRunState(runDir)
	if readErr != nil || state.Status != string(RunStatusQuarantined) {
		t.Fatalf("state=%#v err=%v", state, readErr)
	}
}

func createResumeFixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "baseline")
	return repo, strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
}

func writeResumeFixture(t *testing.T, runDir, runID, repo, baseline string, status RunStatus) {
	t.Helper()
	meta := Meta{SchemaVersion: ArtifactSchemaVersion, RunID: runID, Workdir: repo, Baseline: baseline, Command: "run", Prompt: "fixture", StartedAt: time.Now().UTC(), Adapters: map[string]string{"worker": "fake", "supervisor": "fake"}, Models: map[string]string{}}
	if err := writeJSONWithNewline(filepath.Join(runDir, "meta.json"), meta); err != nil {
		t.Fatal(err)
	}
	emptyDiff := sha256Sum([]byte{})
	state := RunState{SchemaVersion: runStateSchemaVersion, RunID: runID, Workdir: repo, BaselineSHA: baseline, Status: string(status), Phase: string(PhaseReviewing), DiffHash: emptyDiff}
	if err := writeJSONWithNewline(filepath.Join(runDir, "state.json"), state); err != nil {
		t.Fatal(err)
	}
	final := FinalRun{SchemaVersion: ArtifactSchemaVersion, RunID: runID, RunDir: runDir, Workdir: repo, Baseline: baseline, Mode: ModeSupervisor, Status: status, Verdict: "pass", ExitCode: ExitSuccess, FinishedAt: time.Now().UTC()}
	if err := writeJSONWithNewline(filepath.Join(runDir, "final.json"), final); err != nil {
		t.Fatal(err)
	}
}
