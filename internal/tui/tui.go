// Package tui renders read-only Tribunal RunSnapshots.
package tui

import (
	"fmt"
	"strings"

	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

func RenderSnapshot(snapshot storage.Snapshot) string {
	var out strings.Builder
	fmt.Fprintf(&out, "TRIBUNAL  %s\n", snapshot.State.RunID)
	fmt.Fprintf(&out, "Phase:  %s\nStatus: %s\n", snapshot.State.Phase, snapshot.State.Status)
	if snapshot.Waiting {
		fmt.Fprintf(&out, "Waiting for lock held by PID %d\n", snapshot.WaitPID)
	}
	if snapshot.Final != nil {
		fmt.Fprintf(&out, "\n%s\nFindings: %d  Decisions: %d  Pending: %d\n", snapshot.Final.Summary, len(snapshot.Final.Findings), len(snapshot.Final.Decisions), len(snapshot.Final.Arbitration))
	}
	return strings.TrimRight(out.String(), "\n")
}
