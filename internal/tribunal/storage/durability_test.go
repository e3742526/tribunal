package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func testWorkspace(t *testing.T) Workspace {
	t.Helper()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.Workspace(strings.Repeat("ab", 12), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func testDecision(fingerprint string) DecisionRecord {
	return DecisionRecord{SchemaVersion: domain.SchemaVersion, PacketItem: "artifact:x.md", Fingerprint: fingerprint, Ruling: "accepted", Date: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), Operator: "op"}
}

// A crash mid-append leaves a torn trailing line. Reads must skip it and the
// next append must quarantine it instead of permanently bricking the journal.
func TestTornDecisionTailIsQuarantinedAndRecovered(t *testing.T) {
	workspace := testWorkspace(t)
	fp := strings.Repeat("ab", 8)
	if err := AppendDecision(workspace, testDecision(fp)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace.Root, "decisions.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"schema_version":1,"packet_item":"artifact:x.md","fing`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	records, err := ReadDecisions(workspace)
	if err != nil || len(records) != 1 {
		t.Fatalf("read with torn tail: %d records, %v", len(records), err)
	}
	second := strings.Repeat("cd", 8)
	if err := AppendDecision(workspace, testDecision(second)); err != nil {
		t.Fatalf("append after torn tail: %v", err)
	}
	records, err = ReadDecisions(workspace)
	if err != nil || len(records) != 2 {
		t.Fatalf("read after repair: %d records, %v", len(records), err)
	}
	corrupt, err := os.ReadFile(path + ".corrupt")
	if err != nil || !strings.Contains(string(corrupt), `"fing`) {
		t.Fatalf("torn fragment not quarantined: %q, %v", corrupt, err)
	}
}

// Transition must refuse an invalid next state; persisting it would brick
// every later read of the run directory.
func TestTransitionValidatesNextState(t *testing.T) {
	runDir := t.TempDir()
	next := domain.RunState{SchemaVersion: domain.SchemaVersion, RunID: "run", WorkspaceID: "ws", PacketHash: "hash", Phase: domain.PhaseReviewing, Status: ""}
	if err := Transition(runDir, next); err == nil {
		t.Fatal("empty status accepted")
	}
	if _, err := os.Stat(filepath.Join(runDir, "state.json")); !os.IsNotExist(err) {
		t.Fatal("invalid state was persisted")
	}
}

// Findings that vanish from later runs must not stay "observed" forever;
// explicit dispositions and open blocker/major records survive.
func TestUpdateLedgerMarksUnseenFindingsStale(t *testing.T) {
	workspace := testWorkspace(t)
	minor := domain.Finding{SchemaVersion: domain.FindingSchemaVersion, ID: "F-1", Reviewer: "R-001", Origin: "panel", Severity: domain.SeverityMinor, Category: domain.CategoryStyle, Anchor: domain.Anchor{Kind: "quote", PacketItem: "artifact:x.md", Quote: "one", ItemSHA256: "s"}, Issue: "i", Recommendation: "r", EvidenceStatus: domain.EvidenceAnchored, Confidence: "high"}
	major := minor
	major.ID, major.Severity, major.Anchor.Quote = "F-2", domain.SeverityMajor, "two"
	if err := UpdateLedger(workspace, "run-1", []domain.Finding{minor, major}, []domain.Decision{{SchemaVersion: domain.SchemaVersion, FindingID: "F-2", Outcome: "accepted", Severity: domain.SeverityMajor}}, []string{"artifact:x.md"}); err != nil {
		t.Fatal(err)
	}
	// run-2 re-examines artifact:x.md and observes neither finding.
	if err := UpdateLedger(workspace, "run-2", nil, nil, []string{"artifact:x.md"}); err != nil {
		t.Fatal(err)
	}
	ledger, err := LoadLedger(workspace)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, record := range ledger.Findings {
		statuses[record.Finding.ID] = record.Status
	}
	if statuses["F-1"] != "stale" {
		t.Fatalf("unseen minor finding status = %q, want stale", statuses["F-1"])
	}
	if statuses["F-2"] != "open" {
		t.Fatalf("unseen open major finding status = %q, want open", statuses["F-2"])
	}
	// A run scoped to a different packet item must not disturb records it
	// never examined.
	if err := UpdateLedger(workspace, "run-3", nil, nil, []string{"artifact:other.md"}); err != nil {
		t.Fatal(err)
	}
	ledger, err = LoadLedger(workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range ledger.Findings {
		if record.Finding.ID == "F-2" && record.Status != "open" {
			t.Fatalf("out-of-scope run changed F-2 status to %q", record.Status)
		}
	}
}

// On case-insensitive filesystems a case-variant spelling must not defeat the
// state/document disjointness guard.
func TestWorkspaceRejectsCaseVariantContainment(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("case-insensitive default filesystem is darwin-specific")
	}
	base := t.TempDir()
	stateRoot := filepath.Join(base, "State", "tribunal")
	docRoot := filepath.Join(base, "state", "tribunal", "docs")
	if err := os.MkdirAll(docRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := New(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Workspace(strings.Repeat("ab", 12), docRoot); err == nil {
		t.Fatal("case-variant document root inside state root was accepted")
	}
}
