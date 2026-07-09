package tui

import (
	"bytes"
	"testing"
	"time"

	"github.com/cephalopod-ai/tagteam/internal/tagteam"
)

func fixtureSnapshot() tagteam.RunSnapshot {
	updated := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	return tagteam.RunSnapshot{
		SchemaVersion:   1,
		RunID:           "2026-01-01T120000.000000000Z",
		RunDir:          "/repo/.tagteam/runs/2026-01-01T120000.000000000Z",
		Mode:            tagteam.ModeSupervisor,
		Status:          "running",
		Phase:           "round-1",
		Verdict:         "needs_changes",
		ExitCode:        1,
		Degraded:        true,
		DegradedReason:  "scout_unavailable",
		BlockingReason:  "blocking_findings",
		CurrentRound:    1,
		RoundsCompleted: 0,
		RoundsRequested: 2,
		RoleStatuses: map[string]tagteam.RoleStatus{
			"worker":     {Role: "worker", Status: "completed", Adapter: "claude", Model: "sonnet"},
			"supervisor": {Role: "supervisor", Status: "running", Adapter: "claude", Model: "opus"},
		},
		LatestDiffPath:   "/repo/.tagteam/runs/x/diff-round-1.patch",
		LatestReviewPath: "/repo/.tagteam/runs/x/supervisor-round-1.json",
		LatestTestPath:   "/repo/.tagteam/runs/x/test-round-1.txt",
		ChangedFiles:     []string{"main.go", "README.md"},
		FindingsCount:    2,
		UpdatedAt:        updated,
	}
}

func fixturePlan() *tagteam.ExecutionPlan {
	return &tagteam.ExecutionPlan{
		RunID:  "2026-01-01T120000.000000000Z",
		Status: "running",
		Items: []tagteam.PlanItem{
			{ID: "P1", Title: "one", Status: tagteam.PlanStatusPassed},
			{ID: "P2", Title: "two", Status: tagteam.PlanStatusPending},
		},
	}
}

func TestRenderStableOutput(t *testing.T) {
	snapshot := fixtureSnapshot()
	plan := fixturePlan()

	var first, second bytes.Buffer
	Render(&first, snapshot, plan, DefaultToggles())
	Render(&second, snapshot, plan, DefaultToggles())

	if first.String() != second.String() {
		t.Fatalf("Render output is not stable across identical inputs:\n--- first ---\n%s\n--- second ---\n%s", first.String(), second.String())
	}
	if first.Len() == 0 {
		t.Fatal("expected non-empty render output")
	}
}

func TestRenderIncludesExpectedPanels(t *testing.T) {
	snapshot := fixtureSnapshot()
	plan := fixturePlan()

	var buf bytes.Buffer
	Render(&buf, snapshot, plan, DefaultToggles())
	out := buf.String()

	for _, want := range []string{
		"run=2026-01-01T120000.000000000Z",
		"mode=supervisor",
		"status=running",
		"degraded=true reason=scout_unavailable",
		"blocking_reason=blocking_findings",
		"Roles:",
		"worker",
		"supervisor",
		"Plan (run=2026-01-01T120000.000000000Z status=running items=2):",
		"[P1]",
		"[P2]",
		"Findings: 2",
		"Changed files (2):",
		"main.go",
		"diff:   /repo/.tagteam/runs/x/diff-round-1.patch",
		"[q] quit",
	} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Fatalf("render output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestRenderTogglesHidePanels(t *testing.T) {
	snapshot := fixtureSnapshot()
	plan := fixturePlan()

	var buf bytes.Buffer
	Render(&buf, snapshot, plan, Toggles{})
	out := buf.String()

	for _, notWant := range []string{"Plan (run=", "Findings:", "Changed files ("} {
		if bytes.Contains([]byte(out), []byte(notWant)) {
			t.Fatalf("expected toggled-off panel %q to be hidden, got:\n%s", notWant, out)
		}
	}
	// The header/roles/footer panels are not gated by toggles.
	for _, want := range []string{"run=2026-01-01T120000.000000000Z", "Roles:", "[q] quit"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Fatalf("expected always-on panel %q, got:\n%s", want, out)
		}
	}
}

func TestRenderNilPlanOmitsPlanPanel(t *testing.T) {
	snapshot := fixtureSnapshot()

	var buf bytes.Buffer
	Render(&buf, snapshot, nil, DefaultToggles())
	if bytes.Contains(buf.Bytes(), []byte("Plan (run=")) {
		t.Fatalf("expected no plan panel when plan is nil, got:\n%s", buf.String())
	}
}
