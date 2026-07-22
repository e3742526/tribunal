# Consensus/arbitration playtest — 2026-07-23

Skill: `audit-playtest-app`, adapted for a CLI (no GUI surface). Target:
Tribunal's panel-voting and consensus-resolution logic, end to end through
`Service.Review`/`Service.Arbitrate`/`domain.ResolveVotes`.

## Scope and method

No `docs/test_scenarios/` library existed. Created one:
- `docs/test_scenarios/README.md` — library index, safety scope, coverage checklist
- `docs/test_scenarios/01-review-workflow.md` — 8 CLI-lifecycle cards (launch,
  resume, persistence, invalid input, settings, navigation, interruption)
- `docs/test_scenarios/02-consensus-scenarios.md` — 52 consensus/voting cards
  (S00, S01-S52), 8 explicitly marked `[DISAGREE->CONSENSUS]`

Runnable driver: `internal/tribunal/app/consensus_playtest_test.go` (32
pure vote-arithmetic subtests calling `domain.ResolveVotes` directly — the
same computation `Service.Review`'s panel pipeline performs) and
`internal/tribunal/app/consensus_playtest_e2e_test.go` (19 tests running the
real `Service.Review`/`Service.Arbitrate`/`Service.Replay` through
`adapters.FuncAdapter`, the same injection point used for real provider
CLIs). Total: **51 individually-asserted scenarios**, exceeding the
50-scenario request.

No real provider credentials, no network calls; every scenario runs in
`t.TempDir()` state roots against scripted deterministic reviewer/voter
responses — real code paths, fake models.

## Result: zero product bugs in the 51 scenarios as authored

Every scenario that failed on first run failed because of a harness
authoring mistake, not a product defect:

1. **Dissent-count expectations wrong in 5 vote-arithmetic cards** (S03,
   S08, S27, S45, S44) — I under-counted `Decision.Dissent`; the actual rule
   (`severityDivergence || rejectsAccepted`) flags *every* reject vote on an
   accepted outcome, not just severity-mismatched ones. Corrected the test
   expectations; the product behavior was already correct.
2. **Panelist lookup keyed on the wrong field** in the e2e harness — the
   scripted-response table matched `panelist.ID`, but `domain.ParsePanel`
   always assigns `R-001`/`R-002`/... sequentially regardless of the panel
   string; the scripted identity survives in `panelist.Model`. Fixed the
   harness.
3. **`Findings: nil` serialized as JSON `null`**, which the review schema's
   `"findings": {"type":"array"}` rejects — an unscripted reviewer's "raise
   nothing" case needs `[]domain.Finding{}`. Fixed the harness.
4. **Test asserted the wrong exit code** for `Arbitrate` ruling "accepted"
   on a major finding — that legitimately returns `ExitBlockingFindings`
   (exit 1), not `ExitSuccess`; the exit-code contract was correct, the test
   assertion wasn't.

## Result: one real product bug found and repaired

**D-070** — `defaultRecommendation` (the text `RankArbitration` attaches to
each `ArbitrationDispute.Default`, and the exact string
`arbitrationRulings`' `--accept-majority` path parses for its "accept"/
"reject" prefix) derived its recommendation from raw, unweighted
`Accepts`/`Rejects` vote counts. Under a non-uniform panel weighting, those
raw counts can diverge from the actual weighted comparison that produced
the decision's outcome. Concretely: a senior reviewer (weight 2.0) rejects
while two junior reviewers (weight 1.0 each) accept — raw counts read
2-accept/1-reject, but the weighted comparison is an exact 2.0/2.0 tie. The
dispute's `Default` read `"accept majority"` on a decision the panel had
*not* actually leaned toward accepting, and a human operator running
`tribunal arbitrate --accept-majority` would have silently auto-accepted a
genuine tie instead of it routing to them for judgment — a consensus-
integrity bug (a false "majority" bypassing intended human arbitration),
not a cosmetic one.

**First fix attempt** (special-case `Decision.Reason == "vote_tie"`) was
caught incomplete by adversarial review: two sibling `Reason` values
(`category_requires_full_panel_unanimity`, `unanimity_not_reached`) can
co-occur with the identical raw/weighted divergence and were still
mislabeled. **Second, correct fix**: added `Decision.WeightedLean`
("accept"/"reject"/"tie"), computed once directly from the weighted sums
inside `ResolveVotes` before the outcome switch runs, so it is correct for
every `Reason` structurally, not by enumeration. `defaultRecommendation` now
reads `WeightedLean` exclusively. `ValidateDecision` fails closed on any
decision missing a valid `WeightedLean` (protects against stale/pre-fix
persisted state). A second adversarial review confirmed no remaining
bypass; a follow-up regression test was added for the third sibling case
(`unanimity_not_reached`) that the second review flagged as untested
(though structurally covered).

Files changed: `internal/tribunal/domain/{consensus,types,validation}.go`,
plus a pre-existing hand-built `Decision` literal in
`internal/tribunal/app/edit_test.go` updated for the new required field.

Regression tests: `TestConsensusPlaytest_WeightedTieNeverReadsAsMajority`,
`TestConsensusPlaytest_StrictCategoryWeightedMismatchNeverReadsAsMajority`,
`TestConsensusPlaytest_UnanimityNotReachedWeightedMismatchNeverReadsAsMajority`
(`internal/tribunal/app/consensus_playtest_test.go`).

Ledger: D-070 in `docs/build/defect-ledger.md`.

## Disagree-then-consensus coverage (explicit ask)

8 scenarios exercise a first-pass panel disagreement (arbitration outcome)
resolved by a follow-on step, exceeding the 5-minimum:

- **S05, S06, S07, S42** — vote ties (unweighted and weighted) producing
  `arbitration/vote_tie`, resolved by operator ruling or decision replay.
- **S38** — explicit interactive-shaped operator ruling (reject) resolves a
  tied dispute to a final consensus; `Final.Arbitration` empties, decision
  memory records the ruling.
- **S37** — `--accept-majority` with a mixed accept/reject-leaning dispute
  set and one exception, each dispute resolved independently.
- **S35/S36** — decision-memory match sets `MemoryHint` (not `Default`) on
  a replayed disputed run; legacy pre-`MemoryHint` persisted format still
  honored by `--accept-majority`.
- **S52** — replay of a disputed run deterministically reproduces the same
  tie (not "gets lucky" and resolves).

Explicit 2/3-agree happy path: **S00** — three reviewers, two accept one
reject, non-strict category, resolves to `accepted/majority_accept` with the
dissenting reject vote correctly recorded in `Decision.Dissent`.

## Verification

- `scripts/check.sh`: green (format, 800-line limits, architecture
  conformance, all tests, vet, build, module verify, tidy -diff).
- `go test -race -count=1 ./...`: green across all 11 packages.
- `gofmt -l .`: clean.
- Two independent adversarial-review passes on the D-070 fix (first caught
  an incomplete fix; second confirmed the corrected version closes every
  bypass and flagged one untested-but-structurally-covered case, closed
  with an added regression test).

## Residual scope not covered by this pass

- The `01-review-workflow.md` cards (CLI lifecycle: resume, defer, status/
  transcript/explain navigation, degraded-panel interruption) are
  documented as a standing library but were not run as part of this pass —
  this session's playtest budget went to the consensus/voting request
  specifically. Running that file is the natural next playtest pass.
- Real provider-CLI behavior (actual `codex`/`claude`/`agy` process
  invocation, network-dependent evidence verification) remains outside this
  pass's scope, consistent with prior campaigns' documented residuals.
- `defaultRecommendation`'s text is still a coarse two-way/tie summary; it
  does not weight-adjust the displayed vote tally shown alongside it in
  `report.md` (only `Default` itself was in scope for this bug).
