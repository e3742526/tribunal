package adapters

import (
	"context"
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

// TestGrokArgvKeepsThePromptOutOfTheArgumentList is the Tribunal side of the
// same defect tagteam fixed: a packet passed positionally is visible to every
// user on the host through `ps`.
func TestGrokArgvKeepsThePromptOutOfTheArgumentList(t *testing.T) {
	a := &Subprocess{AdapterID: "grok", Binary: "grok"}
	const secret = "CONFIDENTIAL-PACKET-BODY"
	argv, stdin, err := a.argv(RoleReviewer, domain.Panelist{Model: "grok-4.6"}, Request{RunDir: "/run", Schema: ProviderReviewSchema, TimeoutSeconds: 30}, secret)
	if err != nil {
		t.Fatalf("argv() error = %v", err)
	}
	joined := strings.Join(argv, " ")
	if strings.Contains(joined, secret) {
		t.Fatalf("grok argv exposes the prompt in the process argument list: %s", joined)
	}
	if !strings.Contains(joined, "--prompt-file /dev/stdin") {
		t.Fatalf("grok argv = %s, want the prompt streamed through /dev/stdin", joined)
	}
	if !strings.Contains(string(stdin), secret) {
		t.Fatal("grok stdin does not carry the prompt")
	}
	for _, want := range []string{"--model grok-4.6", "--output-format json", "--permission-mode dontAsk", "--no-plan", "--no-subagents", "--no-memory"} {
		if !strings.Contains(joined, want) {
			t.Errorf("grok argv is missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "--always-approve") || strings.Contains(joined, "write_file") {
		t.Errorf("grok argv grants mutation in a read-only Tribunal role: %s", joined)
	}
	// Tribunal's subprocess adapters run only on macOS and Linux, so there is
	// no positional-prompt fallback to fall into.
	if strings.Contains(joined, "--single") {
		t.Errorf("grok argv still carries the positional-prompt path: %s", joined)
	}
}

func TestGrokExtraEnvDisablesInheritedMCPServers(t *testing.T) {
	env := strings.Join((&Subprocess{AdapterID: "grok", Binary: "grok"}).extraEnv(), " ")
	for _, want := range []string{"GROK_CLAUDE_MCPS_ENABLED=false", "GROK_CURSOR_MCPS_ENABLED=false"} {
		if !strings.Contains(env, want) {
			t.Errorf("grok environment is missing %q: %s", want, env)
		}
	}
	if (&Subprocess{AdapterID: "codex", Binary: "codex"}).extraEnv() != nil {
		t.Error("only grok needs adapter-specific environment")
	}
}

func TestUnwrapGrokPrefersStructuredOutput(t *testing.T) {
	structured := unwrapGrok([]byte(`{"text":"prose","structuredOutput":{"schema_version":1}}`))
	if string(structured) != `{"schema_version":1}` {
		t.Fatalf("structured payload = %s", structured)
	}
	if got := string(unwrapGrok([]byte(`{"text":"just prose","structuredOutput":null}`))); got != "just prose" {
		t.Fatalf("text payload = %s", got)
	}
	const notAnEnvelope = "plain output"
	if got := string(unwrapGrok([]byte(notAnEnvelope))); got != notAnEnvelope {
		t.Fatalf("non-envelope payload = %s", got)
	}
}

func TestParseGrokModelList(t *testing.T) {
	models, defaultModel := parseGrokModelList([]byte("Available models:\n* grok-4.6 (default)\n- grok-4.5\nDefault model: grok-4.6\n"))
	if defaultModel != "grok-4.6" {
		t.Fatalf("default = %q", defaultModel)
	}
	if len(models) != 2 || models[0] != "grok-4.6" || models[1] != "grok-4.5" {
		t.Fatalf("models = %#v", models)
	}
}

func TestParseTabularModelListSkipsProse(t *testing.T) {
	models := parseTabularModelList([]byte("Models:\ngemini-3.8-flash-medium\tGemini 3.8 Flash (Medium)\ngemini-3.8-flash-high\tGemini 3.8 Flash (High)\n"))
	if len(models) != 2 || models[0] != "gemini-3.8-flash-medium" {
		t.Fatalf("models = %#v", models)
	}
}

// TestDiscoverModelsPreservesFallbacksAndWarnings pins the behavior that
// makes the catalog trustworthy offline: an unreachable provider keeps its
// roster entry and gains a visible warning, and it never renders as a
// provider with no models.
func TestDiscoverModelsPreservesFallbacksAndWarnings(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	registry := NewRegistry(
		&Subprocess{AdapterID: "codex", Binary: "codex"},
		&Subprocess{AdapterID: "claude", Binary: "claude"},
		&Subprocess{AdapterID: "agy", Binary: "agy"},
		&Subprocess{AdapterID: "grok", Binary: "grok"},
		&OpenAICompatible{Model: "gemma4:latest"},
		&MistralAcp{Binary: "tribunal-test-nonexistent-vibe-acp"},
	)
	entries := registry.DiscoverModels(context.Background(), t.TempDir(), map[string]string{"openai-compatible": "gemma4:latest"})
	byAdapter := map[string]ModelCatalogEntry{}
	for _, entry := range entries {
		byAdapter[entry.Adapter] = entry
	}
	if len(entries) != 6 {
		t.Fatalf("catalog covered %d adapters, want 6", len(entries))
	}
	for _, adapter := range []string{"codex", "claude", "agy", "grok"} {
		entry := byAdapter[adapter]
		if entry.Source != "maintained" || len(entry.Models) == 0 {
			t.Errorf("%s = source %q, models %#v; want the vendored roster as fallback", adapter, entry.Source, entry.Models)
		}
	}
	for _, adapter := range []string{"agy", "grok", "mistral-acp"} {
		if byAdapter[adapter].Error == "" {
			t.Errorf("%s discovery failure was swallowed instead of reported", adapter)
		}
	}
	// Codex and Claude have no model-list command; that is not a failure and
	// must not be reported as one.
	for _, adapter := range []string{"codex", "claude"} {
		if byAdapter[adapter].Error != "" {
			t.Errorf("%s reported a warning for simply having no model-list command: %s", adapter, byAdapter[adapter].Error)
		}
	}
	if entry := byAdapter["openai-compatible"]; entry.Source != "config" || entry.Default != "gemma4:latest" {
		t.Errorf("openai-compatible = %#v; want the configured model with a config source", entry)
	}
	if entry := byAdapter["mistral-acp"]; entry.Source != "config" || len(entry.Models) != 0 {
		t.Errorf("mistral-acp = %#v; its models come from live ACP discovery only", entry)
	}
}
