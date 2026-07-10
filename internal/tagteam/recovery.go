package tagteam

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const RecoveryDecisionSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["schema_version", "decision", "reason", "evidence"],
  "properties": {
    "schema_version": {"type": "integer", "const": 1},
    "decision": {"type": "string", "enum": ["repair", "continue_with_fallback", "quarantine"]},
    "reason": {"type": "string", "minLength": 1},
    "evidence": {"type": "array", "items": {"type": "string"}}
  }
}`

type RecoveryDecision struct {
	SchemaVersion int      `json:"schema_version"`
	Decision      string   `json:"decision"`
	Reason        string   `json:"reason"`
	Evidence      []string `json:"evidence"`
}

type RecoveryArtifact struct {
	SchemaVersion int               `json:"schema_version"`
	Round         int               `json:"round"`
	Failure       string            `json:"failure"`
	PartialDiff   DiffFilesMetadata `json:"partial_diff"`
	DiffPath      string            `json:"diff_path"`
	Test          *TestRun          `json:"test,omitempty"`
	Decision      RecoveryDecision  `json:"decision"`
	Original      RoleTarget        `json:"original_worker"`
	Selected      RoleTarget        `json:"selected_worker,omitempty"`
	Status        string            `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

func (d RecoveryDecision) Validate(allowed map[string]bool) error {
	if d.SchemaVersion != ArtifactSchemaVersion {
		return fmt.Errorf("unsupported recovery schema_version %d", d.SchemaVersion)
	}
	if !allowed[d.Decision] {
		return fmt.Errorf("recovery decision %q is not available", d.Decision)
	}
	if strings.TrimSpace(d.Reason) == "" || d.Evidence == nil {
		return fmt.Errorf("recovery decision requires reason and evidence")
	}
	return nil
}

func parseRecoveryDecision(raw []byte, allowed map[string]bool) (RecoveryDecision, error) {
	var decision RecoveryDecision
	if err := json.Unmarshal(raw, &decision); err != nil {
		extracted, extractErr := extractJSONObject(raw)
		if extractErr != nil || json.Unmarshal(extracted, &decision) != nil {
			return RecoveryDecision{}, &OutputContractError{Err: fmt.Errorf("decode recovery decision: %w", err)}
		}
	}
	if err := decision.Validate(allowed); err != nil {
		return RecoveryDecision{}, &OutputContractError{Err: err}
	}
	return decision, nil
}

func recoveryPrompt(failure error, diff DiffArtifact, test *TestRun, allowed map[string]bool) string {
	actions := []string{"quarantine"}
	if allowed["repair"] {
		actions = append(actions, "repair")
	}
	if allowed["continue_with_fallback"] {
		actions = append(actions, "continue_with_fallback")
	}
	testSummary := "not configured"
	if test != nil {
		testSummary = fmt.Sprintf("passed=%t output=%s", test.Passed, safeTestOutput(test.Output))
	}
	return fmt.Sprintf(`You are the read-only recovery supervisor. An implementation adapter failed after changing the worktree.

Failure: %s
Diff path: %s
Changed files: %v
Diff hash: %s
Focused tests: %s
Allowed decisions: %s

Choose repair only when the same resumable worker should repair its partial work. Choose continue_with_fallback when another configured worker should preserve and continue the partial work. Choose quarantine when further automated edits are unsafe.

Return JSON only matching the supplied schema.`, failure, diff.PatchPath, diff.ChangedFiles(), diff.Metadata.DiffSHA256, testSummary, strings.Join(actions, ", "))
}

