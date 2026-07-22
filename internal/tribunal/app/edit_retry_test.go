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

// A rolled-back apply leaves its recovery backup behind; retrying the same
// apply must reuse the identical backup instead of failing forever on
// "recovery backup already exists".
func TestEditTransactionRetriesAfterRollback(t *testing.T) {
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
	service.Clock = func() time.Time { return time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC) }
	fail := true
	service.EditFault = func(point string) error {
		if fail && point == "after-source-0" {
			return errors.New("simulated transient failure")
		}
		return nil
	}
	plan := transactionPlan{source: source, recovery: filepath.Join(runDir, "backups", "before"), before: before, after: after, mode: 0o640}
	tx, err := service.executeEditTransaction(runDir, "run", "packet", "apply", []transactionPlan{plan})
	if err == nil {
		t.Fatal("expected injected interruption")
	}
	if err := service.rollbackEditTransaction(runDir, &tx, err.Error()); err != nil {
		t.Fatal(err)
	}
	fail = false
	if _, err := service.executeEditTransaction(runDir, "run", "packet", "apply", []transactionPlan{plan}); err != nil {
		t.Fatalf("retry after rollback failed: %v", err)
	}
	live, err := os.ReadFile(source)
	if err != nil || string(live) != string(after) {
		t.Fatalf("retry left source at %q: %v", live, err)
	}
	backup, err := os.ReadFile(plan.recovery)
	if err != nil || string(backup) != string(before) {
		t.Fatalf("recovery backup = %q, %v", backup, err)
	}
}

// A backup whose content differs from the new transaction's before-state is
// archived under a content-addressed name, never destroyed.
func TestEditTransactionArchivesMismatchedBackup(t *testing.T) {
	runDir := t.TempDir()
	source := filepath.Join(t.TempDir(), "document.md")
	if err := os.WriteFile(source, []byte("current\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	canonicalSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	source = canonicalSource
	recovery := filepath.Join(runDir, "backups", "before")
	stale := []byte("stale backup content\n")
	if err := storage.WriteFileMode(recovery, stale, 0o600); err != nil {
		t.Fatal(err)
	}
	store, _ := storage.New(t.TempDir())
	service, _ := New(config.Default(), store, adapters.NewRegistry())
	service.Clock = func() time.Time { return time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC) }
	plan := transactionPlan{source: source, recovery: recovery, before: []byte("current\n"), after: []byte("edited\n"), mode: 0o640}
	if _, err := service.executeEditTransaction(runDir, "run", "packet", "apply", []transactionPlan{plan}); err != nil {
		t.Fatalf("apply with mismatched stale backup failed: %v", err)
	}
	archived := recovery + ".superseded-" + hashText(string(stale))[:24]
	preserved, err := os.ReadFile(archived)
	if err != nil || string(preserved) != string(stale) {
		t.Fatalf("stale backup not archived: %q, %v", preserved, err)
	}
	current, err := os.ReadFile(recovery)
	if err != nil || string(current) != "current\n" {
		t.Fatalf("fresh backup = %q, %v", current, err)
	}
}

// After crash recovery rolls an apply back, revert must report the honest
// condition (already rolled back) rather than accusing the user of foreign
// changes.
func TestRevertAfterCrashRollbackReportsHonestly(t *testing.T) {
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
	// Interrupt the apply mid-write, then let Revert run: recovery inside
	// Revert rolls the transaction back and the record must surface as
	// rolled back, not as user changes.
	final, reviewErr := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel})
	var exit *ExitError
	if !errors.As(reviewErr, &exit) || exit.Code != ExitBlockingFindings {
		t.Fatalf("review error = %v", reviewErr)
	}
	workspace, err := service.Store.Workspace(packet.WorkspaceID, documentDir)
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(workspace.RunsDir, final.RunID)
	source, err := filepath.EvalSymlinks(documentPath)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	record := EditRecord{SchemaVersion: 1, RunID: final.RunID, PacketHash: final.PacketHash, AppliedAt: service.now(), Files: []EditFileRecord{{SchemaVersion: 1, PacketItem: packet.Items[0].ID, SourcePath: source, BackupPath: filepath.Join(runDir, "backups", "b.original"), BeforeSHA256: hashText(string(before)), AfterSHA256: hashText("edited content\n")}}}
	if err := storage.WriteJSON(filepath.Join(runDir, "edit-record.json"), record); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash mid-apply: transaction persisted in "applying" phase,
	// source still at its before content.
	tx := editTransaction{SchemaVersion: 1, RunID: final.RunID, PacketHash: final.PacketHash, Operation: "apply", Phase: "applying", UpdatedAt: service.now(), Files: []editTransactionFile{{SchemaVersion: 1, SourcePath: source, RecoveryPath: filepath.Join(runDir, "backups", "b.original"), BeforeSHA256: hashText(string(before)), AfterSHA256: hashText("edited content\n"), Mode: 0o600}}}
	if err := storage.WriteJSON(filepath.Join(runDir, "edit-transaction.json"), tx); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteFileMode(filepath.Join(runDir, "backups", "b.original"), before, 0o600); err != nil {
		t.Fatal(err)
	}
	_, revertErr := service.Revert(RunRef{Input: documentPath, RunID: final.RunID})
	if revertErr == nil {
		t.Fatal("revert of rolled-back apply succeeded")
	}
	if strings.Contains(revertErr.Error(), "user changes") {
		t.Fatalf("revert misreported rollback as user changes: %v", revertErr)
	}
	if !strings.Contains(revertErr.Error(), "rolled back") && !strings.Contains(revertErr.Error(), "reverted") {
		t.Fatalf("revert error not honest about rollback: %v", revertErr)
	}
}

// Aborted and degraded finals must not stale ledger records: the run never
// completed its examination of any item.
func TestLedgerScopeRequiresCompletedReview(t *testing.T) {
	for status, eligible := range map[string]bool{"final": true, "findings": true, "arbitration_pending": true, "aborted": false, "degraded": false} {
		if ledgerScopeEligible(status) != eligible {
			t.Fatalf("ledgerScopeEligible(%q) = %v, want %v", status, !eligible, eligible)
		}
	}
}
