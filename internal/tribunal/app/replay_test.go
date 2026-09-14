package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func TestCanonicalRequestGoldenVectors(t *testing.T) {
	a := &adapters.FuncAdapter{AdapterID: "test"}
	panelist := domain.Panelist{ID: "R-001", Adapter: "test", Model: "model", Persona: "plain", Weight: 1, MaxContextTokens: 100, ReservedOutputTokens: 10}
	base := adapters.Request{OperationKey: "provider.review.R-001", SystemPrompt: "café", Prompt: "审查", Schema: `{"type":"object"}`, MaxOutputBytes: 100, MaxOutputTokens: 10, TimeoutSeconds: 4, EnvSecrets: map[string]string{"B": "two", "A": "one"}}
	want, err := semanticRequestHash(a, adapters.RoleReviewer, panelist, base)
	if err != nil {
		t.Fatal(err)
	}
	reordered := base
	reordered.EnvSecrets = map[string]string{"A": "one", "B": "two"}
	got, _ := semanticRequestHash(a, adapters.RoleReviewer, panelist, reordered)
	if got != want {
		t.Fatalf("map order changed digest: %s != %s", got, want)
	}
	mutations := []adapters.Request{base, base, base, base, base, base}
	mutations[0].Prompt = "different"
	mutations[1].Schema = `{"type":"array"}`
	mutations[2].MaxOutputBytes++
	mutations[3].MaxOutputTokens++
	mutations[4].TimeoutSeconds++
	mutations[5].EnvSecrets = map[string]string{"A": "changed", "B": "two"}
	for i, mutation := range mutations {
		digest, _ := semanticRequestHash(a, adapters.RoleReviewer, panelist, mutation)
		if digest == want {
			t.Errorf("semantic mutation %d did not change digest", i)
		}
	}
}

func TestImmutableExecutionSnapshotRejectsMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "execution-snapshot.json")
	snapshot := ExecutionSnapshot{SchemaVersion: 1, RunID: "run", WorkflowRevision: "rev", PacketHash: "packet", Kind: "generic", LimitsDigest: "limits", ProviderConfigDigest: "provider"}
	snapshot.Digest, _ = canonicalDigest(snapshot)
	if err := persistImmutableSnapshot(path, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := persistImmutableSnapshot(path, snapshot); err != nil {
		t.Fatalf("identical snapshot: %v", err)
	}
	snapshot.Kind = "mutated"
	snapshot.Digest, _ = canonicalDigest(snapshot)
	if err := persistImmutableSnapshot(path, snapshot); err == nil {
		t.Fatal("mutable workflow snapshot was accepted")
	}
}

func TestExactReplayReturnsCommittedResultWithoutLiveCall(t *testing.T) {
	source := t.TempDir()
	live, err := newReplayJournal(source, "source", "revision", "")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	response, err := live.invoke(time.Now(), "provider.review.R-001", "hash", func() (adapters.Response, error) { calls++; return adapters.Response{Raw: []byte("result")}, nil })
	if err != nil || string(response.Raw) != "result" {
		t.Fatalf("live invoke = %q, %v", response.Raw, err)
	}
	replay, err := newReplayJournal(t.TempDir(), "replay", "revision", source)
	if err != nil {
		t.Fatal(err)
	}
	response, err = replay.invoke(time.Now(), "provider.review.R-001", "hash", func() (adapters.Response, error) { calls++; return adapters.Response{}, nil })
	if err != nil || string(response.Raw) != "result" || calls != 1 {
		t.Fatalf("replay calls=%d response=%q err=%v", calls, response.Raw, err)
	}
}

func TestReplayDivergenceAndUncertainEffectsFailClosed(t *testing.T) {
	for _, tc := range []struct{ name, typ, hash string }{
		{"type", "provider.vote.R-001", "hash"}, {"hash", "provider.review.R-001", "changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := t.TempDir()
			live, _ := newReplayJournal(source, "source", "revision", "")
			_, _ = live.invoke(time.Now(), "provider.review.R-001", "hash", func() (adapters.Response, error) { return adapters.Response{Raw: []byte("ok")}, nil })
			replay, _ := newReplayJournal(t.TempDir(), "replay", "revision", source)
			_, err := replay.invoke(time.Now(), tc.typ, tc.hash, func() (adapters.Response, error) {
				t.Fatal("divergence made live call")
				return adapters.Response{}, nil
			})
			var divergence *DivergenceError
			if !errors.As(err, &divergence) {
				t.Fatalf("error = %T %v, want divergence", err, err)
			}
		})
	}
	t.Run("missing", func(t *testing.T) {
		source := t.TempDir()
		if err := os.WriteFile(filepath.Join(source, "operations.json"), []byte(`{"schema_version":1,"run_id":"source","operations":[]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		replay, _ := newReplayJournal(t.TempDir(), "replay", "revision", source)
		_, err := replay.invoke(time.Now(), "provider.review.R-001", "hash", func() (adapters.Response, error) {
			t.Fatal("missing history made live call")
			return adapters.Response{}, nil
		})
		var divergence *DivergenceError
		if !errors.As(err, &divergence) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("in-doubt", func(t *testing.T) {
		source := t.TempDir()
		live, _ := newReplayJournal(source, "source", "revision", "")
		_, _ = live.invoke(time.Now(), "provider.review.R-001", "hash", func() (adapters.Response, error) { return adapters.Response{}, errors.New("transport uncertain") })
		replay, _ := newReplayJournal(t.TempDir(), "replay", "revision", source)
		_, err := replay.invoke(time.Now(), "provider.review.R-001", "hash", func() (adapters.Response, error) {
			t.Fatal("uncertain operation repeated")
			return adapters.Response{}, nil
		})
		if err == nil {
			t.Fatal("in-doubt operation replayed")
		}
	})
}
