package app

import (
	"context"
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/config"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

func TestProviderUsageBudgetStopsBeforeNextCall(t *testing.T) {
	calls := 0
	fake := &adapters.FuncAdapter{AdapterID: "fake", InvokeFn: func(context.Context, adapters.Role, domain.Panelist, adapters.Request) (adapters.Response, error) {
		calls++
		return adapters.Response{Raw: []byte(`{}`), InputTok: 4, OutputTok: 4}, nil
	}}
	cfg := config.Default()
	cfg.Limits.TokenBudget = 12
	cfg.Limits.ReservedOutput = 5
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(cfg, store, adapters.NewRegistry(fake))
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	panelist := domain.Panelist{ID: "R-001", ReservedOutputTokens: 5}
	req := adapters.Request{SystemPrompt: "sys", Prompt: "abc", MaxOutputBytes: 1024}
	if _, err := service.invokeWithProviderLock(context.Background(), runDir, fake, adapters.RoleReviewer, panelist, req); err != nil {
		t.Fatal(err)
	}
	if _, err := service.invokeWithProviderLock(context.Background(), runDir, fake, adapters.RoleReviewer, panelist, req); err == nil || !strings.Contains(err.Error(), "token budget exhausted") {
		t.Fatalf("second call error = %v, want token budget exhaustion", err)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
	var ledger usageLedger
	if err := storage.ReadJSON(runDir+"/usage.json", &ledger); err != nil {
		t.Fatal(err)
	}
	if ledger.UsedTokens != 8 || ledger.Reserved != 0 || len(ledger.Calls) != 1 {
		t.Fatalf("usage ledger = %#v", ledger)
	}
}

func TestProviderByteCapIsIndependentFromOutputTokenReservation(t *testing.T) {
	var captured adapters.Request
	fake := &adapters.FuncAdapter{AdapterID: "fake", InvokeFn: func(_ context.Context, _ adapters.Role, _ domain.Panelist, req adapters.Request) (adapters.Response, error) {
		captured = req
		return adapters.Response{Raw: []byte(`{}`), InputTok: 1, OutputTok: 1}, nil
	}}
	cfg := config.Default()
	cfg.Limits.TokenBudget = 100
	cfg.Limits.ReservedOutput = 5
	cfg.Limits.MaxOutputBytes = 4096
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(cfg, store, adapters.NewRegistry(fake))
	if err != nil {
		t.Fatal(err)
	}
	panelist := domain.Panelist{ID: "R-001", ReservedOutputTokens: 5}
	req := adapters.Request{SystemPrompt: "sys", Prompt: "prompt", MaxOutputBytes: 2048}
	if _, err := service.invokeWithProviderLock(context.Background(), t.TempDir(), fake, adapters.RoleReviewer, panelist, req); err != nil {
		t.Fatal(err)
	}
	if captured.MaxOutputTokens != 5 {
		t.Fatalf("output token cap = %d, want 5", captured.MaxOutputTokens)
	}
	if captured.MaxOutputBytes != 2048 {
		t.Fatalf("output byte cap = %d, want explicit 2048", captured.MaxOutputBytes)
	}
}

func TestProviderByteCapDefaultsFromConfig(t *testing.T) {
	var captured adapters.Request
	fake := &adapters.FuncAdapter{AdapterID: "fake", InvokeFn: func(_ context.Context, _ adapters.Role, _ domain.Panelist, req adapters.Request) (adapters.Response, error) {
		captured = req
		return adapters.Response{Raw: []byte(`{}`), InputTok: 1, OutputTok: 1}, nil
	}}
	cfg := config.Default()
	cfg.Limits.TokenBudget = 100
	cfg.Limits.ReservedOutput = 5
	cfg.Limits.MaxOutputBytes = 4096
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(cfg, store, adapters.NewRegistry(fake))
	if err != nil {
		t.Fatal(err)
	}
	panelist := domain.Panelist{ID: "R-001", ReservedOutputTokens: 5}
	if _, err := service.invokeWithProviderLock(context.Background(), t.TempDir(), fake, adapters.RoleReviewer, panelist, adapters.Request{Prompt: "prompt"}); err != nil {
		t.Fatal(err)
	}
	if captured.MaxOutputBytes != 4096 {
		t.Fatalf("output byte cap = %d, want configured 4096", captured.MaxOutputBytes)
	}
}
