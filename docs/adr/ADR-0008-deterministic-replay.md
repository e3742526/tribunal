# ADR-0008: Deterministic replay and uncertain external effects

Status: accepted

## Decision

At the `PACKET_BUILT` transition, Tribunal writes `execution-snapshot.json`.
The versioned snapshot freezes the packet/workflow digest, normalized panel,
review options, limits, and provider configuration references. Existing
snapshots are immutable. A changed snapshot is divergence and requires a new
run; running code does not silently adopt changed arguments or configuration.
Credentials are excluded. Each provider request instead includes a one-way
digest of every selected secret value in its canonical semantic envelope.

Every provider invocation crosses Tribunal's nondeterministic external-effect
boundary. Before invoking it, the host allocates the next run-scoped sequence
and durably writes a `prepared` record to `operations.json`, then durably marks
it `in_flight`. The envelope covers the operation type, adapter destination,
role, complete panelist/model policy, prompts, response schema, timeout, output
limits, and secret digests. JSON canonicalization is versioned and hashes map
keys deterministically. Results are committed with their byte hash and bounded
raw bytes. `(run_id, sequence)` is unique because mutation occurs under the run
lock and the in-process journal mutex.

`tribunal replay` consumes the source stream from sequence one. It reuses a
committed result only when sequence, operation type, request hash, workflow
revision, and result hash all match. Missing, reordered, inserted, corrupt, or
incompatible records fail closed as typed divergence; replay never searches
forward or falls back to a live provider call.

An invocation error after `in_flight`, or failure to durably record its result,
is not proof that the provider did nothing. Tribunal records the operation as
`in_doubt` with `duplicate_effect_risk`, and replay blocks for operator/provider
reconciliation. Provider adapters currently expose no native reconciliation
port, so this boundary remains intentionally unverified rather than inventing
a committed result.

> Tribunal deterministically replays its durable control history. External
> operations without a durably recorded commit may execute again. Resume is
> therefore at-least-once at uncertain external-effect boundaries, not
> magically exactly-once.

## Migration and rollback

There is no fabricated migration for old active runs. Runs without an execution
snapshot are reported as `legacy_non_replayable`; they may finish through the
existing checkpoint recovery path but cannot be the source of deterministic
replay. Operators must start a new run to opt in. Rollback requires stopping
new work first; old binaries do not understand the new authority artifacts and
must not resume a deterministic-v1 run.

## Parallel panels and budgets

Tribunal's existing usage ledger keeps cumulative token `reserved_tokens` and
`used_tokens` separate. A completion moves a reservation into monotonic used
work; releasing provider serialization or a completed goroutine does not refund
it. Failed or crash-stranded reservations are conservatively consumed. The
current CLI has no tenant scheduler or configurable live-slot pool: the run
owns its complete frozen panel and starts the panel as a unit only after its
run lock, snapshot, operation journal, and usage ledger are durable. Therefore
cross-run provider serialization is a provider lock, not cumulative budget.
Adding a shared scheduler requires transactional whole-panel admission before
any member launch; partial or post-start degraded substitution is forbidden.

## Traceability

| Invariant | Implementation | Executable evidence |
|---|---|---|
| Immutable workflow/arguments | `app/replay.go`, `app/review.go`, `app/recovery.go` | `TestImmutableExecutionSnapshotRejectsMutation` |
| Canonical complete provider envelope | `app/replay.go`, `app/budget.go` | `TestCanonicalRequestGoldenVectors` |
| Sequence/type/hash exact replay | `app/replay.go`, `app/operations.go` | `TestExactReplayReturnsCommittedResultWithoutLiveCall`, `TestReplayDivergenceAndUncertainEffectsFailClosed` |
| At-least-once uncertain effects | `app/replay.go`, status/TUI projection | `TestReplayDivergenceAndUncertainEffectsFailClosed/in-doubt` |
| Separate cumulative work | `app/budget.go` | `TestUsageBudgetConcurrentReservations`, `TestUsageBudgetChargesFailuresAndRecoversReservations` |
| Legacy safety | `storage.BuildSnapshot`, replay source loading | missing operation history fails closed; status is `legacy_non_replayable` |

