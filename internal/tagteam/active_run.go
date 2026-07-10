package tagteam

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ActiveRun is a lightweight pointer to a run that is still in progress. It is
// written to .tagteam/active.json immediately after the run directory is
// created so a standalone reader (e.g. the TUI) can discover an in-flight run
// without waiting for latest.json, which is only written once a run reaches a
// terminal state.
type ActiveRun struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	RunDir        string    `json:"run_dir"`
	StatePath     string    `json:"state_path"`
	FinalPath     string    `json:"final_path"`
	Mode          Mode      `json:"mode,omitempty"`
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"started_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func activeRunPath(workdir string) string {
	return statePathForWorkdir(workdir, "active.json")
}

func writeActiveRun(workdir string, active ActiveRun) error {
	active.SchemaVersion = ArtifactSchemaVersion
	active.UpdatedAt = time.Now().UTC()
	return writeJSONWithNewline(activeRunPath(workdir), active)
}

func readActiveRun(workdir string) (ActiveRun, error) {
	var active ActiveRun
	data, err := os.ReadFile(activeRunPath(workdir))
	if err != nil {
		return ActiveRun{}, err
	}
	if err := json.Unmarshal(data, &active); err != nil {
		return ActiveRun{}, err
	}
	return active, nil
}

// finalizeActiveRun updates the active-run pointer's status in place. It is a
// no-op if the pointer is missing, corrupt, or already claimed by a different
// (newer) run -- a stale run must never clobber a run that superseded it.
func finalizeActiveRun(workdir, runID, status string) error {
	active, err := readActiveRun(workdir)
	if err != nil || active.RunID != runID {
		return nil
	}
	active.Status = status
	active.UpdatedAt = time.Now().UTC()
	return writeJSONWithNewline(activeRunPath(workdir), active)
}

// clearActiveRun removes the active-run pointer, but only when it still names
// runID.
func clearActiveRun(workdir, runID string) error {
	active, err := readActiveRun(workdir)
	if err != nil || active.RunID != runID {
		return nil
	}
	if err := os.Remove(activeRunPath(workdir)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// activateRun writes the running active-run pointer for a freshly created run
// directory.
func activateRun(workdir, runID, runDir string, mode Mode) {
	_ = writeActiveRun(workdir, ActiveRun{
		RunID:     runID,
		RunDir:    runDir,
		StatePath: filepath.Join(runDir, "state.json"),
		FinalPath: filepath.Join(runDir, "final.json"),
		Mode:      mode,
		Status:    "running",
		StartedAt: time.Now().UTC(),
	})
}

// deactivateRun finalizes the active-run pointer once a run stops executing.
// completed reflects whether the run reached its normal completion point in
// the main code path (regardless of the run's exit code/verdict); false means
// the process aborted through an error path, so the pointer is marked failed
// instead of silently staying "running".
func deactivateRun(workdir, runID string, completed bool) {
	if completed {
		_ = clearActiveRun(workdir, runID)
		return
	}
	_ = finalizeActiveRun(workdir, runID, "failed")
}
