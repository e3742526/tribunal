# 01 — Review Workflow (CLI Lifecycle)

Cards exercising the review lifecycle at the CLI/application boundary:
launch, persistence, resume, errors, settings, navigation, interruption.

---

### 01-C1 — First review on a fresh state root

## User Goal

Run `tribunal review` against a document for the first time with no prior
state, and see a sensible, complete result.

## Category

Empty state / Happy path

## Preconditions

- Fresh `--state-root` (t.TempDir()), no prior runs for this workspace.
- A short Markdown document with one clearly unsupported claim.

## Inputs

- `tribunal review brief.md` with a 3-reviewer panel, all reviewers healthy.

## Steps

1. Build a packet from the document.
2. Run `Service.Review` with a scripted panel that finds one major finding
   and unanimously accepts it.
3. Inspect the returned `domain.Final` and the run directory artifacts.

## Expected Result

Exit code 1 (blocking findings), `final.Status == "findings"`, one accepted
decision, workspace `findings.json`/`latest.json`/`active.json` created,
document itself untouched.

## Observe

- Whether `packet.json`, `meta.json`, `final.json`, `votes.json`,
  `arbitration.json` all exist and are internally consistent.
- Whether the source document byte-for-byte survives the run.

## Variations

- Empty document / zero findings.

---

### 01-C2 — Resume a completed run (idempotent)

## User Goal

Resume a run that already finished, expecting a fast idempotent replay of
the same outcome with no duplicate provider calls.

## Category

Save-reopen / Happy path

## Preconditions

- A completed run from 01-C1.

## Inputs

- `tribunal resume` on the completed run ID.

## Steps

1. Complete a review (01-C1).
2. Call `Service.Resume` on the same run.
3. Compare the resumed `Final` to the original.

## Expected Result

Identical `RunID`, `PacketHash`, exit code, and decisions; zero additional
provider invocations.

## Observe

- Provider call counter before/after resume.

---

### 01-C3 — Relaunch/resume an interrupted run

## User Goal

Simulate a crash mid-review (context cancellation) and resume, expecting the
run to complete without repeating already-persisted provider calls.

## Category

Interrupted / Relaunch-recovery

## Preconditions

- A run whose context is cancelled after pass-1 review persists but before
  voting completes.

## Inputs

- A context with a very short deadline that expires mid-run.

## Steps

1. Start `Service.Review` with a context that cancels after reviewer calls
   land but before voting finishes.
2. Confirm the run finalizes as `aborted` with partial findings preserved.
3. Call `Service.Resume` with a fresh context.

## Expected Result

The aborted final is non-empty (carries whatever findings/worker output was
already produced); resume either completes the run or reports a clean
preflight state — it must never re-invoke a reviewer whose result is already
durably persisted.

## Observe

- Whether aborted finals include worker findings (regression: they used to
  be dropped — see repair D-067 in the defect ledger).
- Whether resume duplicates provider calls.

---

### 01-C4 — Invalid input: nonexistent document

## User Goal

Attempt to review a file that does not exist and get a clear, correctly
coded failure — not a crash, not a silently empty success.

## Category

Invalid input / Error recovery

## Preconditions

- None.

## Inputs

- `tribunal review /nonexistent/path.md`

## Steps

1. Call `Service.Review` with `opts.Input` pointing at a nonexistent path.

## Expected Result

A non-nil `*app.ExitError` with `ExitPreflight` (5), and the returned
`domain.Final` is the zero value (this is exactly what the JSON-envelope
contract fix (D-032) exists to render correctly at the CLI layer).

## Observe

- Exit code value.
- Error message clarity (does it name the missing path?).

---

### 01-C5 — Settings: weighted panel changes the outcome

## User Goal

Configure a weighted panel (one senior reviewer weighted higher) and confirm
the weight is honored in the vote arithmetic, not just cosmetically stored.

## Category

Settings / Boundary

## Preconditions

- Panel with 3 reviewers, weights 2.0 / 1.0 / 1.0 (senior reviewer counts
  double).

## Inputs

- Vote split: senior reviewer rejects (weight 2.0), two juniors accept
  (weight 1.0 each) → unweighted count says accept 2-1, weighted math says
  reject 2.0 vs accept 2.0 → **exact tie**, must route to arbitration.

## Steps

1. Configure `ConsensusOptions.Weighted = true` with the above weights.
2. Cast the described votes on one finding.
3. Call `domain.ResolveVotes`.

## Expected Result

`Outcome == "arbitration"`, `Reason == "vote_tie"` — the weighted tie
detection (repair D-045, integer-hundredth weights) must fire exactly, not
approximately.

## Observe

- Whether the tie is genuinely exact (not a near-miss that silently resolves
  one way due to float rounding).

---

### 01-C6 — Delete/defer a finding

## User Goal

Explicitly defer a finding with a reason and operator, and confirm it is
excluded from future staleness sweeps and default arbitration recommendation
flows.

## Category

Delete-undo

## Preconditions

- A completed run with at least one open, non-blocker finding.

## Inputs

- `tribunal findings defer <run> <finding-id> --reason "known false
  positive" --operator "qa@example.com"`

## Steps

1. Call `Service.DeferFinding` with reason and operator.
2. Reload the ledger.
3. Run another review over the same workspace scope and confirm the
   deferred record's status is not silently reset to "observed" or "stale".

## Expected Result

Ledger record status becomes `"deferred"`; it stays `"deferred"` — sticky —
across subsequent `UpdateLedger` calls even when absent from a later run's
findings, per repair D-040.

## Observe

- Ledger `Reason`/`ApprovedBy` fields populated.

---

### 01-C7 — Navigation across surfaces (status, transcript, explain)

## User Goal

After a review, navigate the read-only reporting surfaces (`status`,
`transcript`, `explain <finding-id>`) and confirm they agree with each
other and with `final.json`.

## Category

Navigation

## Preconditions

- A completed run with at least one finding and one dispute.

## Inputs

- Run ID from a completed review.

## Steps

1. Call `Service.Status`.
2. Call `Service.Transcript`.
3. Call `Service.Explain` for a known finding ID.

## Expected Result

`Status.Final` matches the persisted `final.json`; `Transcript.Events`
reflects the phase sequence (reviewing → reviewed → verifying → clustered →
voting → consensus → recommended/arbitration_pending); `Explain` returns the
correct `Decision` for the finding.

## Observe

- Any surface disagreeing with another about the same run's outcome.

---

### 01-C8 — Interrupted workflow: quorum loss mid-panel

## User Goal

Simulate 2 of 3 reviewers failing (adapter errors) and confirm the run
degrades gracefully instead of silently proceeding as if nothing happened.

## Category

Interrupted / Error recovery

## Preconditions

- 3-reviewer panel; 2 reviewers configured to return an adapter error.

## Inputs

- Reviewer 1: succeeds with one finding. Reviewers 2 and 3: return errors.

## Steps

1. Run `Service.Review`.

## Expected Result

`valid` panelist count is 1, which fails the `len(valid)*2 <= len(panel)`
quorum check → `finalizeDegraded` → exit code 3 (`ExitDegraded`), status
`"degraded"`, `PanelIncomplete == true`, `DegradedReason` populated.

## Observe

- Whether the sole surviving reviewer's finding is still recorded (it
  should be — degraded doesn't mean silent).
