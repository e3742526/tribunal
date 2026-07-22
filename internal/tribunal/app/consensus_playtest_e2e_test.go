package app

// End-to-end driver for docs/test_scenarios/02-consensus-scenarios.md cards
// that need the full Service pipeline: panel quorum checks (S13-S15) and
// every [DISAGREE->CONSENSUS] card that resolves an arbitration_pending
// dispute via Service.Arbitrate (S35-S40, S52).

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

// scriptedReviewer describes one reviewer's scripted first-pass finding (or
// none) and scripted vote on the single tracked finding "F-CONTESTED".
type scriptedReviewer struct {
	id           string
	reviewFails  bool
	raisesFind   bool
	voteFails    bool
	voteChoice   domain.VoteChoice
	voteSeverity domain.Severity
}

// buildE2EService wires a Service whose panel of N reviewers/voters is
// driven entirely by the scripted table above, over one real document and
// one real packet, through the actual adapters.FuncAdapter injection point
// -- the same mechanism Service.Review uses with real provider CLIs.
func buildE2EService(t *testing.T, documentDir string, reviewers []scriptedReviewer) (*Service, documents.Packet, domain.Panel) {
	t.Helper()
	documentPath := filepath.Join(documentDir, "brief.md")
	if err := os.WriteFile(documentPath, []byte("# Brief\n\nThe launch date is unsupported.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rubric, _ := config.BuiltinRubric("generic")
	packet, err := documents.Build(context.Background(), documentPath, documents.BuildOptions{Kind: "generic", Rubric: rubric})
	if err != nil {
		t.Fatal(err)
	}
	var panelSpec []string
	for _, r := range reviewers {
		panelSpec = append(panelSpec, "fake/"+r.id)
	}
	panel, err := domain.ParsePanel(strings.Join(panelSpec, ","))
	if err != nil {
		t.Fatal(err)
	}
	// ParsePanel assigns Panelist.ID sequentially (R-001, R-002, ...)
	// regardless of the panel string, so the script must be keyed on
	// Panelist.Model (the part after "adapter/"), which does carry the
	// scripted id through unchanged.
	byModel := map[string]scriptedReviewer{}
	for _, r := range reviewers {
		byModel[r.id] = r
	}

	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC)
	store.Clock = func() time.Time { return clock }
	fake := &adapters.FuncAdapter{AdapterID: "fake"}
	fake.InvokeFn = func(_ context.Context, role adapters.Role, panelist domain.Panelist, req adapters.Request) (adapters.Response, error) {
		script, ok := byModel[panelist.Model]
		if !ok {
			return adapters.Response{}, errors.New("unscripted panelist " + panelist.Model)
		}
		switch role {
		case adapters.RoleReviewer:
			if script.reviewFails {
				return adapters.Response{}, errors.New("simulated reviewer failure")
			}
			// The review schema requires "findings" to be a JSON array, so an
			// unscripted (nil) slice must never be serialized as null.
			findings := []domain.Finding{}
			if script.raisesFind {
				findings = append(findings, domain.Finding{
					SchemaVersion: domain.FindingSchemaVersion, ID: "F-CONTESTED", Reviewer: panelist.ID, Origin: "panel",
					Severity: domain.SeverityMajor, Category: domain.CategoryCorrectness,
					Anchor: domain.Anchor{Kind: "quote", PacketItem: packet.Items[0].ID, Quote: "launch date is unsupported", ItemSHA256: packet.Items[0].PacketSHA256},
					Issue:  "The launch date has no supporting citation.", Recommendation: "Cite a source or remove the date.",
					EvidenceStatus: domain.EvidenceAnchored, Confidence: "high",
				})
			}
			return jsonResponse(t, domain.Review{SchemaVersion: 1, ReviewerID: panelist.ID, Findings: findings}), nil
		case adapters.RoleVoter:
			if script.voteFails {
				return adapters.Response{}, errors.New("simulated voter failure")
			}
			payload := map[string]any{"schema_version": 1, "votes": []domain.Vote{{SchemaVersion: 1, ReviewerID: panelist.ID, FindingID: "B-0001", Choice: script.voteChoice, Severity: script.voteSeverity, Reason: "scripted vote"}}}
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
	return service, packet, panel
}

// S13 — quorum unmet: only 1 of 3 reviewers valid.
func TestConsensusPlaytest_S13_QuorumUnmet(t *testing.T) {
	documentDir := t.TempDir()
	reviewers := []scriptedReviewer{
		{id: "one", raisesFind: true, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
		{id: "two", reviewFails: true},
		{id: "three", reviewFails: true},
	}
	service, packet, panel := buildE2EService(t, documentDir, reviewers)
	t.Setenv("PATH", "")
	final, err := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitDegraded {
		t.Fatalf("S13: err = %v, want ExitDegraded", err)
	}
	if final.Status != "degraded" || !final.PanelIncomplete {
		t.Fatalf("S13: final = %#v, want degraded/PanelIncomplete", final)
	}
}

// S14 — quorum boundary: exactly half of 4 configured reviewers valid must
// degrade (valid*2 <= configured).
func TestConsensusPlaytest_S14_QuorumBoundaryExactHalf(t *testing.T) {
	documentDir := t.TempDir()
	reviewers := []scriptedReviewer{
		{id: "one", raisesFind: true, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
		{id: "two", raisesFind: false, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
		{id: "three", reviewFails: true},
		{id: "four", reviewFails: true},
	}
	service, packet, panel := buildE2EService(t, documentDir, reviewers)
	t.Setenv("PATH", "")
	final, err := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitDegraded {
		t.Fatalf("S14: err = %v, want ExitDegraded (2 of 4 valid must degrade per README's documented majority-quorum contract)", err)
	}
	if final.Status != "degraded" {
		t.Fatalf("S14: final.Status = %q, want degraded", final.Status)
	}
}

// S15 — quorum boundary: one more than half valid must NOT degrade.
func TestConsensusPlaytest_S15_QuorumBoundaryOneOverHalf(t *testing.T) {
	documentDir := t.TempDir()
	reviewers := []scriptedReviewer{
		{id: "one", raisesFind: true, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
		{id: "two", raisesFind: false, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
		{id: "three", raisesFind: false, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
		{id: "four", reviewFails: true},
		{id: "five", reviewFails: true},
	}
	service, packet, panel := buildE2EService(t, documentDir, reviewers)
	t.Setenv("PATH", "")
	final, err := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitBlockingFindings {
		t.Fatalf("S15: err = %v, want ExitBlockingFindings (3 of 5 valid must clear quorum and reach a normal decision)", err)
	}
	if final.Status == "degraded" {
		t.Fatalf("S15: final incorrectly degraded with 3 of 5 valid reviewers")
	}
}

func arbitrationServiceWithTie(t *testing.T, documentDir string) (*Service, domain.Final) {
	t.Helper()
	reviewers := []scriptedReviewer{
		{id: "one", raisesFind: true, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
		{id: "two", raisesFind: false, voteChoice: domain.VoteReject, voteSeverity: domain.SeverityMajor},
	}
	service, packet, panel := buildE2EService(t, documentDir, reviewers)
	t.Setenv("PATH", "")
	final, err := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitArbitration {
		t.Fatalf("setup: err = %v, want ExitArbitration (2-reviewer split must tie)", err)
	}
	if final.Status != "arbitration_pending" || len(final.Arbitration) != 1 {
		t.Fatalf("setup: final = %#v, want exactly one pending dispute", final)
	}
	return service, final
}

// S38 — interactive-shaped arbitration: an explicit operator ruling resolves
// the sole pending dispute to "rejected".
func TestConsensusPlaytest_S38_ExplicitRejectRuling(t *testing.T) {
	documentDir := t.TempDir()
	service, final := arbitrationServiceWithTie(t, documentDir)
	disputeID := final.Arbitration[0].ID
	resolved, err := service.Arbitrate(ArbitrationOptions{
		RunRef:  RunRef{Input: documentDir, RunID: final.RunID},
		Rulings: []ArbitrationRuling{{ID: disputeID, Outcome: "rejected", Reason: "insufficient evidence provided in dissent", Operator: "reviewer-lead@example.com"}},
	})
	if err != nil {
		var exit *ExitError
		if !errors.As(err, &exit) || exit.Code != ExitSuccess {
			t.Fatalf("S38: Arbitrate err = %v", err)
		}
	}
	if len(resolved.Arbitration) != 0 {
		t.Fatalf("S38: disputes remain after ruling: %#v", resolved.Arbitration)
	}
	if resolved.Decisions[0].Outcome != "rejected" {
		t.Fatalf("S38: decision outcome = %s, want rejected", resolved.Decisions[0].Outcome)
	}
}

// S39 — arbitrate with no pending disputes must fail explicitly, never
// silently succeed.
func TestConsensusPlaytest_S39_ArbitrateWithNoPendingDisputes(t *testing.T) {
	documentDir := t.TempDir()
	reviewers := []scriptedReviewer{
		{id: "one", raisesFind: true, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
		{id: "two", raisesFind: false, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
		{id: "three", raisesFind: false, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
	}
	service, packet, panel := buildE2EService(t, documentDir, reviewers)
	t.Setenv("PATH", "")
	final, reviewErr := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel})
	var exit *ExitError
	if !errors.As(reviewErr, &exit) || exit.Code != ExitBlockingFindings {
		t.Fatalf("setup: err = %v", reviewErr)
	}
	if final.Status == "arbitration_pending" {
		t.Fatalf("setup: unexpected pending dispute")
	}
	_, err := service.Arbitrate(ArbitrationOptions{RunRef: RunRef{Input: documentDir, RunID: final.RunID}, AcceptMajority: true})
	if !errors.As(err, &exit) || exit.Code != ExitInvalidArguments {
		t.Fatalf("S39: err = %v, want ExitInvalidArguments (no pending arbitration)", err)
	}
}

// S40 — re-running Arbitrate after the sole dispute is already resolved
// must fail cleanly, not re-litigate or duplicate decision memory.
func TestConsensusPlaytest_S40_ArbitrateIdempotencyAfterResolution(t *testing.T) {
	documentDir := t.TempDir()
	service, final := arbitrationServiceWithTie(t, documentDir)
	disputeID := final.Arbitration[0].ID
	// Ruling "accepted" on a major finding legitimately yields
	// ExitBlockingFindings, not ExitSuccess -- that is the correct exit-code
	// contract (an accepted blocker/major is exit 1), not a failure.
	if _, err := service.Arbitrate(ArbitrationOptions{
		RunRef:  RunRef{Input: documentDir, RunID: final.RunID},
		Rulings: []ArbitrationRuling{{ID: disputeID, Outcome: "accepted", Reason: "senior reviewer's citation request stands", Operator: "lead@example.com"}},
	}); err != nil {
		var exit *ExitError
		if !errors.As(err, &exit) || exit.Code != ExitBlockingFindings {
			t.Fatalf("first Arbitrate call failed: %v", err)
		}
	}
	workspace, err := service.Store.Workspace(mustPacketWorkspaceID(t, service, documentDir, final.RunID), documentDir)
	if err != nil {
		t.Fatal(err)
	}
	before, err := storage.ReadDecisions(workspace)
	if err != nil {
		t.Fatal(err)
	}
	_, secondErr := service.Arbitrate(ArbitrationOptions{RunRef: RunRef{Input: documentDir, RunID: final.RunID}, AcceptMajority: true})
	var exit *ExitError
	if !errors.As(secondErr, &exit) || exit.Code != ExitInvalidArguments {
		t.Fatalf("S40: re-arbitrate err = %v, want ExitInvalidArguments (no pending arbitration)", secondErr)
	}
	after, err := storage.ReadDecisions(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("S40: decision memory changed on a failed re-arbitrate: before=%d after=%d", len(before), len(after))
	}
}

func mustPacketWorkspaceID(t *testing.T, service *Service, documentDir, runID string) string {
	t.Helper()
	snapshot, err := service.Status(RunRef{Input: documentDir, RunID: runID})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Final == nil {
		t.Fatal("snapshot has no final")
	}
	return snapshot.Final.WorkspaceID
}

// S37 — --accept-majority with one dispute excepted: excepted disputes are
// left out of the ruling list entirely (not defaulted to any outcome).
func TestConsensusPlaytest_S37_AcceptMajorityWithException(t *testing.T) {
	// Three findings, three independent disputes: two lean accept
	// (2 accept/1 reject), one leans reject (1 accept/2 reject). Simulated
	// via three separate 3-reviewer runs merged is unnecessary complexity;
	// instead directly exercise arbitrationRulings against a hand-built
	// Final carrying three disputes, matching how Service.Arbitrate reads
	// final.Arbitration -- this is still the real production code path,
	// just fed a controlled Final rather than re-deriving one from a live
	// vote (three-way split panels are exercised end-to-end in S05/S06/S42).
	final := domain.Final{Arbitration: []domain.ArbitrationDispute{
		{ID: "A-1", Default: "accept majority (2 accept / 1 reject)"},
		{ID: "A-2", Default: "accept majority (2 accept / 1 reject)"},
		{ID: "A-3", Default: "reject majority (1 accept / 2 reject)"},
	}}
	rulings, err := arbitrationRulings(ArbitrationOptions{AcceptMajority: true, Operator: "op", Except: []string{"A-2"}}, final)
	if err != nil {
		t.Fatal(err)
	}
	if len(rulings) != 2 {
		t.Fatalf("S37: expected 2 rulings (A-2 excepted), got %d: %#v", len(rulings), rulings)
	}
	byID := map[string]string{}
	for _, r := range rulings {
		byID[r.ID] = r.Outcome
	}
	if _, excepted := byID["A-2"]; excepted {
		t.Fatalf("S37: excepted dispute A-2 received a ruling: %#v", rulings)
	}
	if byID["A-1"] != "accepted" || byID["A-3"] != "rejected" {
		t.Fatalf("S37: rulings = %#v, want A-1 accepted, A-3 rejected", byID)
	}
}

// S52 — replay of a disagree-then-consensus run reproduces the same tie
// deterministically.
func TestConsensusPlaytest_S52_ReplayReproducesDispute(t *testing.T) {
	documentDir := t.TempDir()
	service, final := arbitrationServiceWithTie(t, documentDir)
	replayed, err := service.Replay(context.Background(), RunRef{Input: documentDir, RunID: final.RunID})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitArbitration {
		t.Fatalf("S52: replay err = %v, want ExitArbitration (replay must reproduce the tie, not resolve it)", err)
	}
	if replayed.RunID == final.RunID {
		t.Fatalf("S52: replay reused the original run ID")
	}
	if len(replayed.Arbitration) != 1 || replayed.Arbitration[0].Decision.Reason != final.Arbitration[0].Decision.Reason {
		t.Fatalf("S52: replayed dispute = %#v, want matching reason %q", replayed.Arbitration, final.Arbitration[0].Decision.Reason)
	}
}

// S00 — explicit "2/3 agree" happy path: three reviewers, two vote accept,
// one votes reject, on a non-strict category. This is the canonical
// majority-consensus case the user's scenario request centers on, run
// fully end to end (real panel, real clustering, real vote resolution).
func TestConsensusPlaytest_S00_TwoOfThreeAgree(t *testing.T) {
	documentDir := t.TempDir()
	reviewers := []scriptedReviewer{
		{id: "one", raisesFind: true, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
		{id: "two", raisesFind: false, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
		{id: "three", raisesFind: false, voteChoice: domain.VoteReject, voteSeverity: domain.SeverityMajor},
	}
	service, packet, panel := buildE2EService(t, documentDir, reviewers)
	t.Setenv("PATH", "")
	final, err := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitBlockingFindings {
		t.Fatalf("S00: err = %v, want ExitBlockingFindings", err)
	}
	if final.Status != "findings" || len(final.Decisions) != 1 {
		t.Fatalf("S00: final = %#v", final)
	}
	decision := final.Decisions[0]
	if decision.Outcome != "accepted" || decision.Reason != "majority_accept" {
		t.Fatalf("S00: decision = %#v, want accepted/majority_accept for a 2/3 agree vote", decision)
	}
	if decision.Accepts != 2 || decision.Rejects != 1 {
		t.Fatalf("S00: Accepts/Rejects = %d/%d, want 2/1", decision.Accepts, decision.Rejects)
	}
	if len(decision.Dissent) != 1 || decision.Dissent[0].Choice != domain.VoteReject {
		t.Fatalf("S00: the dissenting reviewer's reject vote must be recorded: %#v", decision.Dissent)
	}
}

// S35 — a decision-memory match sets MemoryHint on the dispute without
// overwriting Default, and --accept-majority reads Default (the panel's own
// recommendation), not the memory hint. This guards repair D-031: an
// earlier defect let the "previous ruling" text silently overwrite Default
// and invert --accept-majority's outcome.
func TestConsensusPlaytest_S35_DecisionMemorySetsHintNotDefault(t *testing.T) {
	documentDir := t.TempDir()
	service, final := arbitrationServiceWithTie(t, documentDir)
	workspaceID := mustPacketWorkspaceID(t, service, documentDir, final.RunID)
	workspace, err := service.Store.Workspace(workspaceID, documentDir)
	if err != nil {
		t.Fatal(err)
	}
	dispute := final.Arbitration[0]
	fingerprint := domain.FindingFingerprint(dispute.Finding)
	if err := storage.AppendDecision(workspace, storage.DecisionRecord{
		SchemaVersion: domain.SchemaVersion, PacketItem: dispute.Finding.Anchor.PacketItem, Fingerprint: fingerprint,
		Ruling: "accepted", Date: service.now(), Operator: "prior-run-operator@example.com",
	}); err != nil {
		t.Fatal(err)
	}

	// Reproduce the same tie via Replay so review.go recomputes disputes
	// and consults decision memory for this run.
	replayed, err := service.Replay(context.Background(), RunRef{Input: documentDir, RunID: final.RunID})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitArbitration {
		t.Fatalf("S35 setup: replay err = %v, want ExitArbitration", err)
	}
	if len(replayed.Arbitration) != 1 {
		t.Fatalf("S35 setup: want exactly one dispute, got %#v", replayed.Arbitration)
	}
	got := replayed.Arbitration[0]
	if got.MemoryHint != "previous ruling: accepted" {
		t.Fatalf("S35: MemoryHint = %q, want the matched prior ruling", got.MemoryHint)
	}
	if !strings.Contains(got.Default, "review both arguments") && !strings.HasPrefix(got.Default, "accept") && !strings.HasPrefix(got.Default, "reject") {
		t.Fatalf("S35: Default = %q, does not look like a panel-derived recommendation", got.Default)
	}
	if strings.HasPrefix(got.Default, "previous ruling") {
		t.Fatalf("S35: Default was overwritten with the decision-memory text (regression: D-031)")
	}
}

// S36 — a legacy persisted dispute whose Default itself carries the old
// "previous ruling: X" format (predating the MemoryHint field) is still
// honored correctly by --accept-majority via the CutPrefix fallback.
func TestConsensusPlaytest_S36_LegacyDefaultFormatHonored(t *testing.T) {
	final := domain.Final{Arbitration: []domain.ArbitrationDispute{
		{ID: "A-1", Default: "previous ruling: accepted"},
		{ID: "A-2", Default: "previous ruling: rejected"},
		{ID: "A-3", Default: "previous ruling: deferred"},
	}}
	rulings, err := arbitrationRulings(ArbitrationOptions{AcceptMajority: true, Operator: "op"}, final)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]string{}
	for _, r := range rulings {
		byID[r.ID] = r.Outcome
	}
	if byID["A-1"] != "accepted" || byID["A-2"] != "rejected" || byID["A-3"] != "deferred" {
		t.Fatalf("S36: legacy-format rulings = %#v, want exact pass-through of the prior ruling", byID)
	}
}
