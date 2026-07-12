package tagteam

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const codeIntelEnvelopeSchemaVersion = 1

type SnapshotIdentity struct {
	SchemaVersion int    `json:"schema_version"`
	Revision      string `json:"revision"`
	DirtyDigest   string `json:"dirty_digest,omitempty"`
}

type CodeIntelEnvelope struct {
	SchemaVersion int              `json:"schema_version"`
	EnvelopeKind  string           `json:"envelope_kind"`
	ID            string           `json:"id,omitempty"`
	Snapshot      SnapshotIdentity `json:"snapshot"`
	Payload       json.RawMessage  `json:"payload"`
	CreatedAt     time.Time        `json:"created_at"`
}

func SnapshotIdentityForCodeIntel(ctx context.Context, workdir string) SnapshotIdentity {
	identity := SnapshotIdentity{SchemaVersion: codeIntelEnvelopeSchemaVersion}
	revision, err := runCommand(ctx, workdir, "git", "rev-parse", "--verify", "HEAD")
	if err != nil || !codeIntelRevisionPattern.MatchString(strings.TrimSpace(revision)) {
		return identity
	}
	identity.Revision = strings.TrimSpace(revision)
	status, statusErr := runCommand(ctx, workdir, "git", "status", "--porcelain")
	diff, diffErr := runCommand(ctx, workdir, "git", "diff", "--no-ext-diff", "HEAD", "--")
	if statusErr == nil && diffErr == nil && strings.TrimSpace(status) != "" {
		sum := sha256.Sum256([]byte(status + "\n" + diff))
		identity.DirtyDigest = hex.EncodeToString(sum[:])
	}
	return identity
}

func WriteDoryCheckpoint(ctx context.Context, workdir, runDir string, bridge CodeIntelFileBridgeConfig, artifact CodeIntelArtifact) (string, error) {
	// The run-local copy is authoritative. A configured Dory path receives a
	// second copy only after explicit opt-in; Tagteam never creates .dory by
	// default or treats it as a source of authority.
	local := bridge
	local.Path = ""
	path, err := writeCodeIntelEnvelope(ctx, workdir, runDir, local, "dory.checkpoint", artifact)
	if err != nil || bridge.Path == "" {
		return path, err
	}
	envelope, err := readCodeIntelEnvelope(path, "dory.checkpoint")
	if err != nil {
		return path, err
	}
	return path, writeJSONWithNewline(bridge.Path, envelope)
}

func ReadDoryHandoff(path string) (CodeIntelEnvelope, CodeIntelArtifact, error) {
	envelope, err := readCodeIntelEnvelope(path, "dory.checkpoint")
	if err != nil {
		return CodeIntelEnvelope{}, CodeIntelArtifact{}, err
	}
	var artifact CodeIntelArtifact
	if err := json.Unmarshal(envelope.Payload, &artifact); err != nil {
		return CodeIntelEnvelope{}, CodeIntelArtifact{}, fmt.Errorf("decode dory handoff: %w", err)
	}
	return envelope, artifact, nil
}

func writeCodeIntelEnvelope(ctx context.Context, workdir, runDir string, bridge CodeIntelFileBridgeConfig, kind string, payload any) (string, error) {
	if !bridge.Enabled {
		return "", fmt.Errorf("%s bridge is disabled", strings.Split(kind, ".")[0])
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if len(data) > maxCodeIntelArtifactSize {
		return "", fmt.Errorf("%s payload exceeds %d bytes", kind, maxCodeIntelArtifactSize)
	}
	data = []byte(redactSecrets(string(data)))
	snapshot := SnapshotIdentityForCodeIntel(ctx, workdir)
	sum := sha256.Sum256(append([]byte(kind+"\n"+snapshot.Revision+"\n"), data...))
	envelope := CodeIntelEnvelope{SchemaVersion: codeIntelEnvelopeSchemaVersion, EnvelopeKind: kind, ID: hex.EncodeToString(sum[:]), Snapshot: snapshot, Payload: data, CreatedAt: time.Now().UTC()}
	path := filepath.Join(runDir, strings.ReplaceAll(kind, ".", "-")+".json")
	if bridge.Path != "" {
		path = bridge.Path
	}
	if err := writeJSONWithNewline(path, envelope); err != nil {
		return "", err
	}
	return path, nil
}

func readCodeIntelEnvelope(path, kind string) (CodeIntelEnvelope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CodeIntelEnvelope{}, err
	}
	if len(data) > maxCodeIntelArtifactSize {
		return CodeIntelEnvelope{}, fmt.Errorf("envelope exceeds %d bytes", maxCodeIntelArtifactSize)
	}
	var envelope CodeIntelEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return CodeIntelEnvelope{}, fmt.Errorf("decode envelope: %w", err)
	}
	if envelope.SchemaVersion != codeIntelEnvelopeSchemaVersion || envelope.EnvelopeKind != kind || envelope.CreatedAt.IsZero() {
		return CodeIntelEnvelope{}, fmt.Errorf("invalid %s envelope", kind)
	}
	return envelope, nil
}
