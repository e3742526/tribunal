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

func TestEditAppliesAcceptedScopeAndRevertProtectsUserChanges(t *testing.T) {
	documentDir := t.TempDir()
	documentPath := filepath.Join(documentDir, "brief.md")
	original := "# Brief\n\nThe date is unsupported.\n"
	if err := os.WriteFile(documentPath, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	rubric, _ := config.BuiltinRubric("generic")
	packet, err := documents.Build(context.Background(), documentPath, documents.BuildOptions{Kind: "generic", Rubric: rubric})
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(original, "unsupported")
	finding := domain.Finding{SchemaVersion: 2, ID: "F-1", Reviewer: "R-001", Origin: "panel", Severity: domain.SeverityMajor, Category: domain.CategoryCorrectness, Anchor: domain.Anchor{Kind: "quote", PacketItem: packet.Items[0].ID, Quote: "unsupported", ItemSHA256: packet.Items[0].PacketSHA256, CharOffset: start, EndOffset: start + len("unsupported")}, Issue: "The date lacks support.", Recommendation: "Qualify the statement.", EvidenceStatus: domain.EvidenceAnchored, Confidence: "high"}
	decision := domain.Decision{SchemaVersion: 1, FindingID: finding.ID, Outcome: "accepted", Severity: domain.SeverityMajor, Accepts: 3, Configured: 3, Valid: 3}
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.Workspace(packet.WorkspaceID, documentDir)
	if err != nil {
		t.Fatal(err)
	}
	runID, runDir, err := store.CreateRun(workspace)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 21, 13, 0, 0, 0, time.UTC)
	final := domain.Final{SchemaVersion: 1, RunID: runID, WorkspaceID: packet.WorkspaceID, PacketHash: packet.PacketHash, Status: "findings", ExitCode: 1, Findings: []domain.Finding{finding}, Decisions: []domain.Decision{decision}, StartedAt: now, FinishedAt: now}
	panel, err := domain.ParsePanel("fake/editor")
	if err != nil {
		t.Fatal(err)
	}
	for path, value := range map[string]any{
		filepath.Join(runDir, "packet.json"):         packet,
		filepath.Join(runDir, "meta.json"):           Meta{SchemaVersion: 1, RunID: runID, WorkspaceID: packet.WorkspaceID, InputRoot: packet.InputRoot, PacketHash: packet.PacketHash, Panel: panel, StartedAt: now},
		filepath.Join(workspace.Root, "latest.json"): map[string]any{"schema_version": 1, "run_id": runID},
	} {
		if err := storage.WriteJSON(path, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := storage.PersistTerminal(runDir, final); err != nil {
		t.Fatal(err)
	}
	proposal := domain.EditProposal{SchemaVersion: 1, RunID: runID, PacketHash: packet.PacketHash, Hunks: []domain.EditHunk{{PacketItem: packet.Items[0].ID, FindingIDs: []string{finding.ID}, Scope: domain.EditLocal, SourceSHA256: packet.Items[0].SourceSHA256, Start: start, End: start + len("unsupported"), Replacement: "not yet verified"}}}
	proposalPath := filepath.Join(t.TempDir(), "proposal.json")
	if err := storage.WriteJSON(proposalPath, proposal); err != nil {
		t.Fatal(err)
	}
	service, err := New(config.Default(), store, adapters.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	service.Clock = func() time.Time { return now }
	result, err := service.Edit(context.Background(), EditOptions{RunRef: RunRef{Input: documentPath, RunID: runID}, ProposalPath: proposalPath, Apply: true})
	if err != nil || !result.Applied {
		t.Fatalf("Edit() = %#v, %v", result, err)
	}
	edited, _ := os.ReadFile(documentPath)
	if !strings.Contains(string(edited), "not yet verified") {
		t.Fatalf("edit was not applied: %q", edited)
	}
	if err := os.WriteFile(documentPath, append(edited, []byte("User note.\n")...), 0o640); err != nil {
		t.Fatal(err)
	}
	_, err = service.Revert(RunRef{Input: documentPath, RunID: runID})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitAborted {
		t.Fatalf("Revert after user change = %v, want abort", err)
	}
	if err := os.WriteFile(documentPath, edited, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Revert(RunRef{Input: documentPath, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(documentPath)
	if string(restored) != original {
		t.Fatalf("revert restored %q, want %q", restored, original)
	}
}

func TestValidateEditRejectsOutOfScopeAndStaleSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "long.md")
	content := "claim" + strings.Repeat("x", 900) + "tail"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	packet, err := documents.Build(context.Background(), path, documents.BuildOptions{Kind: "generic", Rubric: "rubric"})
	if err != nil {
		t.Fatal(err)
	}
	finding := domain.Finding{SchemaVersion: 2, ID: "F-1", Reviewer: "R-001", Origin: "panel", Severity: domain.SeverityMinor, Category: domain.CategoryCorrectness, Anchor: domain.Anchor{Kind: "quote", PacketItem: packet.Items[0].ID, Quote: "claim", ItemSHA256: packet.Items[0].PacketSHA256, CharOffset: 0, EndOffset: 5}, Issue: "issue", Recommendation: "recommend", EvidenceStatus: domain.EvidenceAnchored, Confidence: "high"}
	final := domain.Final{SchemaVersion: 1, RunID: "run", PacketHash: packet.PacketHash, Findings: []domain.Finding{finding}, Decisions: []domain.Decision{{SchemaVersion: 1, FindingID: finding.ID, Outcome: "accepted"}}}
	proposal := domain.EditProposal{SchemaVersion: 1, RunID: "run", PacketHash: packet.PacketHash, Hunks: []domain.EditHunk{{PacketItem: packet.Items[0].ID, FindingIDs: []string{finding.ID}, Scope: domain.EditLocal, SourceSHA256: packet.Items[0].SourceSHA256, Start: 900, End: 904, Replacement: "end"}}}
	if _, err := validateEditProposal(packet, final, proposal, false); err == nil {
		t.Fatal("expected local-scope rejection")
	}
	proposal.Hunks[0].Start, proposal.Hunks[0].End = 0, 5
	if err := os.WriteFile(path, []byte("user changed it"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateEditProposal(packet, final, proposal, false); err == nil {
		t.Fatal("expected stale source rejection")
	}
}

func TestEditTransactionRecoversInterruptedMutation(t *testing.T) {
	runDir := t.TempDir()
	source := filepath.Join(t.TempDir(), "document.md")
	before, after := []byte("before\n"), []byte("after\n")
	if err := os.WriteFile(source, before, 0o640); err != nil {
		t.Fatal(err)
	}
	canonicalSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	source = canonicalSource
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(config.Default(), store, adapters.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	service.Clock = func() time.Time { return time.Date(2026, 7, 21, 14, 0, 0, 0, time.UTC) }
	service.EditFault = func(point string) error {
		if point == "after-source-0" {
			return errors.New("simulated process stop")
		}
		return nil
	}
	plan := transactionPlan{source: source, recovery: filepath.Join(runDir, "backups", "before"), before: before, after: after, mode: 0o640}
	if _, err := service.executeEditTransaction(runDir, "run", "packet", "apply", []transactionPlan{plan}); err == nil {
		t.Fatal("expected injected interruption")
	}
	// A fresh service instance represents restart: it has no in-memory progress,
	// and must infer the partial write from durable hashes.
	restarted, err := New(config.Default(), store, adapters.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	restarted.Clock = service.Clock
	if err := restarted.recoverEditTransaction(runDir, domain.Final{EditsApplied: false}); err != nil {
		t.Fatal(err)
	}
	live, err := os.ReadFile(source)
	if err != nil || string(live) != string(before) {
		t.Fatalf("recovery left %q: %v", live, err)
	}
	tx, err := loadEditTransaction(runDir)
	if err != nil || tx.Phase != "rolled_back" {
		t.Fatalf("transaction = %#v, %v", tx, err)
	}
}

func TestEditTransactionCommitsOnlyAgainstMatchingTerminalState(t *testing.T) {
	runDir := t.TempDir()
	source := filepath.Join(t.TempDir(), "document.md")
	before, after := []byte("before\n"), []byte("after\n")
	if err := os.WriteFile(source, before, 0o600); err != nil {
		t.Fatal(err)
	}
	canonicalSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	source = canonicalSource
	store, _ := storage.New(t.TempDir())
	service, _ := New(config.Default(), store, adapters.NewRegistry())
	service.Clock = func() time.Time { return time.Date(2026, 7, 21, 14, 0, 0, 0, time.UTC) }
	plan := transactionPlan{source: source, recovery: filepath.Join(runDir, "backups", "before"), before: before, after: after, mode: 0o600}
	if _, err := service.executeEditTransaction(runDir, "run", "packet", "apply", []transactionPlan{plan}); err != nil {
		t.Fatal(err)
	}
	if err := service.recoverEditTransaction(runDir, domain.Final{EditsApplied: true}); err != nil {
		t.Fatal(err)
	}
	tx, err := loadEditTransaction(runDir)
	if err != nil || tx.Phase != "committed" {
		t.Fatalf("transaction = %#v, %v", tx, err)
	}
	live, _ := os.ReadFile(source)
	if string(live) != string(after) {
		t.Fatalf("commit recovery changed source to %q", live)
	}
}
