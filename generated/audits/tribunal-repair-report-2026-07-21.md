# Tribunal defect repair campaign report

Date: 2026-07-21  
Source audit revision: `4a8003c790857fbc3d7c5d7adacebda6fe030f32`  
Repair branch: `repair/audit-findings-20260721`  
Authority: local repair and verification; no push, tag, release, or history rewrite

## Outcome

All 11 findings from `tribunal-audit-2026-07-21.md` have implemented repairs,
regression evidence, native defect-ledger entries, and local checkpoint commits.
The five High and six Medium audit rows are marked resolved. No finding was
waived, deferred, or silently rescoped.

| Finding | Severity | Repair | Evidence | Checkpoint |
|---|---|---|---|---|
| ARC-TRIBUNAL-002 | High | complete frozen blind vote packet and delivery attestation | synthetic identical-ballot/content test | `4a8b63b` |
| LLM-TRIBUNAL-001 | High | retrieval remains semantically unverified | unrelated successful retrieval test | `4a8b63b` |
| LLM-TRIBUNAL-002 | Medium | durable provider reservation/reconciliation ledger | near-limit call-stop and OpenAI cap tests | `4a8b63b` |
| ARC-TRIBUNAL-003 | Medium | global unevidenced severity normalization | every-category table test | `4a8b63b` |
| FSR-TRIBUNAL-001 | High | strict checkpoint reducer with immutable completed calls | terminal/candidate deletion resume with zero new calls | `2c6f654` |
| SEC-TRIBUNAL-001 | High | canonical persisted-packet hash/path/version revalidation | field-tampering matrix and corrupt-resume refusal | `2c6f654` |
| ARC-TRIBUNAL-004 | Medium | closed JSON decoding and recursive semantic validation | nested version corruption refusal before provider call | `2c6f654` |
| FSR-TRIBUNAL-003 | Medium | terminal artifacts precede idempotent workspace projections | publication marker/candidate recovery path | `2c6f654` |
| FSR-TRIBUNAL-002 | High | durable apply/revert transaction and recovery protocol | post-source interruption, rollback, terminal-match tests | `c9a7d7d` |
| SEC-TRIBUNAL-002 | Medium | exact archives smoke-tested before external publication | workflow ordering test and YAML parse | `ca15fec` |
| ARC-TRIBUNAL-001 | Medium | versioned intent/invariant/exception/baseline registry | conformance, forbidden-edge, expired-exception tests | `cb99216` |

## Verification

- `scripts/check.sh`: passed after every repair group and at closeout. It covers
  formatting, 800-line limits, architecture conformance, all tests, vet, build,
  module verification, and tidy-diff enforcement.
- `go test -race ./...`: passed at closeout.
- Focused application, domain, document, adapter, storage, workflow, edit fault,
  packet corruption, and architecture tests passed.
- Ruby parsed `.github/workflows/release.yml` successfully.
- Repository scans found no open audit table rows, runtime Git subprocesses,
  Tagteam identifiers, or undocumented TODO/FIXME placeholders in runtime scope.
- Clean-checkout build evidence is recorded in the campaign session log.

## Evidence gaps and residual risk

- `govulncheck` was not installed; `scripts/check.sh` reported and skipped it.
- `actionlint` was not installed. YAML parsing and the structural workflow test
  passed, but the tag-triggered release workflow was not executed because release
  publication was outside campaign authority.
- Real paid-provider calls, OS process-kill matrices at every syscall boundary, and
  external rate-limit behavior were not rerun. Deterministic adapters, durable
  checkpoint tests, fault seams, and the full race suite cover the local contracts.

## Local commit sequence

1. `6d62a27` — audit and campaign records
2. `4a8b63b` — deliberation integrity and usage limits
3. `2c6f654` — fail-closed recovery and publication
4. `c9a7d7d` — crash-recoverable edit/revert
5. `ca15fec` — pre-publication archive smoke
6. `cb99216` — architecture invariant gate

The branch remains local and unpushed.
