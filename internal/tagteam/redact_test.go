package tagteam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedactSecrets_ReplacesSensitiveEnvValues(t *testing.T) {
	t.Setenv("FEATHERLESS_API_KEY", "secret-token")
	t.Setenv("OPENROUTER_API_KEY", "another-secret")

	got := redactSecrets("Bearer secret-token and another-secret should not print")
	if strings.Contains(got, "secret-token") || strings.Contains(got, "another-secret") {
		t.Fatalf("redacted text still contains secret: %q", got)
	}
	if count := strings.Count(got, redactedSecret); count != 2 {
		t.Fatalf("redacted count = %d, want 2: %q", count, got)
	}
}

func TestOpenAICompatibleRunDirectRedactsErrorBody(t *testing.T) {
	t.Setenv("FEATHERLESS_API_KEY", "secret-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key secret-token", http.StatusUnauthorized)
	}))
	defer server.Close()

	adapter := &OpenAICompatibleAdapter{
		BaseURL:      server.URL,
		APIKeyEnv:    "FEATHERLESS_API_KEY",
		DefaultModel: "openai/gpt-oss-120b",
	}
	_, err := adapter.RunDirect(RoleAdversary, Request{
		Context: context.Background(),
		Prompt:  "review this",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error leaked secret: %v", err)
	}
	if !strings.Contains(err.Error(), redactedSecret) {
		t.Fatalf("error did not show redaction marker: %v", err)
	}
}
