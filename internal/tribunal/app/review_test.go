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
			payload := map[string]any{"schema_version": 1, "votes": []domain.Vote{{SchemaVersion: 1, ReviewerID: panelist.ID, FindingID: "B-0001", Choice: domain.VoteAccept, Severity: domain.SeverityMajor, Reason: "The claim needs support."}}}
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
	for _, name := range []string{"packet.json", "packet-manifest.json", "meta.json", "verification-evidence.json", "clusters.json", "votes.json", "arbitration.json", "report.md", "report.html", "state.json", "events.jsonl", "final.json"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	for _, name := range []string{"active.json", "latest.json", "findings.json"} {
		if _, err := os.Stat(filepath.Join(workspace.Root, name)); err != nil {
			t.Errorf("missing workspace artifact %s: %v", name, err)
		}
	}
	after, err := os.ReadFile(documentPath)
	if err != nil || string(after) != content {
		t.Fatalf("review changed source document: %q, %v", after, err)
	}
	resumed, resumeErr := service.Resume(context.Background(), RunRef{Input: documentPath, RunID: final.RunID})
	if !errors.As(resumeErr, &exit) || exit.Code != ExitBlockingFindings || resumed.RunID != final.RunID {
		t.Fatalf("completed resume = %#v, %v", resumed, resumeErr)
	}
	replayed, replayErr := service.Replay(context.Background(), RunRef{Input: documentPath, RunID: final.RunID})
	if !errors.As(replayErr, &exit) || exit.Code != ExitBlockingFindings || replayed.RunID == final.RunID || replayed.PacketHash != final.PacketHash {
		t.Fatalf("replay = %#v, %v", replayed, replayErr)
	}
	replayMeta, err := readMeta(filepath.Join(workspace.RunsDir, replayed.RunID, "meta.json"))
	if err != nil || replayMeta.ReplayOf != final.RunID {
		t.Fatalf("replay metadata = %#v, %v", replayMeta, err)
	}
	if err := os.Remove(filepath.Join(workspace.RunsDir, replayed.RunID, "final.json")); err != nil {
		t.Fatal(err)
	}
	restarted, restartErr := service.Resume(context.Background(), RunRef{Input: documentPath, RunID: replayed.RunID})
	if !errors.As(restartErr, &exit) || exit.Code != ExitBlockingFindings || restarted.RunID != replayed.RunID || restarted.PacketHash != final.PacketHash {
		t.Fatalf("incomplete resume = %#v, %v", restarted, restartErr)
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

func TestRunTimeoutPersistsAbortedTerminal(t *testing.T) {
	documentDir := t.TempDir()
	documentPath := filepath.Join(documentDir, "brief.md")
	if err := os.WriteFile(documentPath, []byte("# Brief\n\nText.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rubric, _ := config.BuiltinRubric("generic")
	packet, err := documents.Build(context.Background(), documentPath, documents.BuildOptions{Kind: "generic", Rubric: rubric})
	if err != nil {
		t.Fatal(err)
	}
	panel, _ := domain.ParsePanel("slow/one,slow/two,slow/three")
	slow := &adapters.FuncAdapter{AdapterID: "slow", InvokeFn: func(ctx context.Context, _ adapters.Role, _ domain.Panelist, _ adapters.Request) (adapters.Response, error) {
		<-ctx.Done()
		return adapters.Response{}, ctx.Err()
	}}
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Limits.RunTimeout = 25 * time.Millisecond
	service, err := New(cfg, store, adapters.NewRegistry(slow))
	if err != nil {
		t.Fatal(err)
	}
	final, runErr := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel})
	var exit *ExitError
	if !errors.As(runErr, &exit) || exit.Code != ExitAborted || final.Status != "aborted" {
		t.Fatalf("Review() = %#v, %v", final, runErr)
	}
	workspace, err := store.Workspace(packet.WorkspaceID, documentDir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := readFinal(filepath.Join(workspace.RunsDir, final.RunID, "final.json"))
	if err != nil || loaded.ExitCode != ExitAborted {
		t.Fatalf("aborted terminal artifact = %#v, %v", loaded, err)
	}
}

func TestBlindFindingsRemoveIdentityAndUseOpaqueIDs(t *testing.T) {
	findings := []domain.Finding{{ID: "F-R-001", Reviewer: "R-001", Persona: "skeptic", Issue: "issue"}, {ID: "F-R-002", Reviewer: "R-002", Persona: "methodologist", Issue: "other"}}
	one, mapping := blindFindings(findings, "seed")
	two, _ := blindFindings(findings, "seed")
	if len(one) != 2 || one[0].ID != two[0].ID || len(mapping) != 2 {
		t.Fatalf("blind packet is not deterministic: %#v %#v", one, mapping)
	}
	for _, finding := range one {
		if !strings.HasPrefix(finding.ID, "B-") || finding.Reviewer != "anonymous" || finding.Persona != "" || strings.Contains(finding.ID, "R-") {
			t.Fatalf("identity leaked into blind finding: %#v", finding)
		}
	}
}
