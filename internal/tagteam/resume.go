package tagteam

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ResumeRecord struct {
	SchemaVersion  int       `json:"schema_version"`
	SourceRunID    string    `json:"source_run_id"`
	ContinuedRunID string    `json:"continued_run_id,omitempty"`
	VerifiedPhase  RunPhase  `json:"verified_phase"`
	Baseline       string    `json:"baseline"`
	DiffHash       string    `json:"diff_hash,omitempty"`
	Status         string    `json:"status"`
	Message        string    `json:"message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// Resume verifies an interrupted run and continues it in a linked run. The
// source run remains immutable evidence and the continuation names its source.
func (a *App) Resume(ctx context.Context, opts RunOptions, runID string) (FinalRun, error) {
	locator, err := resolveStateLocator(opts.Workdir, opts.StateRoot)
	if err != nil {
		return FinalRun{}, &ExitError{Code: ExitPreflightFailed, Err: err}
	}
	if err := locator.Prepare(); err != nil {
		return FinalRun{}, &ExitError{Code: ExitAdapterFailure, Err: err}
	}
	runDir, err := locator.RunDir(runID)
	if err != nil {
		return FinalRun{}, &ExitError{Code: ExitInvalidArguments, Err: err}
	}
	if info, statErr := os.Stat(runDir); statErr != nil || !info.IsDir() {
		return FinalRun{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("run %q not found", runID)}
	}
	lock, err := acquireRunLock(runDir, true)
	if err != nil {
		return FinalRun{}, &ExitError{Code: ExitPreflightFailed, Err: err}
	}
	defer lock.Release()
	state, err := readRunState(runDir)
	if err != nil {
		return FinalRun{}, &ExitError{Code: ExitPreflightFailed, Err: fmt.Errorf("read resumable state: %w", err)}
	}
	if state.SchemaVersion < runStateSchemaVersion {
		return FinalRun{}, &ExitError{Code: ExitPreflightFailed, Err: fmt.Errorf("legacy run state schema %d is readable but not resumable", state.SchemaVersion)}
	}
	meta, err := readMeta(filepath.Join(runDir, "meta.json"))
	if err != nil {
		return FinalRun{}, &ExitError{Code: ExitPreflightFailed, Err: fmt.Errorf("read run metadata: %w", err)}
	}
	currentHead, err := ensureGitRepo(opts.Workdir)
	if err != nil {
		return FinalRun{}, err
	}
	if currentHead != meta.Baseline {
		return quarantineResume(runDir, state, fmt.Errorf("current HEAD %s does not match run baseline %s", currentHead, meta.Baseline))
	}
	patch, err := deterministicDiffPatch(ctx, opts.Workdir, meta.Baseline, filepath.Join(runDir, "resume-verify.index"))
	if err != nil {
		return FinalRun{}, err
	}
	currentDiffHash := sha256Sum(patch)
	phase := normalizeRunPhase(state.Phase)
	if state.DiffHash != "" && state.DiffHash != currentDiffHash && phase != PhaseImplementing && phase != PhaseRepairing {
		return quarantineResume(runDir, state, fmt.Errorf("worktree diff hash changed after completed %s phase", phase))
	}
	prompt, err := readRunPrompt(runDir, opts.Prompt)
	if err != nil {
		return FinalRun{}, err
	}
	saved, _ := readFinal(filepath.Join(runDir, "final.json"))
	if saved.Status == RunStatusPassed || saved.Status == RunStatusDegraded {
		return saved, nil
	}
	opts.Prompt = prompt
	opts.Baseline = meta.Baseline
	opts.SkipDirtyCheck = true
	opts.AllowDirty = true
	opts.ResumedFrom = runID
	if saved.Mode != "" {
		opts.Mode = saved.Mode
	}
	if saved.Coder.Adapter != "" {
		opts.Coder = saved.Coder
	}
	if saved.Adversary.Adapter != "" {
		opts.Adversary = saved.Adversary
	}
	if saved.Scout.Adapter != "" {
		opts.Scout = saved.Scout
	}
	if opts.Mode == "" {
		opts.Mode = metaMode(meta)
	}
	if opts.Coder.Adapter == "" || (opts.Mode != ModeSolo && opts.Adversary.Adapter == "") {
		restoreTargetsFromMeta(&opts, meta)
	}
	record := ResumeRecord{SchemaVersion: ArtifactSchemaVersion, SourceRunID: runID, VerifiedPhase: phase, Baseline: meta.Baseline, DiffHash: currentDiffHash, Status: "verified", CreatedAt: time.Now().UTC()}
	_ = writeJSONWithNewline(filepath.Join(runDir, "resume.json"), record)
	var prior *Review
	if state.LatestReviewPath != "" {
		if review, reviewErr := readReviewArtifact(state.LatestReviewPath); reviewErr == nil {
			prior = &review
		}
	}
	var continued FinalRun
	if opts.Mode == ModeSolo {
		continued, err = a.runSolo(ctx, opts)
	} else {
		continued, err = a.runLoop(ctx, opts, prior)
	}
	record.ContinuedRunID = continued.RunID
	record.Status = "continued"
	if err != nil {
		record.Status = "continuation_failed"
		record.Message = err.Error()
	}
	_ = writeJSONWithNewline(filepath.Join(runDir, "resume.json"), record)
	state.RecoveryStatus = "continued_as:" + continued.RunID
	_ = writeRunState(runDir, state)
	return continued, err
}

func quarantineResume(runDir string, state RunState, cause error) (FinalRun, error) {
	state.Status = string(RunStatusQuarantined)
	state.RecoveryStatus = cause.Error()
	_ = writeRunState(runDir, state)
	record := ResumeRecord{SchemaVersion: ArtifactSchemaVersion, SourceRunID: state.RunID, VerifiedPhase: normalizeRunPhase(state.Phase), Baseline: state.BaselineSHA, DiffHash: state.DiffHash, Status: "quarantined", Message: cause.Error(), CreatedAt: time.Now().UTC()}
	_ = writeJSONWithNewline(filepath.Join(runDir, "resume.json"), record)
	final := FinalRun{RunID: state.RunID, RunDir: runDir, Workdir: state.Workdir, Baseline: state.BaselineSHA, Mode: state.Mode, Status: RunStatusQuarantined, Verdict: "quarantined", BlockingReason: string(ReasonQuarantined), ExitCode: ExitAdapterFailure}
	return final, &ExitError{Code: ExitAdapterFailure, Err: cause}
}

func metaMode(meta Meta) Mode {
	if _, ok := meta.Adapters["solo"]; ok {
		return ModeSolo
	}
	if _, ok := meta.Adapters["worker"]; ok {
		return ModeSupervisor
	}
	if _, ok := meta.Adapters["scout"]; ok {
		return ModeRelay
	}
	return ModeAdversarial
}

func restoreTargetsFromMeta(opts *RunOptions, meta Meta) {
	editor, reviewer := roleLabels(opts.Mode)
	if adapter := strings.TrimSpace(meta.Adapters[editor]); adapter != "" {
		opts.Coder = RoleTarget{Adapter: adapter, Model: meta.Models[editor]}
	}
	if opts.Mode != ModeSolo {
		if adapter := strings.TrimSpace(meta.Adapters[reviewer]); adapter != "" {
			opts.Adversary = RoleTarget{Adapter: adapter, Model: meta.Models[reviewer]}
		}
	}
	if opts.Mode == ModeRelay {
		if adapter := strings.TrimSpace(meta.Adapters["scout"]); adapter != "" {
			opts.Scout = RoleTarget{Adapter: adapter, Model: meta.Models["scout"]}
		}
	}
}
