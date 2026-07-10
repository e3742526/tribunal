package tagteam

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	liveProgressArtifact        = "live-progress.json"
	preexistingWorktreeArtifact = "preexisting-worktree.json"
)

type PreexistingWorktree struct {
	SchemaVersion int       `json:"schema_version"`
	Baseline      string    `json:"baseline"`
	Files         []string  `json:"files"`
	CapturedAt    time.Time `json:"captured_at"`
}

// LiveProgress is host-owned runtime evidence collected while an adapter is
// active. Agents never write this artifact.
type LiveProgress struct {
	SchemaVersion  int       `json:"schema_version"`
	InvocationID   string    `json:"invocation_id,omitempty"`
	Phase          string    `json:"phase"`
	Role           Role      `json:"role"`
	Status         string    `json:"status"`
	Elapsed        string    `json:"elapsed"`
	FilesChanged   int       `json:"files_changed"`
	Additions      int       `json:"additions"`
	Deletions      int       `json:"deletions"`
	ChangedFiles   []string  `json:"changed_files,omitempty"`
	DiffHash       string    `json:"diff_hash,omitempty"`
	StdoutBytes    int64     `json:"stdout_bytes,omitempty"`
	StderrBytes    int64     `json:"stderr_bytes,omitempty"`
	LastActivityAt time.Time `json:"last_activity_at,omitempty"`
	NoProgressFor  string    `json:"no_progress_for,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func writeLiveProgress(
	ctx context.Context,
	req Request,
	role Role,
	phase string,
	started time.Time,
	status string,
) (LiveProgress, error) {
	progress := LiveProgress{
		SchemaVersion: ArtifactSchemaVersion,
		InvocationID:  req.InvocationID,
		Phase:         phase,
		Role:          role,
		Status:        status,
		Elapsed:       shortDuration(time.Since(started)),
		UpdatedAt:     time.Now().UTC(),
	}
	if req.ProgressStdout != nil {
		progress.StdoutBytes = req.ProgressStdout.Received()
	}
	if req.ProgressStderr != nil {
		progress.StderrBytes = req.ProgressStderr.Received()
	}
	if req.ProgressLastActivity != nil {
		progress.LastActivityAt = *req.ProgressLastActivity
		progress.NoProgressFor = shortDuration(time.Since(*req.ProgressLastActivity))
	}
	if req.Workdir != "" {
		diff, err := captureLiveDiff(ctx, req.Workdir)
		if err != nil {
			return progress, err
		}
		progress.ChangedFiles = diff.files
		progress.FilesChanged = len(diff.files)
		progress.Additions = diff.additions
		progress.Deletions = diff.deletions
		progress.DiffHash = sha256Sum(diff.patch)
	}
	if req.RunDir == "" {
		return progress, nil
	}
	if err := writeJSONAtomic(filepath.Join(req.RunDir, liveProgressArtifact), progress); err != nil {
		return progress, err
	}
	return progress, nil
}

type liveDiff struct {
	patch     []byte
	files     []string
	additions int
	deletions int
}

func captureLiveDiff(ctx context.Context, workdir string) (liveDiff, error) {
	tempDir, err := os.MkdirTemp("", "tagteam-live-diff-*")
	if err != nil {
		return liveDiff{}, err
	}
	defer os.RemoveAll(tempDir)
	patch, _, statusZ, numstatZ, err := deterministicDiffOutputs(ctx, workdir, "HEAD", filepath.Join(tempDir, "index"))
	if err != nil {
		return liveDiff{}, err
	}
	diff := liveDiff{patch: patch}
	for _, file := range buildDiffFiles(statusZ, numstatZ) {
		diff.files = append(diff.files, file.Path)
		diff.additions += file.Additions
		diff.deletions += file.Deletions
	}
	return diff, nil
}

func writeJSONAtomic(path string, value any) error {
	return writeJSONDurable(path, value, true, true)
}

func writePreexistingWorktree(ctx context.Context, workdir, runDir, baseline string) error {
	snapshot, err := captureWorktreeSnapshot(ctx, workdir)
	if err != nil {
		return err
	}
	files := make([]string, 0, len(snapshot))
	for path := range snapshot {
		files = append(files, path)
	}
	sort.Strings(files)
	return writeJSONWithNewline(filepath.Join(runDir, preexistingWorktreeArtifact), PreexistingWorktree{
		SchemaVersion: ArtifactSchemaVersion,
		Baseline:      baseline,
		Files:         files,
		CapturedAt:    time.Now().UTC(),
	})
}
