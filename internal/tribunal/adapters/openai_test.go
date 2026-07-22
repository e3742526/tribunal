package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func TestOpenAICompatibleSendsOutputTokenCap(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`))
	}))
	defer server.Close()
	adapter := &OpenAICompatible{BaseURL: server.URL, Client: server.Client()}
	response, err := adapter.Invoke(context.Background(), RoleReviewer, domain.Panelist{Model: "test"}, Request{Prompt: "prompt", MaxOutputTokens: 77, MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if payload["max_tokens"] != float64(77) || response.InputTok != 2 || response.OutputTok != 1 {
		t.Fatalf("payload=%#v response=%#v", payload, response)
	}
}
