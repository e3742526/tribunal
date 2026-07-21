package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
