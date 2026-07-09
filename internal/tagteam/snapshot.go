package tagteam

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// BuildRunSnapshot assembles a RunSnapshot for runDir by reading whichever of
// active.json (in workdir), state.json, final.json, and plan.json exist. Only
// runDir itself must exist; every artifact inside it is optional, and a
// missing/unreadable one simply leaves the corresponding fields at their zero
// value instead of failing snapshot creation.
func BuildRunSnapshot(workdir, runDir string) (RunSnapshot, error) {
	info, err := os.Stat(runDir)
	if err != nil {
		return RunSnapshot{}, err
	}
	if !info.IsDir() {
		return RunSnapshot{}, &ExitError{Code: ExitInvalidArguments, Err: fmt.Errorf("%s is not a run directory", runDir)}
	}

	snapshot := RunSnapshot{
		SchemaVersion: ArtifactSchemaVersion,
		RunID:         filepath.Base(runDir),
		RunDir:        runDir,
	}

	if active, err := readActiveRun(workdir); err == nil && active.RunID == snapshot.RunID {
		snapshot.Mode = active.Mode
		snapshot.Status = active.Status
		snapshot.UpdatedAt = active.UpdatedAt
	}

	if state, err := readRunState(runDir); err == nil {
		if snapshot.Mode == "" {
			snapshot.Mode = state.Mode
		}
		if state.Status != "" {
			snapshot.Status = state.Status
		}
		snapshot.Phase = state.Phase
		snapshot.Degraded = state.Degraded
		snapshot.DegradedReason = state.DegradedReason
		snapshot.BlockingReason = state.BlockingReason
		snapshot.RoleStatuses = state.RoleStatuses
		snapshot.CurrentRound = state.CurrentRound
		snapshot.LatestDiffPath = state.LatestDiffPath
		snapshot.LatestReviewPath = state.LatestReviewPath
		snapshot.ExitCode = state.ExitCode
		if state.UpdatedAt.After(snapshot.UpdatedAt) {
			snapshot.UpdatedAt = state.UpdatedAt
		}
	}

	var finalReview *Review
	if final, err := readFinal(filepath.Join(runDir, "final.json")); err == nil && final.RunID != "" {
		if snapshot.Mode == "" {
			snapshot.Mode = final.Mode
		}
		snapshot.Status = string(final.Status)
		snapshot.Phase = final.Phase
		snapshot.Verdict = final.Verdict
		snapshot.ExitCode = final.ExitCode
		snapshot.Degraded = final.Degraded
		snapshot.DegradedReason = final.DegradedReason
		snapshot.BlockingReason = final.BlockingReason
		if len(final.RoleStatuses) > 0 {
			snapshot.RoleStatuses = final.RoleStatuses
		}
		snapshot.RoundsCompleted = final.RoundsCompleted
		snapshot.RoundsRequested = final.RoundsRequested
		if len(final.ChangedFiles) > 0 {
			snapshot.ChangedFiles = final.ChangedFiles
		}
		if final.LatestDiffPath != "" {
			snapshot.LatestDiffPath = final.LatestDiffPath
		}
		if final.LatestReviewPath != "" {
			snapshot.LatestReviewPath = final.LatestReviewPath
		}
		finalReview = final.Review
		if !final.FinishedAt.IsZero() && final.FinishedAt.After(snapshot.UpdatedAt) {
			snapshot.UpdatedAt = final.FinishedAt
		}
	}

	if plan, err := readExecutionPlan(runDir); err == nil && plan.RunID != "" {
		snapshot.PlanSummary = summarizeExecutionPlan(runDir, &plan)
	}

	if len(snapshot.ChangedFiles) == 0 && snapshot.LatestDiffPath != "" {
		snapshot.ChangedFiles = readChangedFilesFromDiffPath(snapshot.LatestDiffPath)
	}

	if finalReview != nil {
		snapshot.FindingsCount = len(finalReview.Findings)
	} else if snapshot.LatestReviewPath != "" {
		if review, err := readReviewArtifact(snapshot.LatestReviewPath); err == nil {
			snapshot.FindingsCount = len(review.Findings)
		}
	}

	if testPath := latestTestOutputPath(runDir); testPath != "" {
		snapshot.LatestTestPath = testPath
	}

	return snapshot, nil
}

func readRunState(runDir string) (RunState, error) {
	var state RunState
	data, err := os.ReadFile(filepath.Join(runDir, "state.json"))
	if err != nil {
		return RunState{}, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return RunState{}, err
	}
	return state, nil
}

func readReviewArtifact(path string) (Review, error) {
	var review Review
	data, err := os.ReadFile(path)
	if err != nil {
		return Review{}, err
	}
	if err := json.Unmarshal(data, &review); err != nil {
		return Review{}, err
	}
	return review, nil
}

// readChangedFilesFromDiffPath derives the sibling "<round>.files.json"
// metadata path from a "diff-round-N.patch" path (see captureDiffArtifact)
// and returns the changed file list it records, if the file exists and
// parses.
func readChangedFilesFromDiffPath(diffPath string) []string {
	if !strings.HasSuffix(diffPath, ".patch") {
		return nil
	}
	filesPath := strings.TrimSuffix(diffPath, ".patch") + ".files.json"
	data, err := os.ReadFile(filesPath)
	if err != nil {
		return nil
	}
	var metadata DiffFilesMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil
	}
	files := make([]string, 0, len(metadata.Files))
	for _, file := range metadata.Files {
		files = append(files, file.Path)
	}
	return files
}

// latestTestOutputPath returns the highest-round "test-round-N.txt" artifact
// in runDir, if any (see runTestCommand's output naming).
func latestTestOutputPath(runDir string) string {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return ""
	}
	best := ""
	bestRound := -1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "test-round-") || !strings.HasSuffix(name, ".txt") {
			continue
		}
		roundStr := strings.TrimSuffix(strings.TrimPrefix(name, "test-round-"), ".txt")
		round, err := strconv.Atoi(roundStr)
		if err != nil {
			continue
		}
		if round > bestRound {
			bestRound = round
			best = filepath.Join(runDir, name)
		}
	}
	return best
}
