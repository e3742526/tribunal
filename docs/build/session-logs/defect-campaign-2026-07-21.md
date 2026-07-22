# Defect repair campaign — 2026-07-21

## Gate 0 — orientation and baseline

- Branch: `repair/audit-findings-20260721`, created from `main` at `4a8003c`.
- Remote: `origin`; no fetch, push, tag, or release is authorized.
- Initial worktree: only `generated/audits/` from the immediately preceding audit.
  These files are the campaign source records, not unrelated user work.
- Baseline: `scripts/check.sh` passed. `govulncheck` was unavailable and reported as
  an evidence gap. The test results were cached.
- Validation contract: focused package tests per stage, then `scripts/check.sh` and
  `go test -race ./...` at closeout.
- Repository limits: Go source files must remain at or below 800 lines. The largest
  touched file is `internal/tribunal/app/review.go` at 778 lines, below the campaign's
  1001-line modularization threshold; no in-band modularization is required.
- Session/ledger conventions: this log plus `docs/build/defect-ledger.md`,
  `docs/TEST_LEDGER.md`, and the generated audit source records.

## Gates 1–2 — inventory and locality grouping

All 11 supplied findings were rechecked against revision `4a8003c` and are actionable.
No item is an intentionally deferred feature. ARC-TRIBUNAL-001 is a governance defect;
its accepted SPEC/ADR declarations constrain the repair, so the campaign will encode
those declarations without redesigning them.

| Group | Findings | Priority / complexity | Touch set and data path | Regression surface | Modularization |
|---|---|---|---|---|---|
| 1. Decision integrity | ARC-TRIBUNAL-002, LLM-TRIBUNAL-001, LLM-TRIBUNAL-002, ARC-TRIBUNAL-003 | P0/P1, high | `app/review.go`, `app/prompts.go`, `app/verification.go`, `domain/consensus.go`; review→verification→blind vote→consensus and provider budget | app synthetic run, prompt/delivery goldens, evidence trust tests, domain tables | none; all files <=1000 lines |
| 2. Durable recovery and schema integrity | FSR-TRIBUNAL-001, SEC-TRIBUNAL-001, ARC-TRIBUNAL-004, FSR-TRIBUNAL-003 | P0/P1, high | `documents/packet.go`, `app/review.go`, `app/operations.go`, storage readers/publication; packet→manifest→journal/state→final→ledger | corruption matrix, resume call counts, publication fault tests | none |
| 3. Edit transaction recovery | FSR-TRIBUNAL-002 | P0, high | `app/edit.go` and storage durable writes; proposal→backup→source→record→lifecycle/final and reverse | per-boundary fault/recovery tests | none |
| 4. Release acceptance ordering | SEC-TRIBUNAL-002 | P1, medium | `.github/workflows/release.yml`; build→archive→smoke→publish/attest | workflow structure/static assertions | none |
| 5. Architecture governance | ARC-TRIBUNAL-001 | P2, medium | `.architecture/`, declared SPEC/ADR invariants, CI/check integration | registry validation and deliberate violation fixture | none |

Ordering constraints:

1. Decision artifact contracts land before resume/schema readers are tightened.
2. Strict packet validation and checkpoint recovery land before edit recovery reuses them.
3. Release and governance stages are independent and follow runtime repairs.

Commit policy: one local commit per completed group after focused regression,
adversarial review, and full stage-diff review. No remote synchronization or push.

## Stage results

### Group 1 — decision integrity

- Fixed ARC-TRIBUNAL-002 with a versioned blind vote packet containing the frozen
  rubric, item/chunk content, pre-review evidence, post-review evidence/hash, shuffle
  seed, and blinded findings. Delivery records attest every item, chunk, evidence ID,
  finding ID, and hash. Synthetic voters received byte-identical ballot artifacts.
- Fixed LLM-TRIBUNAL-001 fail-closed: successful retrieval is recorded as
  `retrieved-unverified`; provider output cannot self-declare `worker-verified`.
  A deterministic fetch seam proves an unrelated successful source is not promoted.
- Fixed LLM-TRIBUNAL-002 with a durable `usage.json` reservation/reconciliation ledger
  at the central provider boundary. Calls stop before exceeding the run budget; actual
  OpenAI usage is accounted, unknown failed-call use is charged conservatively, and
  OpenAI-compatible requests receive `max_tokens`.
- Fixed ARC-TRIBUNAL-003 by applying the minor cap to every unevidenced category before
  category-specific consensus outcomes.
