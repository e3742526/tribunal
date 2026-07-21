# Build State

- Project: Tribunal v0.1.0
- Current gate: 8 — final acceptance and handoff
- Last update: Gate 7 documentation/vendor-contract verification complete
- Status: final acceptance in progress
- Waypoint: `2ba8980` (Gate 6)

## Verified facts

- The starting worktree was clean at the waypoint above (Gate 0 evidence).
- `go test ./...` passed on the inherited baseline; core package 107.873s
  (Gate 0 evidence).
- Module, binary, configuration, CLI, state, CI, release metadata, and user
  documentation now use Tribunal identity only.
- The local HTTP CLI E2E succeeds with Git absent and no review-time workspace
  writes.
- Focused package tests, full tests, vet, build, module verification, and the
  800-line gate pass. `govulncheck` is unavailable locally.
- A bounded authenticated three-family smoke reached blind voting with Codex
  and Agy valid, Claude invalid-output isolated, quorum met, two accepted
  recommendations, and one arbitration item (exit 2).

## Work in flight

Gate 8 is rerunning the complete gate from a clean worktree and recording the
final QA and handoff state.

## Next actions

1. Run `scripts/check.sh`, race, scans, and clean-checkout quickstart.
2. Close or explicitly rescope every remaining evidence gap.
3. Produce the Gate 8 acceptance commit and verify a clean worktree.

## Open items

- D-001 through D-018 are fixed with regression evidence.
- Linux runtime archive smoke, Windows multiprocess locks, and unavailable
  local third-party QA tools remain explicit evidence gaps.

## Resume verification

Run `git status --short`, read this file plus `docs/INTENT.md`, then run
`go test ./...`. Adopt this state only if the current slice compiles or is
described here accurately.
