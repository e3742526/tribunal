package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

const replaySchemaVersion = 1

// ExecutionSnapshot is the immutable execution authority of a running run.
// It deliberately contains secret-reference digests, never secret values.
type ExecutionSnapshot struct {
	SchemaVersion        int          `json:"schema_version"`
	RunID                string       `json:"run_id"`
	WorkflowRevision     string       `json:"workflow_revision"`
	PacketHash           string       `json:"packet_hash"`
	Panel                domain.Panel `json:"panel"`
	Kind                 string       `json:"kind"`
	Split                bool         `json:"split"`
	NoWorkers            bool         `json:"no_workers"`
	LimitsDigest         string       `json:"limits_digest"`
	ProviderConfigDigest string       `json:"provider_config_digest"`
	Digest               string       `json:"digest"`
}

type requestEnvelope struct {
	SchemaVersion int               `json:"schema_version"`
	OperationType string            `json:"operation_type"`
	Destination   string            `json:"destination"`
	Role          string            `json:"role"`
	Panelist      domain.Panelist   `json:"panelist"`
	SystemPrompt  string            `json:"system_prompt"`
	Prompt        string            `json:"prompt"`
	Schema        string            `json:"response_schema"`
	MaxBytes      int64             `json:"max_output_bytes"`
	MaxTokens     int               `json:"max_output_tokens"`
	Timeout       int               `json:"timeout_seconds"`
	SecretDigests map[string]string `json:"secret_reference_digests"`
}

type operationRecord struct {
	SchemaVersion    int       `json:"schema_version"`
	RunID            string    `json:"run_id"`
	WorkflowRevision string    `json:"workflow_revision"`
	Sequence         int       `json:"sequence"`
	OperationType    string    `json:"operation_type"`
	RequestHash      string    `json:"request_hash"`
	OperationKey     string    `json:"operation_key"`
	IdempotencyKey   string    `json:"idempotency_key"`
	State            string    `json:"state"`
	ResultHash       string    `json:"result_hash,omitempty"`
	ResultBase64     string    `json:"result_base64,omitempty"`
	DuplicateRisk    bool      `json:"duplicate_effect_risk,omitempty"`
	Attempt          int       `json:"attempt"`
	PreparedAt       time.Time `json:"prepared_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type operationLedger struct {
	SchemaVersion int               `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Operations    []operationRecord `json:"operations"`
}

// DivergenceError is returned instead of making a live call whenever durable
// replay history differs from the operation the executor requested.
type DivergenceError struct {
	Sequence                 int
	Reason, Expected, Actual string
}

func (e *DivergenceError) Error() string {
	return fmt.Sprintf("replay divergence at operation %d: %s (expected=%s actual=%s)", e.Sequence, e.Reason, e.Expected, e.Actual)
}

type replayJournal struct {
	mu                    sync.Mutex
	path, runID, revision string
	strict                bool
	cursor                int
	ledger                operationLedger
	source                operationLedger
}

type replayJournalKey struct{}

func withReplayJournal(ctx context.Context, journal *replayJournal) context.Context {
	return context.WithValue(ctx, replayJournalKey{}, journal)
}
func replayJournalFromContext(ctx context.Context) *replayJournal {
	journal, _ := ctx.Value(replayJournalKey{}).(*replayJournal)
	return journal
}

func canonicalDigest(value any) (string, error) {
	data, err := json.Marshal(value) // encoding/json sorts string map keys and preserves null/omitted distinctions.
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func secretDigests(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		sum := sha256.Sum256([]byte(values[key]))
		out[key] = hex.EncodeToString(sum[:])
	}
	return out
}

func semanticRequestHash(adapter adapters.Adapter, role adapters.Role, panelist domain.Panelist, req adapters.Request) (string, error) {
	return canonicalDigest(requestEnvelope{replaySchemaVersion, req.OperationKey, adapter.ID(), string(role), panelist, req.SystemPrompt, req.Prompt, req.Schema, req.MaxOutputBytes, req.MaxOutputTokens, req.TimeoutSeconds, secretDigests(req.EnvSecrets)})
}

func newReplayJournal(runDir, runID, revision, source string) (*replayJournal, error) {
	j := &replayJournal{path: filepath.Join(runDir, "operations.json"), runID: runID, revision: revision, ledger: operationLedger{SchemaVersion: 1, RunID: runID, Operations: []operationRecord{}}}
	if source != "" {
		if err := storage.ReadJSONStrict(filepath.Join(source, "operations.json"), &j.source); err != nil {
			return nil, fmt.Errorf("read deterministic replay operations: %w", err)
		}
		j.strict = true
	} else if err := storage.ReadJSONStrict(j.path, &j.ledger); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if j.ledger.SchemaVersion != 1 || (j.strict && j.source.SchemaVersion != 1) {
		return nil, fmt.Errorf("unsupported operation ledger schema")
	}
	return j, nil
}

