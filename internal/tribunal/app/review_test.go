package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/config"
	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

func TestReviewPersistsBarrierAndCompletesWithoutGit(t *testing.T) {
	documentDir := t.TempDir()
	documentPath := filepath.Join(documentDir, "brief.md")
	content := "# Brief\n\nThe launch date is unsupported.\n"
	if err := os.WriteFile(documentPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	rubric, _ := config.BuiltinRubric("generic")
	packet, err := documents.Build(context.Background(), documentPath, documents.BuildOptions{Kind: "generic", Rubric: rubric})
	if err != nil {
		t.Fatal(err)
	}
	stateRoot := t.TempDir()
	store, err := storage.New(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	store.Clock = func() time.Time { return clock }
	panel, err := domain.ParsePanel("fake/one,fake/two,fake/three")
	if err != nil {
		t.Fatal(err)
	}
	fake := &adapters.FuncAdapter{AdapterID: "fake"}
	fake.InvokeFn = func(_ context.Context, role adapters.Role, panelist domain.Panelist, req adapters.Request) (adapters.Response, error) {
		switch role {
		case adapters.RoleReviewer:
			finding := domain.Finding{
				SchemaVersion: domain.FindingSchemaVersion,
				ID:            "F-" + panelist.ID,
				Reviewer:      panelist.ID,
				Origin:        "panel",
				Severity:      domain.SeverityMajor,
				Category:      domain.CategoryCorrectness,
				Anchor: domain.Anchor{
					Kind:       "quote",
					PacketItem: packet.Items[0].ID,
					Quote:      "launch date is unsupported",
					ItemSHA256: packet.Items[0].PacketSHA256,
				},
				Issue:          "The launch date has no support.",
				Recommendation: "Cite a source or remove the date.",
				EvidenceStatus: domain.EvidenceAnchored,
				Confidence:     "high",
			}
			return jsonResponse(t, domain.Review{SchemaVersion: 1, ReviewerID: panelist.ID, Findings: []domain.Finding{finding}}), nil
		case adapters.RoleVoter:
			runDir := filepath.Clean(filepath.Join(req.RunDir, "..", "..", ".."))
			for _, reviewer := range panel.Reviewers {
				if _, err := os.Stat(filepath.Join(runDir, "calls", reviewer.ID, "review", "parsed.json")); err != nil {
					t.Errorf("vote began before pass-1 durability barrier for %s: %v", reviewer.ID, err)
				}
			}
			if strings.Contains(req.Prompt, "reviewer\":\"R-") || strings.Contains(req.Prompt, "persona\":\"") {
				t.Errorf("vote prompt leaked reviewer identity: %s", req.Prompt)
			}
			payload := map[string]any{"schema_version": 1, "votes": []domain.Vote{{SchemaVersion: 1, ReviewerID: panelist.ID, FindingID: "F-R-001", Choice: domain.VoteAccept, Severity: domain.SeverityMajor, Reason: "The claim needs support."}}}
			return jsonResponse(t, payload), nil
		default:
			return adapters.Response{}, errors.New("unexpected role")
		}
	}
	service, err := New(config.Default(), store, adapters.NewRegistry(fake))
	if err != nil {
		t.Fatal(err)
	}
	service.Clock = func() time.Time { return clock }
	t.Setenv("PATH", "")
	final, reviewErr := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel})
	var exit *ExitError
	if !errors.As(reviewErr, &exit) || exit.Code != ExitBlockingFindings {
		t.Fatalf("Review error = %v, want exit %d", reviewErr, ExitBlockingFindings)
	}
	if final.Status != "findings" || len(final.Decisions) != 1 || final.Decisions[0].Outcome != "accepted" {
		t.Fatalf("unexpected final: %#v", final)
	}
	workspace, err := store.Workspace(packet.WorkspaceID, documentDir)
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(workspace.RunsDir, final.RunID)
	for _, name := range []string{"packet.json", "meta.json", "clusters.json", "votes.json", "arbitration.json", "report.md", "report.html", "state.json", "events.jsonl", "final.json"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	after, err := os.ReadFile(documentPath)
	if err != nil || string(after) != content {
		t.Fatalf("review changed source document: %q, %v", after, err)
	}
}

func jsonResponse(t *testing.T, value any) adapters.Response {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return adapters.Response{Raw: data}
}
