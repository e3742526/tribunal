package tagteam

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestValidateTestCommandRejectsMissingLiteralPath(t *testing.T) {
	err := validateTestCommand(t.TempDir(), "go test ./missing_test.go")
	if err == nil || ExitCode(err) != ExitPreflightFailed {
		t.Fatalf("error = %v, want preflight failure", err)
	}
}

func TestIsolatedTestDirectoriesArePerInvocation(t *testing.T) {
	runDir := t.TempDir()
	firstState, firstTemp, err := isolatedTestDirectories(filepath.Join(runDir, "baseline-test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	secondState, secondTemp, err := isolatedTestDirectories(filepath.Join(runDir, "test-round-1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if firstState == secondState || firstTemp == secondTemp {
		t.Fatalf("test isolation reused directories: %q %q", firstState, secondState)
	}
}

func TestRegressionComparesFailureIdentitySets(t *testing.T) {
	baseline := TestRun{Passed: false, FailureIdentities: []string{"TestKnown"}}
	known := compareRegression(baseline, TestRun{Passed: false, FailureIdentities: []string{"TestKnown"}})
	if known.Status != "no_new_failures" {
		t.Fatalf("known status = %q", known.Status)
	}
	newFailure := compareRegression(baseline, TestRun{Passed: false, FailureIdentities: []string{"TestKnown", "TestNew"}})
	if newFailure.Status != "new_failures" || !reflect.DeepEqual(newFailure.NewFailures, []string{"TestNew"}) {
		t.Fatalf("new failure result = %#v", newFailure)
	}
	unknown := compareRegression(baseline, TestRun{Passed: false})
	if unknown.Status != "unknown" {
		t.Fatalf("unknown status = %q", unknown.Status)
	}
}

func TestCustomFailureIdentityRegex(t *testing.T) {
	got := extractFailureIdentitiesWithRegex("CASE=checkout-flow failed\n", `CASE=([^ ]+)`)
	if !reflect.DeepEqual(got, []string{"checkout-flow"}) {
		t.Fatalf("identities = %#v", got)
	}
}

func TestQualityGateRejectsOutOfScopeAndHostPaths(t *testing.T) {
	findings := evaluateScopeFindings([]DiffFile{{Path: "other.go"}, {Path: ".tagteam/repo.json"}}, []string{"allowed.go"})
	if len(findings) != 2 || findings[0].Severity != "major" || findings[1].Severity != "blocker" {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestChurnGateUsesConfiguredThresholds(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "baseline")
	findings := evaluateChurnFindings(context.Background(), repo, stringsTrim(runGit(t, repo, "rev-parse", "HEAD")), []DiffFile{{Path: "a.txt"}, {Path: "b.txt"}}, ChurnThresholds{MaxFiles: 1, MaxChangedLines: 100, MaxFixtureFiles: 100, WhitespaceRatio: 1, MinimumSemanticRatio: 0.01})
	if len(findings) != 1 || findings[0].ID != "CHURN-FILES" {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestLearnedTimeoutCapRequiresTwoCloseObservations(t *testing.T) {
	repoState := t.TempDir()
	runDir := filepath.Join(repoState, "runs", "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	calibration := TimeoutCalibration{Adapter: "agy", Model: "model", AdapterVersion: "1.2.3"}
	history := timeoutHistory{SchemaVersion: ArtifactSchemaVersion, Observations: []timeoutObservation{
		{Adapter: "agy", Model: "model", AdapterVersion: "1.2.3", Duration: "5m"},
		{Adapter: "agy", Model: "model", AdapterVersion: "1.2.3", Duration: "5m10s"},
	}}
	if err := writeJSONWithNewline(filepath.Join(repoState, "adapter-timeout-history.json"), history); err != nil {
		t.Fatal(err)
	}
	if got := learnedTimeoutCap(runDir, calibration); got != 5*time.Minute {
		t.Fatalf("learned cap = %s", got)
	}
	history.Observations[1].Duration = "8m"
	if err := writeJSONWithNewline(filepath.Join(repoState, "adapter-timeout-history.json"), history); err != nil {
		t.Fatal(err)
	}
	if got := learnedTimeoutCap(runDir, calibration); got != 0 {
		t.Fatalf("divergent observations learned cap %s", got)
	}
}
