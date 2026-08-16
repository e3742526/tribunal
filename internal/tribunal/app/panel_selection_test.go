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

func policyConfig() config.Config {
	cfg := config.Default()
	cfg.PanelPolicy = "balanced"
	cfg.Models = []domain.ModelCandidate{
		{ID: "alpha", Adapter: "fake", Model: "alpha-1", Family: "alpha-labs", Capabilities: []string{"document-analysis", "reasoning"}, Quality: 0.9, Reliability: 0.9, Cost: 1},
		{ID: "beta", Adapter: "fake", Model: "beta-1", Family: "beta-labs", Capabilities: []string{"literal-reading"}, Quality: 0.6, Reliability: 0.8, Cost: 0.2},
		{ID: "gamma", Adapter: "fake", Model: "gamma-1", Family: "gamma-labs", Capabilities: []string{"contradiction-detection"}, Quality: 0.7, Reliability: 0.85, Cost: 0.5},
		{ID: "alpha-two", Adapter: "fake", Model: "alpha-2", Family: "alpha-labs", Capabilities: []string{"reasoning"}, Quality: 0.95, Reliability: 0.95, Cost: 0.9},
	}
	return cfg
}

func policyService(t *testing.T, cfg config.Config) *Service {
	t.Helper()
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(cfg, store, adapters.NewRegistry(&adapters.FuncAdapter{AdapterID: "fake"}))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestPolicyComposedPanelAppliesPersonasAndConfiguredContextBudget(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	service := policyService(t, policyConfig())
	panel, selection, err := service.resolvePanelWithSelection(ReviewOptions{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if selection == nil || selection.Policy != "balanced" {
		t.Fatalf("expected a balanced-policy selection, got %#v", selection)
	}
	if len(panel.Reviewers) != 3 {
		t.Fatalf("expected three seats, got %d", len(panel.Reviewers))
	}
	personas := map[string]bool{}
	for _, reviewer := range panel.Reviewers {
		personas[reviewer.Persona] = true
		if reviewer.MaxContextTokens != service.Config.Limits.MaxContextTokens || reviewer.ReservedOutputTokens != service.Config.Limits.ReservedOutput {
			t.Fatalf("catalog entry escaped the configured context budget: %+v", reviewer)
		}
		// A policy seat is only a real lens if the persona text reached the
		// panelist; an unhydrated seat would silently review as "plain".
		if reviewer.Persona != "plain" && reviewer.PersonaLens == "" {
			t.Fatalf("persona %q was not hydrated for %s", reviewer.Persona, reviewer.ID)
		}
	}
	if !personas["foundation"] {
		t.Fatalf("balanced policy must seat the foundation lens, got %v", personas)
	}
}

func TestPolicyPanelRefusesToSeatOneFamilyTwice(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	service := policyService(t, policyConfig())
	panel, _, err := service.resolvePanelWithSelection(ReviewOptions{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	families := map[string]int{}
	for _, reviewer := range panel.Reviewers {
		families[reviewer.Family]++
	}
	// alpha-two outscores every other candidate on raw quality; seating it
	// beside alpha would give the panel two correlated alpha-labs samples.
	if families["alpha-labs"] > 1 {
		t.Fatalf("independence constraint ignored: %v", families)
	}
}

func TestExplicitPanelOverridesConfiguredPolicy(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	service := policyService(t, policyConfig())
	panel, selection, err := service.resolvePanelWithSelection(ReviewOptions{Panel: "fake/one,fake/two"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if selection != nil {
		t.Fatalf("an explicit panel must not be recomposed by a policy: %#v", selection)
	}
	if len(panel.Reviewers) != 2 || panel.Reviewers[0].Model != "one" {
		t.Fatalf("unexpected panel %#v", panel.Reviewers)
	}
}

func TestReplayedPanelValueIgnoresPolicy(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	service := policyService(t, policyConfig())
	recorded, err := domain.ParsePanel("fake/recorded-a,fake/recorded-b")
	if err != nil {
		t.Fatal(err)
	}
	for i := range recorded.Reviewers {
		recorded.Reviewers[i].MaxContextTokens = service.Config.Limits.MaxContextTokens
		recorded.Reviewers[i].ReservedOutputTokens = service.Config.Limits.ReservedOutput
	}
	// A replay reruns a frozen packet. If a catalog edit could repanel it,
	// the replay would no longer be a replay.
	panel, selection, err := service.resolvePanelWithSelection(ReviewOptions{PanelValue: &recorded})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if selection != nil || len(panel.Reviewers) != 2 || panel.Reviewers[0].Model != "recorded-a" {
		t.Fatalf("recorded panel was not preserved: %#v %#v", panel.Reviewers, selection)
	}
}

func TestInfeasiblePolicyFailsWithInvalidArgumentsBeforeAnyCall(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := policyConfig()
	cfg.Models = cfg.Models[:1]
	service := policyService(t, cfg)
	_, _, err := service.resolvePanelWithSelection(ReviewOptions{})
	if err == nil || !strings.Contains(err.Error(), "catalog offers") {
		t.Fatalf("expected an unsatisfiable-policy error, got %v", err)
	}
	documentDir := t.TempDir()
	documentPath := filepath.Join(documentDir, "brief.md")
	if err := os.WriteFile(documentPath, []byte("# Brief\n\nA claim.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, reviewErr := service.Review(context.Background(), ReviewOptions{Input: documentPath})
	var exit *ExitError
	if !errors.As(reviewErr, &exit) || exit.Code != ExitInvalidArguments {
		t.Fatalf("Review error = %v, want exit %d", reviewErr, ExitInvalidArguments)
	}
}

func TestPanelPreviewReportsPolicySourceWithoutRunningAReview(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	service := policyService(t, policyConfig())
	preview, err := service.PanelPreview(ReviewOptions{})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Source != "panel policy" || preview.Policy != "balanced" || preview.Selection == nil {
		t.Fatalf("unexpected preview %#v", preview)
	}
	if len(preview.Selection.Seats) != len(preview.Panel.Reviewers) {
		t.Fatalf("every seated reviewer needs a rationale: %d seats, %d reviewers", len(preview.Selection.Seats), len(preview.Panel.Reviewers))
	}
	stringPanel, err := service.PanelPreview(ReviewOptions{Panel: "fake/one,fake/two"})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if stringPanel.Source != "panel string" || stringPanel.Selection != nil {
		t.Fatalf("unexpected string-panel preview %#v", stringPanel)
	}
}

func TestPolicyRunPersistsSelectionInMetaAndReport(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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
	stateRoot := t.TempDir()
	store, err := storage.New(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	store.Clock = func() time.Time { return clock }
	fake := &adapters.FuncAdapter{AdapterID: "fake"}
	fake.InvokeFn = func(_ context.Context, role adapters.Role, panelist domain.Panelist, _ adapters.Request) (adapters.Response, error) {
		switch role {
		case adapters.RoleReviewer:
			finding := domain.Finding{
				SchemaVersion: domain.FindingSchemaVersion, ID: "F-" + panelist.ID, Reviewer: panelist.ID, Origin: "panel",
				Severity: domain.SeverityMinor, Category: domain.CategoryCorrectness,
				Anchor: domain.Anchor{Kind: "quote", PacketItem: packet.Items[0].ID, Quote: "launch date is unsupported", ItemSHA256: packet.Items[0].PacketSHA256},
				Issue:  "The launch date has no support.", Recommendation: "Cite a source.",
				EvidenceStatus: domain.EvidenceAnchored, Confidence: "high",
			}
			return jsonResponse(t, domain.Review{SchemaVersion: 1, ReviewerID: panelist.ID, Findings: []domain.Finding{finding}}), nil
		case adapters.RoleVoter:
			payload := map[string]any{"schema_version": 1, "votes": []domain.Vote{{SchemaVersion: 1, ReviewerID: panelist.ID, FindingID: "B-0001", Choice: domain.VoteAccept, Severity: domain.SeverityMinor, Reason: "Unsupported."}}}
			return jsonResponse(t, payload), nil
		default:
			return adapters.Response{}, errors.New("unexpected role")
		}
	}
	service, err := New(policyConfig(), store, adapters.NewRegistry(fake))
	if err != nil {
		t.Fatal(err)
	}
	service.Clock = func() time.Time { return clock }
	t.Setenv("PATH", "")
	final, _ := service.Review(context.Background(), ReviewOptions{Packet: &packet, NoWorkers: true})
	if final.RunID == "" {
		t.Fatalf("policy-composed review produced no run")
	}
	workspace, err := store.Workspace(packet.WorkspaceID, documentDir)
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(workspace.RunsDir, final.RunID)
	raw, err := os.ReadFile(filepath.Join(runDir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.PanelSelection == nil || meta.PanelSelection.Policy != "balanced" {
		t.Fatalf("meta.json lost the panel selection: %s", raw)
	}
	if len(meta.PanelSelection.Seats) != len(meta.Panel.Reviewers) {
		t.Fatalf("selection seats do not match the recorded panel: %+v", meta.PanelSelection)
	}
	report, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(report), "Composed by panel policy `balanced`") {
		t.Fatalf("report.md omitted the composing policy:\n%s", report)
	}
}
