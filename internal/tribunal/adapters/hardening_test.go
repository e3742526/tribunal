package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

// agy takes the prompt as one argv element, so oversized prompts must fail
// closed with an actionable message instead of exec's E2BIG.
func TestAgyRejectsOversizedPromptBeforeExec(t *testing.T) {
	adapter := &Subprocess{AdapterID: "agy", Binary: "agy"}
	prompt := strings.Repeat("a", agyMaxPromptBytes+1)
	if _, _, err := adapter.argv(RoleReviewer, domain.Panelist{Model: "m"}, Request{}, prompt); err == nil || !strings.Contains(err.Error(), "different adapter") {
		t.Fatalf("oversized agy prompt error = %v", err)
	}
	if _, _, err := adapter.argv(RoleReviewer, domain.Panelist{Model: "m"}, Request{}, "small prompt"); err != nil {
		t.Fatalf("small agy prompt rejected: %v", err)
	}
}

// The subprocess environment is an allowlist and must never carry secrets:
// the OpenAI-compatible key is consumed in-process, not by provider CLIs.
func TestRestrictedEnvNeverExportsSecrets(t *testing.T) {
	t.Setenv("TRIBUNAL_TEST_API_KEY", "sk-super-secret")
	t.Setenv("HOME", "/tmp/home")
	env := restrictedEnv()
	for _, pair := range env {
		if strings.Contains(pair, "sk-super-secret") || strings.HasPrefix(pair, "TRIBUNAL_TEST_API_KEY=") {
			t.Fatalf("secret exported to subprocess env: %s", pair)
		}
	}
	found := false
	for _, pair := range env {
		if pair == "HOME=/tmp/home" {
			found = true
		}
	}
	if !found {
		t.Fatal("allowlisted HOME missing from subprocess env")
	}
}

func TestOpenAICompatibleReportsEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()
	adapter := &OpenAICompatible{BaseURL: server.URL, Client: server.Client()}
	_, err := adapter.Invoke(context.Background(), RoleReviewer, domain.Panelist{Model: "test"}, Request{Prompt: "p", MaxOutputBytes: 1024})
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("empty choices error = %v", err)
	}
}

func TestOpenAICompatibleGuardsInjectedClientRedirects(t *testing.T) {
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("cross-origin redirect was followed")
	}))
	defer attacker.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	adapter := &OpenAICompatible{BaseURL: server.URL, Client: &http.Client{}}
	if _, err := adapter.Invoke(context.Background(), RoleReviewer, domain.Panelist{Model: "test"}, Request{Prompt: "p", MaxOutputBytes: 1024}); err == nil {
		t.Fatal("cross-origin redirect on injected client did not fail")
	}
}

func TestSpellcheckCapsFindings(t *testing.T) {
	words := make([]string, 0, maxWorkerFindings+50)
	for i := 0; i < maxWorkerFindings+50; i++ {
		words = append(words, "recieve")
	}
	packet := documents.Packet{Items: []documents.Item{{ID: "artifact:x.md", PacketSHA256: "s", Content: strings.Join(words, " ")}}}
	findings := Spellcheck(packet)
	if len(findings) == 0 || len(findings) > maxWorkerFindings {
		t.Fatalf("spellcheck findings = %d, want 1..%d", len(findings), maxWorkerFindings)
	}
}