func (j *replayJournal) invoke(now time.Time, typ, hash string, live func() (adapters.Response, error)) (adapters.Response, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	seq := len(j.ledger.Operations) + 1
	if j.strict {
		seq = j.cursor + 1
		if seq > len(j.source.Operations) {
			return adapters.Response{}, j.divergence(seq, "missing operation record", "durable record", typ+":"+hash, now)
		}
		record := j.source.Operations[seq-1]
		if record.Sequence != seq || record.OperationType != typ || record.RequestHash != hash || record.WorkflowRevision != j.revision {
			return adapters.Response{}, j.divergence(seq, "sequence/type/hash/revision mismatch", fmt.Sprintf("%d:%s:%s:%s", record.Sequence, record.OperationType, record.RequestHash, record.WorkflowRevision), fmt.Sprintf("%d:%s:%s:%s", seq, typ, hash, j.revision), now)
		}
		if record.State != "committed" {
			return adapters.Response{}, fmt.Errorf("operation %d is %s and may have produced an external effect; human reconciliation required (at-least-once risk)", seq, record.State)
		}
		raw, err := base64.StdEncoding.DecodeString(record.ResultBase64)
		if err != nil {
			return adapters.Response{}, j.divergence(seq, "corrupt committed result", record.ResultHash, "invalid base64", now)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != record.ResultHash {
			return adapters.Response{}, j.divergence(seq, "corrupt committed result", record.ResultHash, hex.EncodeToString(sum[:]), now)
		}
		j.cursor++ // process-local consumption marker; source remains immutable.
		record.RunID, record.PreparedAt, record.UpdatedAt = j.runID, now, now
		j.ledger.Operations = append(j.ledger.Operations, record)
		if err := storage.WriteJSON(j.path, j.ledger); err != nil {
			return adapters.Response{}, fmt.Errorf("persist replayed operation: %w", err)
		}
		return adapters.Response{Raw: raw}, nil
	}
	record := operationRecord{SchemaVersion: 1, RunID: j.runID, WorkflowRevision: j.revision, Sequence: seq, OperationType: typ, RequestHash: hash, OperationKey: typ, IdempotencyKey: fmt.Sprintf("%s-%06d", j.runID, seq), State: "prepared", Attempt: 1, PreparedAt: now, UpdatedAt: now}
	j.ledger.Operations = append(j.ledger.Operations, record)
	if err := storage.WriteJSON(j.path, j.ledger); err != nil {
		j.ledger.Operations = j.ledger.Operations[:len(j.ledger.Operations)-1]
		return adapters.Response{}, err
	}
	j.ledger.Operations[seq-1].State = "in_flight"
	j.ledger.Operations[seq-1].UpdatedAt = now
	if err := storage.WriteJSON(j.path, j.ledger); err != nil {
		return adapters.Response{}, err
	}
	response, err := live()
	current := &j.ledger.Operations[seq-1]
	current.UpdatedAt = time.Now().UTC()
	if err != nil {
		current.State, current.DuplicateRisk = "in_doubt", true
	} else {
		sum := sha256.Sum256(response.Raw)
		current.State, current.ResultHash, current.ResultBase64 = "committed", hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(response.Raw)
	}
	if persistErr := storage.WriteJSON(j.path, j.ledger); persistErr != nil {
		return response, fmt.Errorf("operation result is in doubt because commit persistence failed: %w", persistErr)
	}
	return response, err
}

func (j *replayJournal) divergence(sequence int, reason, expected, actual string, at time.Time) error {
	err := &DivergenceError{sequence, reason, expected, actual}
	// Values are only types, versions, and hashes; request content and secrets
	// never enter the diagnostic artifact.
	_ = storage.WriteJSON(filepath.Join(filepath.Dir(j.path), "divergence.json"), map[string]any{"schema_version": 1, "sequence": sequence, "reason": reason, "expected": expected, "actual": actual, "at": at})
	return err
}

func buildExecutionSnapshot(runID string, packet documents.Packet, panel domain.Panel, opts ReviewOptions, s *Service) (ExecutionSnapshot, error) {
	limits, err := canonicalDigest(s.Config.Limits)
	if err != nil {
		return ExecutionSnapshot{}, err
	}
	provider, err := canonicalDigest(struct {
		OpenAI  any
		Mistral any
		Workers any
	}{s.Config.OpenAICompatible, s.Config.MistralAcp, s.Config.Workers})
	if err != nil {
		return ExecutionSnapshot{}, err
	}
	snapshot := ExecutionSnapshot{SchemaVersion: 1, RunID: runID, WorkflowRevision: packet.PacketHash, PacketHash: packet.PacketHash, Panel: panel, Kind: packet.Kind, Split: opts.Split, NoWorkers: opts.NoWorkers, LimitsDigest: limits, ProviderConfigDigest: provider}
	snapshot.Digest, err = canonicalDigest(snapshot)
	return snapshot, err
}

func persistImmutableSnapshot(path string, snapshot ExecutionSnapshot) error {
	var existing ExecutionSnapshot
	if err := storage.ReadJSONStrict(path, &existing); err == nil {
		if existing.Digest != snapshot.Digest {
			return errors.New("execution snapshot is immutable; start a new run for changed workflow or arguments")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return storage.WriteJSON(path, snapshot)
}
