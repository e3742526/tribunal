package app

import (
	"context"
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

// --accept-majority must apply the panel recommendation carried in Default.
// A matched decision-memory ruling travels in MemoryHint; before the D-031
// repair it overwrote Default with "previous ruling: accepted", which fails
// the "accept" prefix test and silently rejected the dispute.
func TestAcceptMajorityHonorsPanelDefaultWithMemoryHint(t *testing.T) {
	final := domain.Final{Arbitration: []domain.ArbitrationDispute{
		{ID: "A-1", Default: "accept majority (2 accept / 1 reject)", MemoryHint: "previous ruling: accepted"},
		{ID: "A-2", Default: "reject majority (1 accept / 2 reject)", MemoryHint: "previous ruling: accepted"},
	}}
	rulings, err := arbitrationRulings(ArbitrationOptions{AcceptMajority: true, Operator: "op"}, final)
	if err != nil {
		t.Fatal(err)
	}
	if len(rulings) != 2 || rulings[0].Outcome != "accepted" || rulings[1].Outcome != "rejected" {
		t.Fatalf("rulings = %#v; memory hint must not invert the panel default", rulings)
	}
}

// Finals persisted before the MemoryHint repair carry the prior ruling inside
// Default; --accept-majority must honor that ruling, not reject it.
func TestAcceptMajorityHonorsLegacyPreviousRulingDefault(t *testing.T) {
	final := domain.Final{Arbitration: []domain.ArbitrationDispute{
		{ID: "A-1", Default: "previous ruling: accepted"},
		{ID: "A-2", Default: "previous ruling: deferred"},
	}}
	rulings, err := arbitrationRulings(ArbitrationOptions{AcceptMajority: true, Operator: "op"}, final)
	if err != nil {
		t.Fatal(err)
	}
	if len(rulings) != 2 || rulings[0].Outcome != "accepted" || rulings[1].Outcome != "deferred" {
		t.Fatalf("legacy rulings = %#v", rulings)
	}
}

// The host, not the model, owns evidence status. validatePass must downgrade a
// self-declared worker-verified status even when verification never runs
// (--no-workers, degraded finalization).
func TestValidatePassDowngradesSelfDeclaredWorkerVerified(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "brief.md")
	if err := os.WriteFile(path, []byte("# Brief\n\nThe launch date is unsupported.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rubric, _ := config.BuiltinRubric("generic")
	packet, err := documents.Build(context.Background(), path, documents.BuildOptions{Kind: "generic", Rubric: rubric})
	if err != nil {
		t.Fatal(err)
	}
	finding := domain.Finding{SchemaVersion: domain.FindingSchemaVersion, ID: "F-R-001", Reviewer: "R-001", Origin: "panel", Severity: domain.SeverityMajor, Category: domain.CategoryCorrectness, Anchor: domain.Anchor{Kind: "quote", PacketItem: packet.Items[0].ID, Quote: "launch date is unsupported", ItemSHA256: packet.Items[0].PacketSHA256}, Issue: "x", Recommendation: "y", EvidenceStatus: domain.EvidenceWorkerVerified, Confidence: "high"}
	results := []panelResult{{panelist: domain.Panelist{ID: "R-001"}, review: domain.Review{SchemaVersion: 1, ReviewerID: "R-001", Findings: []domain.Finding{finding}}, status: domain.PanelStatus{ReviewerID: "R-001", Status: "ok"}}}
	_, findings, _, _ := validatePass(packet, results)
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	if findings[0].EvidenceStatus == domain.EvidenceWorkerVerified {
		t.Fatalf("model self-declared worker-verified evidence survived validatePass")
	}
}

func syntheticService(t *testing.T, documentDir string, packet documents.Packet, panel domain.Panel) *Service {
	t.Helper()
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	store.Clock = func() time.Time { return clock }
	fake := &adapters.FuncAdapter{AdapterID: "fake"}
	fake.InvokeFn = func(_ context.Context, role adapters.Role, panelist domain.Panelist, req adapters.Request) (adapters.Response, error) {
		switch role {
		case adapters.RoleReviewer:
			finding := domain.Finding{SchemaVersion: domain.FindingSchemaVersion, ID: "F-" + panelist.ID, Reviewer: panelist.ID, Origin: "panel", Severity: domain.SeverityMajor, Category: domain.CategoryCorrectness, Anchor: domain.Anchor{Kind: "quote", PacketItem: packet.Items[0].ID, Quote: "launch date is unsupported", ItemSHA256: packet.Items[0].PacketSHA256}, Issue: "The launch date has no support.", Recommendation: "Cite a source.", EvidenceStatus: domain.EvidenceAnchored, Confidence: "high"}
			return jsonResponse(t, domain.Review{SchemaVersion: 1, ReviewerID: panelist.ID, Findings: []domain.Finding{finding}}), nil
		case adapters.RoleVoter:
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
	return service
}

// Resume mutates run state and must be excluded by run.lock like every other
// mutating entry point.
func TestResumeRequiresRunLock(t *testing.T) {
	documentDir := t.TempDir()
	documentPath := filepath.Join(documentDir, "brief.md")
	if err := os.WriteFile(documentPath, []byte("# Brief\n\nThe launch date is unsupported.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rubric, _ := config.BuiltinRubric("generic")
	packet, err := documents.Build(context.Background(), documentPath, documents.BuildOptions{Kind: "generic", Rubric: rubric})
	if err != nil {
		t.Fatal(err)
	}
	panel, err := domain.ParsePanel("fake/one,fake/two,fake/three")
	if err != nil {
		t.Fatal(err)
	}
	service := syntheticService(t, documentDir, packet, panel)
	t.Setenv("PATH", "")
	final, reviewErr := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel})
	var exit *ExitError
	if !errors.As(reviewErr, &exit) || exit.Code != ExitBlockingFindings {
		t.Fatalf("Review error = %v", reviewErr)
	}
	workspace, err := service.Store.Workspace(packet.WorkspaceID, documentDir)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := storage.AcquireLock(context.Background(), filepath.Join(workspace.RunsDir, final.RunID, "run.lock"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	_, resumeErr := service.Resume(ctx, RunRef{Input: documentPath, RunID: final.RunID})
	if !errors.As(resumeErr, &exit) || exit.Code != ExitPreflight || !strings.Contains(resumeErr.Error(), "run lock") {
		t.Fatalf("Resume under held lock = %v, want run-lock preflight failure", resumeErr)
	}
}

// Replay of a run whose packet was frozen with --split must re-enable
// splitting; the persisted packet still carries full item content, so the
// context preflight would otherwise always fail.
func TestReplaySplitRunSurvivesContextPreflight(t *testing.T) {
	documentDir := t.TempDir()
	documentPath := filepath.Join(documentDir, "brief.md")
	content := "# Brief\n\nThe launch date is unsupported.\n" + strings.Repeat("Filler sentence about governance posture and delivery risk.\n", 200)
	if err := os.WriteFile(documentPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	rubric, _ := config.BuiltinRubric("generic")
	packet, err := documents.Build(context.Background(), documentPath, documents.BuildOptions{Kind: "generic", Rubric: rubric})
	if err != nil {
		t.Fatal(err)
	}
	panel, err := domain.ParsePanel("fake/one,fake/two,fake/three")
	if err != nil {
		t.Fatal(err)
	}
	for i := range panel.Reviewers {
		panel.Reviewers[i].MaxContextTokens = 1200
		panel.Reviewers[i].ReservedOutputTokens = 200
	}
	service := syntheticService(t, documentDir, packet, panel)
	t.Setenv("PATH", "")
	final, reviewErr := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel, Split: true})
	var exit *ExitError
	if !errors.As(reviewErr, &exit) || exit.Code != ExitBlockingFindings {
		t.Fatalf("split review error = %v", reviewErr)
	}
	workspace, err := service.Store.Workspace(packet.WorkspaceID, documentDir)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := readPacket(filepath.Join(workspace.RunsDir, final.RunID, "packet.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.Chunks) == 0 {
		t.Fatalf("test setup did not force splitting")
	}
	replayed, replayErr := service.Replay(context.Background(), RunRef{Input: documentPath, RunID: final.RunID})
	if errors.As(replayErr, &exit) && exit.Code == ExitPreflight {
		t.Fatalf("replay of split run failed preflight: %v", replayErr)
	}
	if !errors.As(replayErr, &exit) || exit.Code != ExitBlockingFindings || replayed.RunID == final.RunID {
		t.Fatalf("replay = %#v, %v", replayed, replayErr)
	}
}
