package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/config"
	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

// L-02 regression (visibility half): a run that quarantines findings must
// say so — a findings_quarantined reason code and a summary that does not
// read as an unqualified clean outcome. The alias fix removes the observed
// cause, so this fixture forces quarantine via an unresolvable quote.
func TestQuarantinedFindingsSurfaceInReasonsAndSummary(t *testing.T) {
	documentPath := filepath.Join(t.TempDir(), "brief.md")
	if err := os.WriteFile(documentPath, []byte("# Brief\n\nThe launch date is unsupported.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rubric, _ := config.BuiltinRubric("generic")
	packet, err := documents.Build(context.Background(), documentPath, documents.BuildOptions{Kind: "generic", Rubric: rubric})
	if err != nil {
		t.Fatal(err)
	}
	bad := domain.Finding{SchemaVersion: domain.FindingSchemaVersion, ID: "F-BAD", Reviewer: "R-001", Origin: "panel", Severity: domain.SeverityMajor, Category: domain.CategoryCorrectness, Anchor: domain.Anchor{Kind: "quote", PacketItem: packet.Items[0].ID, Quote: "text that does not exist anywhere", ItemSHA256: packet.Items[0].PacketSHA256}, Issue: "i", Recommendation: "r", EvidenceStatus: domain.EvidenceAnchored, Confidence: "high"}
	results := []panelResult{{panelist: domain.Panelist{ID: "R-001"}, review: domain.Review{SchemaVersion: 1, ReviewerID: "R-001", Findings: []domain.Finding{bad}}, status: domain.PanelStatus{ReviewerID: "R-001", Status: "ok"}}}
	_, findings, _, reasons := validatePass(packet, results)
	if len(findings) != 1 || !findings[0].Quarantined {
		t.Fatalf("fixture finding was not quarantined: %#v", findings)
	}
	if !strings.Contains(strings.Join(reasons, ","), "findings_quarantined") {
		t.Fatalf("reasons = %v, want findings_quarantined", reasons)
	}
	summary := summaryFor(nil, nil, findings)
	if !strings.Contains(summary, "quarantined before voting") {
		t.Fatalf("summary hides quarantine: %q", summary)
	}
	if unaffected := summaryFor(nil, nil, nil); strings.Contains(unaffected, "quarantined") {
		t.Fatalf("clean summary mentions quarantine: %q", unaffected)
	}
}
