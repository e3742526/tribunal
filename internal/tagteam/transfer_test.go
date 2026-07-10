package tagteam

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTransferRunAppliesReviewedPatchToPrimaryCheckout(t *testing.T) {
	primary := t.TempDir()
	runGit(t, primary, "init")
	runGit(t, primary, "config", "user.email", "test@example.com")
	runGit(t, primary, "config", "user.name", "Test User")
	readme := filepath.Join(primary, "README.md")
	if err := os.WriteFile(readme, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, primary, "add", "README.md")
	runGit(t, primary, "commit", "-m", "baseline")
	baseline := stringsTrim(runGit(t, primary, "rev-parse", "HEAD"))
	source := filepath.Join(t.TempDir(), "source")
	runGit(t, primary, "worktree", "add", "-b", "transfer-test", source, baseline)
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runID := "transfer-success"
	runDir, err := createRunDir(source, "", runID)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := captureDiffArtifact(context.Background(), source, baseline, runDir, 1)
	if err != nil {
		t.Fatal(err)
	}
	review := &Review{
		SchemaVersion:            ReviewSchemaVersion,
		Verdict:                  "pass",
		Summary:                  "review passed",
		Findings:                 []Finding{},
		TestSuggestions:          []string{},
		DataLossChecks:           notApplicableDataLossChecks("not applicable in transfer fixture"),
		PriorFindingDispositions: []FindingDisposition{},
	}
	final := FinalRun{
		SchemaVersion:    ArtifactSchemaVersion,
		RunID:            runID,
		RunDir:           runDir,
		Workdir:          source,
		Baseline:         baseline,
		Status:           RunStatusPassed,
		Verdict:          "pass",
		ExitCode:         ExitSuccess,
		LatestDiffPath:   diff.PatchPath,
		LatestDiffSHA256: diff.Metadata.DiffSHA256,
		Review:           review,
		Regression:       &RegressionResult{Status: "no_new_failures"},
		QualityGates: []QualityGateResult{{
			SchemaVersion: ArtifactSchemaVersion,
			Round:         1,
			AllowedScope:  []string{"README.md"},
			Findings:      []GateFinding{},
			CheckedAt:     time.Now().UTC(),
		}},
		StartedAt:  time.Now().Add(-time.Minute).UTC(),
		FinishedAt: time.Now().UTC(),
	}
	if err := writeJSONWithNewline(filepath.Join(runDir, "final.json"), final); err != nil {
		t.Fatal(err)
	}
	record, err := TransferRun(context.Background(), RunOptions{
		Workdir:        source,
		TestCmd:        "test -f README.md",
		LintCmd:        "git diff --check",
		Timeout:        10 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	}, runID, "")
	if err != nil {
		t.Fatalf("TransferRun() error = %v record=%#v", err, record)
	}
	if record.Status != "transferred" || record.TargetDiffSHA != diff.Metadata.DiffSHA256 {
		t.Fatalf("record = %#v", record)
	}
	data, err := os.ReadFile(readme)
	if err != nil || string(data) != "after\n" {
		t.Fatalf("target README = %q err=%v", data, err)
	}
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}
