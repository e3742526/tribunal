# Tribunal v0.1.0 final QA

## Result

Tribunal v0.1.0 is implemented and locally accepted for macOS/Linux source and
release targets. The primary workflow is Git-independent, review-only, uses
external durable state, enforces a pass-one persistence barrier, isolates
invalid reviewers, completes blind voting/consensus, and continues through
arbitration and separately authorized edit/revert operations.

## Acceptance evidence

See [Gate 8 evidence](evidence/gate-8-evidence.md) for exact commands and gaps.
The strongest end-to-end evidence is the Git-absent local HTTP CLI test and the
bounded authenticated panel run that reached real Codex/Agy blind voting.

## Residual scope

- Install and run optional vulnerability/workflow/release linters before a
  public release.
- Execute the archive smoke on Linux in release CI.
- Do not release Windows until multiprocess lock coverage replaces its stub.
- Treat model contract failures as external degraded inputs; never infer or
  fabricate a missing review.

No known open defect blocks the accepted local contract.
