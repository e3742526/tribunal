// Package tui implements the read-only "tagteam tui" terminal view: it polls
// the on-disk run artifacts (via tagteam.BuildRunSnapshot and the plan
// reader) and renders them. It never invokes an adapter or writes to a run
// directory.
package tui

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/cephalopod-ai/tagteam/internal/tagteam"
)

// Toggles tracks which optional panels are currently shown.
type Toggles struct {
	ShowPlan      bool
	ShowFindings  bool
	ShowArtifacts bool
}

// DefaultToggles returns the MVP's initial panel visibility: everything on.
func DefaultToggles() Toggles {
	return Toggles{ShowPlan: true, ShowFindings: true, ShowArtifacts: true}
}

// Render writes a stable, plain-text view of snapshot (and plan, if given) to
// w. It performs no I/O of its own and reads no wall-clock time, so the same
// inputs always produce the same output.
func Render(w io.Writer, snapshot tagteam.RunSnapshot, plan *tagteam.ExecutionPlan, toggles Toggles) {
	fmt.Fprintf(w, "tagteam tui  run=%s mode=%s status=%s phase=%s verdict=%s exit=%d\n",
		snapshot.RunID, snapshot.Mode, orDash(snapshot.Status), orDash(snapshot.Phase), orDash(snapshot.Verdict), snapshot.ExitCode)
	if snapshot.Degraded {
		fmt.Fprintf(w, "degraded=true reason=%s\n", snapshot.DegradedReason)
	}
	if snapshot.BlockingReason != "" {
		fmt.Fprintf(w, "blocking_reason=%s\n", snapshot.BlockingReason)
	}
	fmt.Fprintf(w, "round=%d rounds=%d/%d updated=%s\n",
		snapshot.CurrentRound, snapshot.RoundsCompleted, snapshot.RoundsRequested, formatTime(snapshot.UpdatedAt))
	fmt.Fprintln(w, strings.Repeat("-", 60))

	renderRoleStatuses(w, snapshot.RoleStatuses)

	if toggles.ShowPlan {
		renderPlan(w, plan)
	}
	if toggles.ShowFindings {
		renderFindings(w, snapshot)
	}
	if toggles.ShowArtifacts {
		renderArtifacts(w, snapshot)
	}

	fmt.Fprintln(w, strings.Repeat("-", 60))
	fmt.Fprintln(w, "[q] quit  [r] refresh  [p] plan  [f] findings  [d] files/artifacts")
}

func renderRoleStatuses(w io.Writer, roles map[string]tagteam.RoleStatus) {
	fmt.Fprintln(w, "Roles:")
	if len(roles) == 0 {
		fmt.Fprintln(w, "  (none reported yet)")
		return
	}
	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		role := roles[name]
		line := fmt.Sprintf("  %-12s status=%-10s adapter=%s", name, orDash(role.Status), orDash(role.Adapter))
		if role.Model != "" {
			line += " model=" + role.Model
		}
		if role.Message != "" {
			line += " message=" + role.Message
		}
		fmt.Fprintln(w, line)
	}
}

func renderPlan(w io.Writer, plan *tagteam.ExecutionPlan) {
	if plan == nil {
		return
	}
	fmt.Fprintf(w, "Plan (run=%s status=%s items=%d):\n", plan.RunID, plan.Status, len(plan.Items))
	for _, item := range plan.Items {
		fmt.Fprintf(w, "  [%s] %-17s %s\n", item.ID, item.Status, item.Title)
	}
}

func renderFindings(w io.Writer, snapshot tagteam.RunSnapshot) {
	fmt.Fprintf(w, "Findings: %d\n", snapshot.FindingsCount)
}

func renderArtifacts(w io.Writer, snapshot tagteam.RunSnapshot) {
	fmt.Fprintf(w, "Changed files (%d):\n", len(snapshot.ChangedFiles))
	for _, file := range snapshot.ChangedFiles {
		fmt.Fprintf(w, "  %s\n", file)
	}
	fmt.Fprintln(w, "Artifacts:")
	fmt.Fprintf(w, "  diff:   %s\n", orDash(snapshot.LatestDiffPath))
	fmt.Fprintf(w, "  review: %s\n", orDash(snapshot.LatestReviewPath))
	fmt.Fprintf(w, "  test:   %s\n", orDash(snapshot.LatestTestPath))
	fmt.Fprintf(w, "  run_dir: %s\n", snapshot.RunDir)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}
