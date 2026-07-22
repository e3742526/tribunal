package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

type usageBudgetKey struct{}

type usageRecord struct {
	SchemaVersion int     `json:"schema_version"`
	Role          string  `json:"role"`
	ReviewerID    string  `json:"reviewer_id"`
	Reserved      int     `json:"reserved_tokens"`
	Input         int     `json:"input_tokens"`
	Output        int     `json:"output_tokens"`
	CostUSD       float64 `json:"cost_usd,omitempty"`
	Status        string  `json:"status"`
}

type usageLedger struct {
	SchemaVersion int           `json:"schema_version"`
	TokenLimit    int           `json:"token_limit"`
	UsedTokens    int           `json:"used_tokens"`
	Reserved      int           `json:"reserved_tokens"`
	CostUSD       float64       `json:"cost_usd"`
	Calls         []usageRecord `json:"calls"`
}

type usageBudget struct {
	mu      sync.Mutex
	path    string
	ledger  usageLedger
	pending map[int]usageRecord
	nextID  int
}

func loadUsageBudget(runDir string, tokenLimit int) (*usageBudget, error) {
	if tokenLimit <= 0 {
		return nil, fmt.Errorf("token budget must be positive")
	}
	path := filepath.Join(runDir, "usage.json")
	budget := &usageBudget{path: path, pending: map[int]usageRecord{}, ledger: usageLedger{SchemaVersion: 1, TokenLimit: tokenLimit, Calls: []usageRecord{}}}
	if err := storage.ReadJSONStrict(path, &budget.ledger); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read usage ledger: %w", err)
		}
	} else if budget.ledger.SchemaVersion != 1 || budget.ledger.TokenLimit != tokenLimit {
		return nil, fmt.Errorf("usage ledger is unsupported or mismatched")
	} else if budget.ledger.Reserved != 0 {
		// A crash can strand a reservation after the provider received the call.
		// Charge it conservatively before any resumed invocation can start.
		budget.ledger.Calls = append(budget.ledger.Calls, usageRecord{SchemaVersion: 1, Role: "recovery", ReviewerID: "unknown", Reserved: budget.ledger.Reserved, Input: budget.ledger.Reserved, Status: "recovered-reservation"})
		budget.ledger.UsedTokens += budget.ledger.Reserved
		budget.ledger.Reserved = 0
	}
	if budget.ledger.UsedTokens > budget.ledger.TokenLimit {
		return nil, fmt.Errorf("usage ledger exceeds token budget after crash recovery")
	}
	if err := storage.WriteJSON(path, budget.ledger); err != nil {
		return nil, fmt.Errorf("persist usage ledger: %w", err)
	}
	return budget, nil
}

func withUsageBudget(ctx context.Context, budget *usageBudget) context.Context {
	return context.WithValue(ctx, usageBudgetKey{}, budget)
}

func budgetFromContext(ctx context.Context) *usageBudget {
	budget, _ := ctx.Value(usageBudgetKey{}).(*usageBudget)
	return budget
}

func (s *Service) invokeWithProviderLock(ctx context.Context, runDir string, adapter adapters.Adapter, role adapters.Role, panelist domain.Panelist, req adapters.Request) (adapters.Response, error) {
	if req.MaxOutputTokens <= 0 {
		req.MaxOutputTokens = panelist.ReservedOutputTokens
		if req.MaxOutputTokens <= 0 {
			req.MaxOutputTokens = s.Config.Limits.ReservedOutput
		}
	}
	maxTokenBytes := int64(req.MaxOutputTokens) * 3
	if req.MaxOutputBytes <= 0 || req.MaxOutputBytes > maxTokenBytes {
		req.MaxOutputBytes = maxTokenBytes
	}
	invoke := func() (adapters.Response, error) {
		budget := budgetFromContext(ctx)
		if budget == nil {
			var err error
			budget, err = loadUsageBudget(runDir, s.Config.Limits.TokenBudget)
			if err != nil {
				return adapters.Response{}, err
			}
		}
		reservation, err := budget.reserve(role, panelist.ID, req)
		if err != nil {
			return adapters.Response{}, err
		}
		response, invokeErr := adapter.Invoke(ctx, role, panelist, req)
		if budgetErr := budget.complete(reservation, response, invokeErr); budgetErr != nil {
			return response, budgetErr
		}
		return response, invokeErr
	}
	if !adapter.Serialize() {
		return invoke()
	}
	lock, err := storage.AcquireLock(ctx, filepath.Join(s.Store.Root, "providers", adapter.ID()+".lock"), nil)
	if err != nil {
		return adapters.Response{}, err
	}
	defer lock.Close()
	return invoke()
}

func (b *usageBudget) reserve(role adapters.Role, reviewerID string, req adapters.Request) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	input := estimateTokens(req.SystemPrompt + "\n\n" + req.Prompt)
	output := req.MaxOutputTokens
	if output <= 0 {
		return 0, fmt.Errorf("provider request is missing an output-token cap")
	}
	reserved := input + output
	if b.ledger.UsedTokens+b.ledger.Reserved+reserved > b.ledger.TokenLimit {
		return 0, fmt.Errorf("token budget exhausted: used=%d reserved=%d requested=%d limit=%d", b.ledger.UsedTokens, b.ledger.Reserved, reserved, b.ledger.TokenLimit)
	}
	b.nextID++
	id := b.nextID
	record := usageRecord{SchemaVersion: 1, Role: string(role), ReviewerID: reviewerID, Reserved: reserved, Input: input, Status: "reserved"}
	b.pending[id] = record
	b.ledger.Reserved += reserved
	if err := storage.WriteJSON(b.path, b.ledger); err != nil {
		delete(b.pending, id)
		b.ledger.Reserved -= reserved
		return 0, fmt.Errorf("persist token reservation: %w", err)
	}
	return id, nil
}

func (b *usageBudget) complete(id int, response adapters.Response, invokeErr error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	record, ok := b.pending[id]
	if !ok {
		return fmt.Errorf("unknown token reservation %d", id)
	}
	delete(b.pending, id)
	b.ledger.Reserved -= record.Reserved
	if invokeErr != nil {
		// A transport error does not prove the provider consumed nothing. Charge the
		// full reservation so retries cannot silently escape the run ceiling.
		record.Input, record.Output, record.Status = record.Reserved, 0, "failed-reserved"
	} else {
		if response.InputTok > 0 {
			record.Input = response.InputTok
		}
		if response.OutputTok > 0 {
			record.Output = response.OutputTok
		} else {
			record.Output = estimateTokens(string(response.Raw))
		}
		record.CostUSD, record.Status = response.CostUSD, "completed"
	}
	b.ledger.UsedTokens += record.Input + record.Output
	b.ledger.CostUSD += record.CostUSD
	b.ledger.Calls = append(b.ledger.Calls, record)
	if err := storage.WriteJSON(b.path, b.ledger); err != nil {
		return fmt.Errorf("persist provider usage: %w", err)
	}
	if b.ledger.UsedTokens > b.ledger.TokenLimit {
		return fmt.Errorf("provider usage exceeded token budget: used=%d limit=%d", b.ledger.UsedTokens, b.ledger.TokenLimit)
	}
	return nil
}

func estimateTokens(value string) int { return (len(value) + 2) / 3 }