- Focused verification: `go test ./internal/tribunal/domain
  ./internal/tribunal/adapters ./internal/tribunal/app` and focused `go vet` passed.
- Concurrency verification: `go test -race ./internal/tribunal/app
  ./internal/tribunal/adapters ./internal/tribunal/domain` passed.
- Repository gate: `scripts/check.sh` passed; `govulncheck` unavailable and explicitly
  skipped.
- Adversarial review: checked identity blindness, fetched-content trust labels,
  concurrent budget reservations, failed-call charging, provider output truncation,
  retry paths, and all-category consensus behavior. No residual in-scope defect found.
- Change review: diff is limited to the four findings, their regression tests, CLI help,
  and closure records. No deferred behavior or unrelated formatting was touched.
- Record closure: audit overview rows ARC-TRIBUNAL-002, LLM-TRIBUNAL-001,
  LLM-TRIBUNAL-002, and ARC-TRIBUNAL-003 changed open → resolved; native ledger rows
  D-019 through D-022 added with passing evidence.
- Commit: `4a8b63b` (`repair: restore deliberation integrity and usage limits`).

### Group 2 — durable recovery and schema integrity

- Fixed FSR-TRIBUNAL-001 with a checkpoint reducer that strictly loads completed
  review and vote attempts, preserves classified failures, reconstructs downstream
  phases, and invokes providers only for absent work. A terminal-artifact deletion
  regression proves recovery makes zero duplicate provider calls.
- Fixed SEC-TRIBUNAL-001 with centralized persisted-packet validation: canonical
  paths and workspace identity are re-established, item/rubric hashes and nested
  versions are checked, and the canonical packet hash is recomputed before resume,
  replay, or edit can consume the packet.
- Fixed ARC-TRIBUNAL-004 with closed JSON decoding and recursive validation for
  packet, meta, state, final, ledger, decision, transcript, delivery, review, vote,
  cluster, evidence, and recovery artifacts. Corrupt known and unknown nested
  versions fail before any provider call.
- Fixed FSR-TRIBUNAL-003 by recording publication intent and the final candidate,
  then committing reports and terminal state/final before updating idempotent
  workspace projections. Pending publication is repairable by `resume`.
- Crash-stranded token reservations are conservatively charged before resumed work.
- Focused verification and `scripts/check.sh` passed. `go test -race` passed for
  app, storage, documents, and domain. `govulncheck` remained unavailable and was
  explicitly skipped.
- Adversarial review covered stored-hash substitution, nested-version corruption,
  missing final/candidate recovery, classified call failures, crash reservations,
  terminal idempotency, and projection ordering. No residual in-scope defect found.
- Record closure: audit overview rows FSR-TRIBUNAL-001, SEC-TRIBUNAL-001,
  ARC-TRIBUNAL-004, and FSR-TRIBUNAL-003 changed open → resolved; ledger rows
  D-023 through D-026 record passing evidence.
- Commit: `2c6f654` (`repair: make recovery and publication fail closed`).

### Group 3 — edit transaction recovery

- Fixed FSR-TRIBUNAL-002 with a durable edit transaction written and fsynced before
  document mutation. It records operation, phase, per-file before/after hashes,
  recovery paths, modes, and applied progress.
- Apply and revert now prepare recovery copies before the first source replacement,
  reconcile mixed crash states from live hashes, roll back pre-terminal mutations,
  leave post-terminal projection failures resumable, and enter a visible manual hold
  when user changes or missing recovery material make automatic action ambiguous.
- `resume`, `edit`, and `revert` reconcile incomplete transactions against the
  terminal `EditsApplied` state. Rolled-back edit records are explicit and cannot be
  mistaken for revertable committed edits.
- Fault tests interrupt immediately after a source replacement and recover from a
  fresh service instance; terminal-match tests prove an applied transaction commits
  only when the durable final agrees. Existing stale hash, scope, user-change, and
  full revert tests remain green.
- `scripts/check.sh` and focused `go test -race` passed. `govulncheck` remained
  unavailable and was explicitly skipped.
- Adversarial review covered crash-before-backup, write-before-progress-marker,
  record-before-final, final-before-projection, revert symmetry, user changes,
  corrupt backups, and state rollback. No residual in-scope defect found.
- Record closure: FSR-TRIBUNAL-002 changed open → resolved and ledger D-027 records
  the regression evidence.
- Commit: pending local group checkpoint.

## Record closure

To be completed only after verification. The generated audit report/findings and
`docs/build/defect-ledger.md` remain open until their corresponding stage passes.
