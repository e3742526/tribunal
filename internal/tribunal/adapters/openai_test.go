package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func TestOpenAICompatibleDetectRejectsMissingConfiguredModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q", r.URL.Path)
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gemma4:26b"}]}`))
	}))
	defer server.Close()

	adapter := &OpenAICompatible{BaseURL: server.URL + "/v1", Model: "gemma4:latest", Client: server.Client()}
	info := adapter.Detect(context.Background())
	if !info.Found || info.Runnable {
		t.Fatalf("info = %#v", info)
	}
	if !strings.Contains(info.Hint, `model "gemma4:latest"`) {
		t.Fatalf("hint = %q", info.Hint)
	}
}

func TestOpenAICompatibleDetectAcceptsAdvertisedConfiguredModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gemma4:26b"}]}`))
	}))
	defer server.Close()

	adapter := &OpenAICompatible{BaseURL: server.URL, Model: "gemma4:26b", Client: server.Client()}
	info := adapter.Detect(context.Background())
	if !info.Found || !info.Runnable {
		t.Fatalf("info = %#v", info)
	}
}

func TestOpenAICompatibleDetectAllowsProviderWithoutModelCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer server.Close()

	adapter := &OpenAICompatible{BaseURL: server.URL, Model: "provider-model", Client: server.Client()}
	info := adapter.Detect(context.Background())
	if !info.Found || !info.Runnable {
		t.Fatalf("info = %#v", info)
	}
}

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

func TestOpenAICompatibleRejectsEmptyContentAndPreservesEnvelope(t *testing.T) {
	envelope := []byte(`{"choices":[{"message":{"content":"  \n\t"}}],"usage":{"prompt_tokens":7,"completion_tokens":2}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(envelope)
	}))
	defer server.Close()

	adapter := &OpenAICompatible{BaseURL: server.URL, Client: server.Client()}
	response, err := adapter.Invoke(context.Background(), RoleReviewer, domain.Panelist{Model: "test"}, Request{Prompt: "prompt", MaxOutputBytes: 1024})
	if err == nil || !strings.Contains(err.Error(), "empty message content") {
		t.Fatalf("error = %v", err)
	}
	if string(response.Raw) != string(envelope) {
		t.Fatalf("raw = %q, want provider envelope %q", response.Raw, envelope)
	}
	if response.InputTok != 7 || response.OutputTok != 2 {
		t.Fatalf("response = %#v", response)
	}
}
