package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/documents"
)

func TestWorkerFetchRequiresExactAllowlistAndHashes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("source evidence")) }))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	worker := WorkerService{AllowedDomains: []string{parsed.Hostname()}, Client: server.Client(), AllowPrivateForTest: true, Clock: func() time.Time { return time.Unix(1, 0) }}
	evidence, err := worker.Fetch(context.Background(), server.URL, "websearch", "pre-review")
	if err != nil {
		t.Fatal(err)
	}
	if evidence.ContentSHA256 == "" || evidence.Excerpt != "source evidence" {
		t.Fatalf("evidence=%#v", evidence)
	}
	worker.AllowedDomains = []string{"example.test"}
	if _, err := worker.Fetch(context.Background(), server.URL, "websearch", "pre-review"); err == nil {
		t.Fatal("expected allowlist rejection")
	}
}

func TestDeterministicWorkersProduceBoundedFindings(t *testing.T) {
	packet := documents.Packet{Items: []documents.Item{{ID: "artifact:x.md", PacketSHA256: "hash", Content: "This occured in [9].\n\n[1] Existing reference."}}}
	spelling := Spellcheck(packet)
	references := ReferenceCheck(packet)
	if len(spelling) == 0 || spelling[0].Severity != "nit" {
		t.Fatalf("spelling=%#v", spelling)
	}
	if len(references) != 1 || references[0].Severity != "minor" {
		t.Fatalf("references=%#v", references)
	}
}

func TestWorkerRedirectCannotEscapeAllowlistWithCustomClient(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("escaped")) }))
	defer target.Close()
	escapingURL := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, escapingURL, http.StatusFound) }))
	defer source.Close()
	parsed, _ := url.Parse(source.URL)
	worker := WorkerService{AllowedDomains: []string{parsed.Hostname()}, Client: source.Client(), AllowPrivateForTest: true}
	if _, err := worker.Fetch(context.Background(), source.URL, "test", "pre-review"); err == nil {
		t.Fatal("redirect escaped exact-domain allowlist")
	}
}

func TestResolveTypedEvidenceTargets(t *testing.T) {
	tests := map[string]string{"doi:10.1000/example": "crossref", "pmid:12345": "pubmed", "arxiv:2401.00001": "arxiv", "https://example.com/source": "url"}
	for input, provider := range tests {
		target, err := ResolveEvidenceTarget(input)
		if err != nil || target.Provider != provider || target.URL == "" {
			t.Fatalf("ResolveEvidenceTarget(%q) = %#v, %v", input, target, err)
		}
	}
}
