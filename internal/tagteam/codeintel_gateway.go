package tagteam

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type CodeIntelGatewayResult struct {
	SchemaVersion int                     `json:"schema_version"`
	Operation     string                  `json:"operation"`
	Status        string                  `json:"status"`
	Staleness     string                  `json:"staleness"`
	Capabilities  []CodeIntelCapabilities `json:"capabilities,omitempty"`
	Artifact      *CodeIntelArtifact      `json:"artifact,omitempty"`
	Errors        []string                `json:"errors,omitempty"`
	GeneratedAt   time.Time               `json:"generated_at"`
}

func RunCodeIntelGateway(ctx context.Context, cfg Config, workdir, prompt, operation string) CodeIntelGatewayResult {
	result := CodeIntelGatewayResult{SchemaVersion: codeIntelEnvelopeSchemaVersion, Operation: operation, Status: codeIntelStatusDisabled, Staleness: codeIntelStalenessUnknown, GeneratedAt: time.Now().UTC()}
	if !validCodeIntelOperation(operation) {
		result.Status = codeIntelStatusError
		result.Errors = []string{"unsupported code-intel operation"}
		return result
	}
	if !codeIntelRepoAllowed(workdir, cfg.CodeIntel.AllowedRepos) {
		result.Errors = []string{"repository is not in code_intel.allowed_repos"}
		return result
	}
	opts := RunOptions{Workdir: workdir, Prompt: prompt, CodeIntelCommand: cfg.Defaults.CodeIntelCommand, CodeIntel: cfg.CodeIntel}
	providers, err := configuredCodeIntelProviders(opts)
	if err != nil {
		result.Status = codeIntelStatusProviderUnavailable
		result.Errors = []string{err.Error()}
		return result
	}
	if len(providers) == 0 {
		result.Status = codeIntelStatusProviderUnavailable
		result.Errors = []string{"no configured provider supports " + operation}
		return result
	}
	capable := []CodeIntelProvider{}
	for _, provider := range providers {
		caps := provider.Capabilities()
		result.Capabilities = append(result.Capabilities, caps)
		for _, capability := range caps.Operations {
			if capability == operation {
				capable = append(capable, provider)
				break
			}
		}
	}
	if len(capable) == 0 {
		result.Status = codeIntelStatusProviderUnavailable
		result.Errors = []string{"no configured provider supports " + operation}
		return result
	}
	artifact := aggregateCodeIntelProviders(ctx, workdir, prompt, capable)
	result.Artifact = &artifact
	result.Status, result.Staleness, result.Errors = artifact.Status, artifact.Staleness, artifact.Errors
	return result
}

func validCodeIntelOperation(operation string) bool {
	switch operation {
	case "orient", "find", "trace", "impact", "resume", "recall", "evidence":
		return true
	}
	return false
}

type CodeIntelBenchResult struct {
	Provider         string `json:"provider"`
	LatencyMS        int64  `json:"latency_ms"`
	ObservationCount int    `json:"observation_count"`
	Valid            bool   `json:"valid"`
	Error            string `json:"error,omitempty"`
}
type CodeIntelBenchArtifact struct {
	SchemaVersion int                    `json:"schema_version"`
	Tasks         []string               `json:"tasks"`
	Results       []CodeIntelBenchResult `json:"results"`
	GeneratedAt   time.Time              `json:"generated_at"`
}

func RunCodeIntelBench(ctx context.Context, cfg Config, workdir, runDir string) (CodeIntelBenchArtifact, error) {
	artifact := CodeIntelBenchArtifact{SchemaVersion: codeIntelEnvelopeSchemaVersion, Tasks: []string{"orient", "find", "trace", "impact"}, Results: []CodeIntelBenchResult{}, GeneratedAt: time.Now().UTC()}
	opts := RunOptions{Workdir: workdir, CodeIntelCommand: cfg.Defaults.CodeIntelCommand, CodeIntel: cfg.CodeIntel}
	providers, err := configuredCodeIntelProviders(opts)
	if err != nil {
		return artifact, err
	}
	for _, provider := range providers {
		for _, task := range artifact.Tasks {
			started := time.Now()
			observed, observeErr := provider.Observe(ctx, CodeIntelRequest{Workdir: workdir, Prompt: task})
			result := CodeIntelBenchResult{Provider: provider.Name(), LatencyMS: time.Since(started).Milliseconds(), ObservationCount: len(observed.Observations), Valid: observeErr == nil}
			if observeErr != nil {
				result.Error = sanitizeCodeIntelText(observeErr.Error(), maxCodeIntelSummaryBytes)
			}
			artifact.Results = append(artifact.Results, result)
		}
	}
	if runDir != "" {
		if err := writeJSONWithNewline(filepath.Join(runDir, "intel-bench.json"), artifact); err != nil {
			return artifact, err
		}
	}
	return artifact, nil
}

// ReadCodeIntelStatus reads only sensor artifacts and does not reconstruct
// final.json. It is safe for an operator/TUI-style inspection path.
func ReadCodeIntelStatus(runsRoot string) ([]CodeIntelArtifact, error) {
	paths := []string{}
	err := filepath.WalkDir(runsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "code-intel-round-1.json" {
			paths = append(paths, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return []CodeIntelArtifact{}, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if len(paths) > 20 {
		paths = paths[len(paths)-20:]
	}
	artifacts := make([]CodeIntelArtifact, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var artifact CodeIntelArtifact
		if json.Unmarshal(data, &artifact) == nil {
			artifacts = append(artifacts, artifact)
		}
	}
	return artifacts, nil
}

func CodeIntelGatewayJSON(result CodeIntelGatewayResult) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}