func (a *App) recoverEditorFailure(
	ctx context.Context,
	opts RunOptions,
	round int,
	runDir, baseline, workerSchemaPath, sessionID string,
	originalTarget RoleTarget,
	originalAdapter, reviewer Adapter,
	registry map[string]Adapter,
	failure error,
	before worktreeSnapshot,
	final *FinalRun,
) (Result, RoleTarget, Adapter, error) {
	after, snapshotErr := captureWorktreeSnapshot(context.Background(), opts.Workdir)
	if snapshotErr != nil || len(worktreeDelta(before, after)) == 0 {
		return Result{}, originalTarget, originalAdapter, failure
	}
	_ = writeRunState(runDir, RunState{RunID: final.RunID, Mode: opts.Mode, Status: "running", Phase: string(PhaseRepairing), CurrentRound: round, RecoveryStatus: "checkpointing", RoleStatuses: final.RoleStatuses})
	diff, err := captureDiffArtifact(context.Background(), opts.Workdir, baseline, runDir, round)
	if err != nil {
		return Result{}, originalTarget, originalAdapter, failure
	}
	artifact := RecoveryArtifact{
		SchemaVersion: ArtifactSchemaVersion,
		Round:         round,
		Failure:       redactSecretsWithOverlay(failure.Error(), opts.EnvOverlay),
		PartialDiff:   diff.Metadata,
		DiffPath:      diff.PatchPath,
		Original:      originalTarget,
		Status:        "awaiting_decision",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if opts.TestCmd != "" && !opts.NoTest {
		testPath := filepath.Join(runDir, fmt.Sprintf("recovery-test-round-%d.txt", round))
		test, _ := runTestCommand(ctx, opts.Workdir, opts.TestCmd, opts.Timeout, testPath, opts.DryRun, opts.EnvOverlay, opts.MaxOutputBytes, opts.TestIdentityRegex)
		artifact.Test = &test
	}
	allowed := map[string]bool{"quarantine": true}
	if originalAdapter.Capabilities().SupportsResume && strings.TrimSpace(sessionID) != "" {
		allowed["repair"] = true
	}
	if policyAttemptsReplacement(opts.LossPolicy.Worker) && len(fallbackTargetsForRole(opts, roleLabelsEditor(opts.Mode), originalTarget)) > 0 {
		allowed["continue_with_fallback"] = true
	}
	decision := RecoveryDecision{SchemaVersion: ArtifactSchemaVersion, Decision: "quarantine", Reason: "recovery supervisor unavailable or invalid", Evidence: []string{failure.Error()}}
	decisionPath := filepath.Join(runDir, fmt.Sprintf("recovery-decision-round-%d.json", round))
	schemaPath := filepath.Join(runDir, "recovery-decision-schema.json")
	_ = writeFileDurable(schemaPath, []byte(RecoveryDecisionSchema+"\n"), 0o644, false)
	if reviewer != nil {
		result, decisionErr := a.runAdapter(ctx, reviewer, RoleSupervisor, Request{
			Context:         ctx,
			Prompt:          recoveryPrompt(failure, diff, artifact.Test, allowed),
			EnvOverlay:      opts.EnvOverlay,
			Model:           opts.Adversary.Model,
			Workdir:         opts.Workdir,
			RunDir:          runDir,
			OutputPath:      decisionPath,
			SchemaPath:      schemaPath,
			Timeout:         opts.Timeout,
			WatchdogTimeout: opts.WatchdogTimeout,
			Phase:           "repairing recovery decision",
			Quiet:           opts.Quiet,
			Verbose:         opts.Verbose,
			Budget:          opts.InvocationBudget,
			MaxOutputBytes:  opts.MaxOutputBytes,
		}, opts.DryRun)
		if decisionErr == nil {
			if parsed, parseErr := parseRecoveryDecision([]byte(result.Text), allowed); parseErr == nil {
				decision = parsed
			}
		}
	}
	artifact.Decision = decision
	artifact.UpdatedAt = time.Now().UTC()

	selectedTarget := originalTarget
	selectedAdapter := originalAdapter
	switch decision.Decision {
	case "repair":
		artifact.Status = "repairing"
	case "continue_with_fallback":
		selectedTarget, selectedAdapter, err = selectRuntimeWorkerFallback(ctx, opts, registry, originalTarget)
		if err != nil {
			decision.Decision = "quarantine"
			decision.Reason = err.Error()
			artifact.Decision = decision
			artifact.Status = "quarantined"
		}
	default:
		artifact.Status = "quarantined"
	}
	artifact.Selected = selectedTarget
	artifact.UpdatedAt = time.Now().UTC()
	_ = writeJSONWithNewline(filepath.Join(runDir, fmt.Sprintf("recovery-round-%d.json", round)), artifact)
	if artifact.Status == "quarantined" {
		final.Status = RunStatusQuarantined
		setFinalBlocking(final, ReasonQuarantined, decision.Reason)
		return Result{}, selectedTarget, selectedAdapter, &ExitError{Code: ExitAdapterFailure, Err: fmt.Errorf("partial work quarantined: %s", decision.Reason)}
	}

	retryPrompt := workerContractPrompt(fmt.Sprintf("Preserve the existing partial diff at %s. %s the requested work, address the failure and focused-test evidence, and return a valid worker result envelope.", diff.PatchPath, map[bool]string{true: "Repair", false: "Continue"}[decision.Decision == "repair"]))
	retryOutput := filepath.Join(runDir, fmt.Sprintf("worker-recovery-round-%d.json", round))
	result, retryErr := a.runAdapter(ctx, selectedAdapter, RoleCoder, Request{
		Context:               ctx,
		Prompt:                retryPrompt,
		EnvOverlay:            opts.EnvOverlay,
		Model:                 selectedTarget.Model,
		Workdir:               opts.Workdir,
		RunDir:                runDir,
		OutputPath:            retryOutput,
		SchemaPath:            workerSchemaPath,
		ResumeID:              map[bool]string{true: sessionID, false: ""}[decision.Decision == "repair"],
		Timeout:               opts.Timeout,
		WatchdogTimeout:       opts.WatchdogTimeout,
		Phase:                 "repairing worker continuation",
		Quiet:                 opts.Quiet,
		Verbose:               opts.Verbose,
		Budget:                opts.InvocationBudget,
		MaxOutputBytes:        opts.MaxOutputBytes,
		RequireWorkerContract: true,
	}, opts.DryRun)
	if retryErr != nil {
		artifact.Status = "quarantined"
		artifact.UpdatedAt = time.Now().UTC()
		_ = writeJSONWithNewline(filepath.Join(runDir, fmt.Sprintf("recovery-round-%d.json", round)), artifact)
		final.Status = RunStatusQuarantined
		setFinalBlocking(final, ReasonQuarantined, retryErr.Error())
		return Result{}, selectedTarget, selectedAdapter, retryErr
	}
	artifact.Status = "recovered"
	artifact.UpdatedAt = time.Now().UTC()
	_ = writeJSONWithNewline(filepath.Join(runDir, fmt.Sprintf("recovery-round-%d.json", round)), artifact)
	return result, selectedTarget, selectedAdapter, nil
}

func roleLabelsEditor(mode Mode) string {
	editor, _ := roleLabels(mode)
	return editor
}

func selectRuntimeWorkerFallback(ctx context.Context, opts RunOptions, registry map[string]Adapter, primary RoleTarget) (RoleTarget, Adapter, error) {
	for _, raw := range fallbackTargetsForRole(opts, roleLabelsEditor(opts.Mode), primary) {
		target, err := ParseRoleTarget(raw)
		if err != nil || roleTargetString(target) == roleTargetString(primary) {
			continue
		}
		adapter := registry[target.Adapter]
		if adapter == nil || checkAdapters(ctx, adapter) != nil {
			continue
		}
		if _, err := adapter.BuildCmd(RoleCoder, Request{Workdir: opts.Workdir, Model: target.Model, Timeout: opts.Timeout}); err != nil {
			continue
		}
		return target, adapter, nil
	}
	return RoleTarget{}, nil, fmt.Errorf("no runnable worker fallback remains")
}
