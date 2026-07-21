package tui

import (
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

func TestRenderSnapshotUsesOnlySnapshot(t *testing.T) {
	snapshot := storage.Snapshot{SchemaVersion: 1, State: domain.RunState{RunID: "01RUN", Phase: domain.PhaseVoting, Status: "running"}, Waiting: true, WaitPID: 42}
	output := RenderSnapshot(snapshot)
	for _, want := range []string{"01RUN", "VOTING", "running", "PID 42"} {
		if !strings.Contains(output, want) {
			t.Fatalf("RenderSnapshot() missing %q: %s", want, output)
		}
	}
}
