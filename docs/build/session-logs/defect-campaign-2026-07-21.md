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

To be appended as each group passes Gates 3–8.

## Record closure

To be completed only after verification. The generated audit report/findings and
`docs/build/defect-ledger.md` remain open until their corresponding stage passes.
