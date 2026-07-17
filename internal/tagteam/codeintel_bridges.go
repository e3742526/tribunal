package tagteam

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type AlexandriaObservationExport struct {
	Observations []CodeIntelObservation `json:"observations"`
	RunID        string                 `json:"run_id"`
}

// ExportAlexandriaObservations writes a bounded, redacted local contract. It
// does not connect to Alexandria and never exports a graph dump.
func ExportAlexandriaObservations(ctx context.Context, workdir, runDir, runID string, bridge CodeIntelFileBridgeConfig, artifact CodeIntelArtifact) (string, bool, error) {
	payload := AlexandriaObservationExport{RunID: runID, Observations: artifact.Observations}
	return writeIdempotentBridgeEnvelope(ctx, workdir, runDir, bridge, "alexandria.observations", runID, payload)
}

// MuninnCandidateEvidence is intentionally evidence-only. Promotion to any
// memory system is an external decision and cannot be triggered by Tagteam.
type MuninnCandidateEvidence struct {
	Candidate string              `json:"candidate"`
	Evidence  []RetrievalEvidence `json:"evidence"`
	RunID     string              `json:"run_id"`
}

func ExportMuninnCandidateEvidence(ctx context.Context, workdir, runDir, runID string, bridge CodeIntelFileBridgeConfig, candidate MuninnCandidateEvidence) (string, error) {
	if candidate.RunID == "" {
		candidate.RunID = runID
	}
	return writeCodeIntelEnvelope(ctx, workdir, runDir, bridge, "muninn.candidate-evidence", candidate)
}

func ImportMuninnCandidateEvidence(path string) (MuninnCandidateEvidence, error) {
	envelope, err := readCodeIntelEnvelope(path, "muninn.candidate-evidence")
	if err != nil {
		return MuninnCandidateEvidence{}, err
	}
	var candidate MuninnCandidateEvidence
	if err := json.Unmarshal(envelope.Payload, &candidate); err != nil {
		return MuninnCandidateEvidence{}, fmt.Errorf("decode muninn candidate evidence: %w", err)
	}
	if candidate.Candidate == "" || len(candidate.Evidence) == 0 {
		return MuninnCandidateEvidence{}, fmt.Errorf("invalid muninn candidate evidence")
	}
	return candidate, nil
}

func writeIdempotentBridgeEnvelope(ctx context.Context, workdir, runDir string, bridge CodeIntelFileBridgeConfig, kind, runID string, payload any) (string, bool, error) {
	if !bridge.Enabled {
		return "", false, fmt.Errorf("alexandria bridge is disabled")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", false, err
	}
	if len(data) > maxCodeIntelArtifactSize {
		return "", false, fmt.Errorf("%s payload exceeds %d bytes", kind, maxCodeIntelArtifactSize)
	}
	snapshot := SnapshotIdentityForCodeIntel(ctx, workdir)
	sum := sha256.Sum256(append([]byte(runID+"\n"+snapshot.Revision+"\n"), data...))
	id := hex.EncodeToString(sum[:])
	path := bridge.Path
	if path == "" {
		path = filepath.Join(runDir, kind+".json")
	}
	if existing, err := os.ReadFile(path); err == nil {
		var envelope CodeIntelEnvelope
		if json.Unmarshal(existing, &envelope) == nil && envelope.ID == id {
			return path, true, nil
		}
	}
	data = []byte(redactSecrets(string(data)))
	envelope := CodeIntelEnvelope{SchemaVersion: codeIntelEnvelopeSchemaVersion, EnvelopeKind: kind, ID: id, Snapshot: snapshot, Payload: data, CreatedAt: time.Now().UTC()}
	if err := writeJSONWithNewline(path, envelope); err != nil {
		return "", false, err
	}
	return path, false, nil
}
