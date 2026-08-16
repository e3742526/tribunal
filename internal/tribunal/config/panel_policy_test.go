package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

const catalogConfig = `schema_version = 1
panel_policy = "balanced"

[[models]]
id = "opus"
adapter = "claude"
model = "claude-opus-5"
family = "anthropic"
capabilities = ["reasoning", "document-analysis"]
quality = 0.9
reliability = 0.9
cost = 1.0

[[models]]
id = "sol"
adapter = "codex"
model = "gpt-5.6-sol"
family = "openai"
capabilities = ["contradiction-detection", "reasoning"]
quality = 0.88
reliability = 0.85
cost = 0.9

[[models]]
id = "mistral"
adapter = "openai-compatible"
model = "mistral-large"
family = "mistral"
capabilities = ["literal-reading"]
quality = 0.6
reliability = 0.75
cost = 0.1
`

func writeUserConfig(t *testing.T, content string) {
	t.Helper()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	dir := filepath.Join(configHome, "tribunal")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogAndPolicyLoadFromTrustedConfig(t *testing.T) {
	writeUserConfig(t, catalogConfig)
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.PanelPolicy != "balanced" || len(cfg.Models) != 3 {
		t.Fatalf("catalog did not load: policy=%q models=%d", cfg.PanelPolicy, len(cfg.Models))
	}
	catalog, notes, err := PanelCatalog(cfg)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(catalog) != 3 || len(notes) != 0 {
		t.Fatalf("a configured catalog must be used verbatim: %d entries, notes %v", len(catalog), notes)
	}
	policy, err := ResolvePanelPolicy(cfg, "balanced")
	if err != nil {
		t.Fatalf("resolve policy: %v", err)
	}
	selection, err := domain.SelectPanel(policy, catalog)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(selection.Families) != 3 {
		t.Fatalf("expected three independent families, got %v", selection.Families)
	}
}

func TestPanelCatalogFallsBackToPanelWithDisclosedNeutralPriors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	catalog, notes, err := PanelCatalog(cfg)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(catalog) != 3 {
		t.Fatalf("expected the default panel's three members, got %d", len(catalog))
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "neutral quality") {
		t.Fatalf("a derived catalog must disclose its neutral priors, notes=%v", notes)
	}
	for _, candidate := range catalog {
		if err := domain.ValidateCandidate(candidate); err != nil {
			t.Fatalf("derived candidate %q is invalid: %v", candidate.ID, err)
		}
		// Deriving priors is honest only if they are uniform; a derived
		// catalog must never assert one vendor is better than another.
		if candidate.Quality != 0.5 || candidate.Reliability != 1 || candidate.Cost != 1 {
			t.Fatalf("derived candidate %q carries invented priors: %+v", candidate.ID, candidate)
		}
	}
}

func TestPanelCatalogSlugifiesVerbatimModelNames(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(LoadOptions{ExplicitPanel: "agy/Gemini 3.5 Flash (Medium),claude/claude-opus-5"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	catalog, _, err := PanelCatalog(cfg)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if catalog[0].ID != "agy-gemini-3-5-flash-medium" {
		t.Fatalf("unexpected slug %q", catalog[0].ID)
	}
	if catalog[0].Model != "Gemini 3.5 Flash (Medium)" {
		t.Fatalf("slugifying the id must not rewrite the model: %q", catalog[0].Model)
	}
}

func TestUniqueSlugDisambiguatesCollisions(t *testing.T) {
	used := map[string]struct{}{}
	first := uniqueSlug("claude/model one", used)
	second := uniqueSlug("claude/model!one", used)
	if first != "claude-model-one" || second != "claude-model-one-2" || first == second {
		t.Fatalf("collision not disambiguated: %q %q", first, second)
	}
}

func TestConfigRejectsInvalidCatalogEntry(t *testing.T) {
	writeUserConfig(t, "schema_version = 1\n\n[[models]]\nid = \"opus\"\nadapter = \"claude\"\nmodel = \"claude-opus-5\"\nquality = 4.0\n")
	if _, err := Load(LoadOptions{}); err == nil || !strings.Contains(err.Error(), "within 0..1") {
		t.Fatalf("expected an invalid-prior rejection, got %v", err)
	}
}

func TestConfigRejectsUnknownPanelPolicy(t *testing.T) {
	writeUserConfig(t, "schema_version = 1\npanel_policy = \"nonexistent\"\n")
	if _, err := Load(LoadOptions{}); err == nil || !strings.Contains(err.Error(), "unknown panel policy") {
		t.Fatalf("expected an unknown-policy rejection, got %v", err)
	}
}

func TestConfigPolicyShadowsBuiltinOfSameName(t *testing.T) {
	writeUserConfig(t, `schema_version = 1

[[policies]]
schema_version = 1
name = "balanced"
minimum_panel = 2
independent_families = 2
diversity_weight = 0.1
reliability_weight = 0.1
cost_weight = 0.1

[[policies.roles]]
name = "only-reviewer"

[[policies.roles]]
name = "second-reviewer"
`)
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	policy, err := ResolvePanelPolicy(cfg, "balanced")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if policy.MinimumPanel != 2 || policy.Roles[0].Name != "only-reviewer" {
		t.Fatalf("trusted config must shadow the built-in policy: %+v", policy)
	}
}

func TestExplicitPanelFlagClearsConfiguredPolicy(t *testing.T) {
	writeUserConfig(t, catalogConfig)
	cfg, err := Load(LoadOptions{ExplicitPanel: "claude/claude-opus-5,codex/gpt-5.6-sol"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.PanelPolicy != "" {
		t.Fatalf("an explicit panel must clear the configured policy, got %q", cfg.PanelPolicy)
	}
}

func TestPanelEnvironmentClearsPolicyAndConflictIsRejected(t *testing.T) {
	writeUserConfig(t, catalogConfig)
	t.Setenv("TRIBUNAL_PANEL", "claude/claude-opus-5,codex/gpt-5.6-sol")
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.PanelPolicy != "" {
		t.Fatalf("TRIBUNAL_PANEL must clear the configured policy, got %q", cfg.PanelPolicy)
	}
	t.Setenv("TRIBUNAL_PANEL_POLICY", "frugal")
	if _, err := Load(LoadOptions{}); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected a mutually-exclusive environment rejection, got %v", err)
	}
}

func TestFoundationPersonaResolvesAndPassesLint(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	persona, err := ResolvePersona("foundation", "", false)
	if err != nil {
		t.Fatalf("resolve foundation persona: %v", err)
	}
	if err := LintPersona(persona, false); err != nil {
		t.Fatalf("the built-in foundation persona must satisfy its own lint: %v", err)
	}
	text := PersonaText(persona)
	if !strings.Contains(text, "cannot be established") || !strings.Contains(text, "do not repair") {
		t.Fatalf("foundation lens lost its refusal-to-reconstruct mandate: %q", text)
	}
}

func TestStarterPoliciesAreValidAndReferenceKnownPersonas(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, policy := range StarterPolicies() {
		if err := domain.ValidatePolicy(policy); err != nil {
			t.Fatalf("built-in policy %q is invalid: %v", policy.Name, err)
		}
		for _, role := range policy.Roles {
			if role.Persona == "" || role.Persona == "plain" {
				continue
			}
			// A policy that names a persona no reviewer can resolve would
			// fail only at review time, after the packet is frozen.
			if _, err := ResolvePersona(role.Persona, "", false); err != nil {
				t.Fatalf("policy %q role %q names unresolvable persona: %v", policy.Name, role.Name, err)
			}
		}
	}
}
