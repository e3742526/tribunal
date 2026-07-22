# Defect repair campaign — 2026-07-22

Source: fresh four-lens code evaluation of 2026-07-22 (app core, adapters/config,
storage/domain/documents, CLI/CI/docs), each finding re-verified against source
before inclusion. Baseline: `main` @ `6709276`, `scripts/check.sh` green,
coverage measured (storage 31.0%, cli 32.3%, domain 43.1%).
Branch: `repair/eval-findings-20260722`. Authority: local repair and
verification; no push, tag, release, or history rewrite.

## Gate 1 — Defect inventory

IDs continue the native ledger sequence (next: D-030). Priority: P0 highest.

| ID | P | Cx | Finding | Touch set |
|---|---|---|---|---|
| D-030 | P0 | med | `Resume` mutates run state (publication completion, edit-transaction recovery) without `run.lock` | app/operations.go |
| D-031 | P0 | med | `arbitrate --accept-majority` rejects disputes whose `Default` was overwritten with `previous ruling: accepted` | app/operations.go, review.go, recovery.go |
| D-032 | P0 | low | `--json` failure paths print a zero-value `schema_version:0`, `exit_code:0` object on stdout | cli/review_commands.go, edit_commands.go, state_commands.go, admin_commands.go |
| D-033 | P0 | low | interactive `arbitrate` with no final returns nil → silent exit 0 | cli/review_commands.go |
| D-034 | P1 | low | model-declared `worker-verified` evidence status survives `--no-workers` and degraded finalization | app/review.go (validatePass) |
| D-035 | P1 | low | `Replay` of a `--split` run always fails context preflight (no Split passthrough) | app/operations.go, review.go |
| D-036 | P1 | med | agy prompt (document content) passed as one argv element: E2BIG >~128KiB, visible in `ps` | adapters/subprocess.go |
| D-037 | P1 | low | OpenAI-compatible API key exported into every provider subprocess environment | adapters/subprocess.go, app/review.go, edit.go |
| D-038 | P1 | med | DOCX/PDF source hash and extracted content come from two independent file reads | documents/packet.go |
| D-039 | P1 | med | anchor resolution: first-occurrence quote binding with no uniqueness check; fuzzy-path ambiguity guard provably dead | documents/anchors.go |
| D-040 | P1 | low | `UpdateLedger` stale-record loop is a no-op; unseen findings keep stale status forever | storage/state.go |
| D-041 | P1 | med | rolled-back edit apply leaves `backups/<hash>.original` forever, permanently blocking retry | app/edit.go, edit_transaction.go |
| D-042 | P1 | med | torn trailing JSONL line permanently bricks decisions append / transcript read | storage/store.go, state.go, app/operations.go |
| D-043 | P1 | low | CI never runs `-race`; govulncheck soft-skips everywhere including release gate | .github/workflows/ci.yml, scripts/check.sh |
| D-044 | P2 | low | docs claim weighted panel files, custom rubrics, per-hunk confirmation/mandatory rereview — none implemented | docs/SPEC.md, docs/io-contract.md |
| D-045 | P2 | low | consensus tie check uses exact float equality on weight sums (latent while all weights are 1) | domain/consensus.go |
| D-046 | P2 | low | 32-bit finding fingerprint is ledger/defer/decision-memory identity | domain/consensus.go |
| D-047 | P2 | low | 1-char cluster ID panics `RankArbitration` (`cluster.ID[2:]`); `ValidateCluster` accepts it | domain/validation.go, consensus.go |
| D-048 | P2 | low | sub-second `call_timeout` truncates to 0 → silently becomes 15-minute default | config/config.go |
| D-049 | P2 | low | `recommend --kind` silently dead (`--rubric` default always wins) | cli/review_commands.go |
| D-050 | P2 | low | transcript timestamp layout has literal `Z`; non-UTC event times print falsely as UTC | cli/state_commands.go |
| D-051 | P2 | low | non-ExitError failures default to exit 4 "invalid arguments", mislabeling runtime faults | main.go |
| D-052 | P2 | low | spellcheck worker findings uncapped (panel reviews cap at 25) | adapters/workers.go |
| D-053 | P2 | low | empty `choices` OpenAI response wraps nil error (`%!w(<nil>)`) | adapters/openai.go |
| D-054 | P2 | low | subprocess output file read unbounded; read errors silently fall back to stdout | adapters/subprocess.go |
| D-055 | P2 | med | hand-rolled kill/wait select: pid-reuse race and escaped-grandchild pipe hang | adapters/process_unix.go, subprocess.go |
| D-056 | P2 | low | `detect` runs `--version` with unbounded CombinedOutput, no per-call deadline | adapters/adapter.go |
| D-057 | P2 | low | redirect guard only on fallback HTTP client; injected client follows cross-origin redirects | adapters/openai.go |
| D-058 | P2 | low | `MaxVerification`/`MaxArbitration` unvalidated; negative silently disables evidence verification | config/config.go |
| D-059 | P2 | low | `ResolvePersona` joins caller name into path without slug validation (defense in depth) | config/rubric.go |
| D-060 | P2 | low | `Transition` never validates `next`; one bad caller bricks the run directory | storage/state.go |
| D-061 | P2 | low | `appendJSONLine` Lstat-then-open TOCTOU; `LockStatus` follows symlinks | storage/store.go, lock_unix.go |
| D-062 | P2 | med | state/document disjointness check is byte-exact; case-insensitive APFS defeats it | storage/store.go |
| D-063 | P2 | low | raw document read and pdftotext stderr unbounded; DOCX XML cap hardcoded | documents/packet.go |
| D-064 | P2 | low | duplicate `word/document.xml` zip entries: first wins vs consumers honoring last | documents/packet.go |
| D-065 | P2 | low | `chunk:%04d` IDs sort lexicographically; >9999 chunks scramble delivery order | documents/anchors.go |
| D-066 | P2 | low | `Revert` validates the stale pre-recovery record → misleading "user changes" error after crash rollback | app/edit.go |
| D-067 | P2 | low | review-abort final omits persisted worker findings and drops accumulated reason codes | app/review.go |
| D-068 | P2 | low | `check.sh` mutates the tree via `go mod tidy` and needs git; `gh release create` not re-run-safe; goreleaser changelog block dead | scripts/check.sh, .github/workflows/release.yml, .goreleaser.yaml |
| D-069 | P2 | low | ARCHITECTURE.md module table lists `config.ResolvePanel` and `tui.Run`, neither exists | docs/ARCHITECTURE.md |

Excluded / deferred (not reopened, feature-shaped, or judgment-call design):

- unbounded silent lock waits (design: publish must survive ctx abort; feedback improvement routed to backlog);
- reduced-trust posture for JSON-repaired model output (behavior change needing design);
- `tui` vs `status` duplication (feature decision);
- `PersistTerminal` same-identity replacement journaling (intentional edit/re-review support; needs design);
- sticky `rejected` ledger status (design decision on disposition semantics);
- darwin workspace-ID normalization (identity migration; only the containment guard is repaired here as D-062);
- weighted panel file input (feature work; docs corrected instead, D-044);
- x/ dependency bumps (maintenance, not defect);
- consensus-tail duplication between review.go and recovery.go (routed: simplification pass; D-031 patches both copies consistently).

## Gate 2 — Group plan (ordered)

- Stage 1 `cli-contract`: D-032, D-033, D-049, D-050, D-051. Files: internal/cli/*, main.go. Regression: cli tests + new JSON-failure tests.
- Stage 2 `arbitration-resume-core`: D-030, D-031, D-034, D-035, D-067. Files: app/operations.go, review.go, recovery.go. Regression: app suite + new lock/memory/evidence/replay tests.
- Stage 3 `domain-identity`: D-045, D-046, D-047. Files: domain/consensus.go, validation.go. Note: fingerprint widening changes ledger/decision-memory identity; acceptable pre-release, recorded here.
- Stage 4 `documents-integrity`: D-038, D-039, D-063, D-064, D-065. Files: documents/packet.go, anchors.go.
- Stage 5 `adapters-hardening`: D-036, D-037, D-048, D-052, D-053, D-054, D-055, D-056, D-057, D-058, D-059. Files: adapters/*, config/*.go, Request construction sites in app.
- Stage 6 `storage-durability`: D-040, D-042, D-060, D-061, D-062. Files: storage/state.go, store.go, lock_unix.go.
- Stage 7 `edit-retry`: D-041, D-066. Files: app/edit.go, edit_transaction.go.
- Stage 8 `ci-release-docs`: D-043, D-044, D-068, D-069 + record closure (defect ledger, CHANGELOG, TEST_LEDGER, io-contract error envelope).

Modularization decisions: none — every touched file is under the repo's own
800-line gate, below this campaign's 1000-line threshold.

Cross-stage risks: Stage 1's JSON error envelope changes the documented IO
contract (doc lands in Stage 8); Stage 2 and Stage 5 both touch `Request`
construction lines in app (kept disjoint: Stage 2 edits logic, Stage 5 edits
`EnvSecrets`/timeout fields); Stage 3 fingerprint widening invalidates
pre-existing workspace ledgers (pre-release state break, no migration).

Commit boundary: one commit per stage, `repair:` prefix per repo convention.

## Per-stage results

(appended as stages complete)

### Stage 1 — cli-contract (commit 7ecf4a2)

D-032/033/049/050/051 fixed. New: `ExitInternal=7`, `app.ExitCodeFor`, JSON
error envelope (`renderError`), zero-final guard (`renderFinalOutcome`), RunE
decorator guaranteeing an envelope for every command failure in `--json` mode.
Adversarial review (fresh agent) found two real issues in the first cut, both
fixed before commit: cobra parse errors regressed from exit 4 to 7 (default
restored to invalid-arguments; runtime-fault sites wrapped explicitly) and
early errors bypassed the envelope (central decorator added). Regression tests:
`internal/cli/contract_test.go` (envelope, zero-final impersonation, parse-error
codes, rubric/kind exclusivity, decorator path). `scripts/check.sh` green.
Interactive-only arbitrate nil-final guard is reasoned, not TTY-tested.

### Stage 2 — arbitration-resume-core (commit 0673151)

D-030/031/034/035/067 fixed. Resume now takes `run.lock` (bounded by the run
timeout) + ValidateRunDir before its mutating branches; the inner
resumeCheckpoint lock was removed (sole caller holds it; double flock would
self-deadlock). Decision-memory hints moved to a new `MemoryHint` dispute
field so `--accept-majority` reads the true panel default; legacy finals with
"previous ruling:" in Default are honored explicitly. `validatePass` enforces
the worker-verified downgrade host-side. Replay and edit `--rereview` re-enable
splitting for chunked packets. Abort finals now include worker findings,
canonical IDs, and accumulated reason codes. finalize helpers extracted to
`finalize.go` (review.go had crossed the 800-line gate). Adversarial review
found four residuals (unbounded lock wait, edit-rereview split gap, hint
missing from report.md, legacy-final inversion) — all fixed pre-commit.
Tests: `arbitration_resume_test.go` (5 tests incl. real flock contention and
split replay). check.sh + `go test -race ./internal/tribunal/app` green.

### Stage 3 — domain-identity

D-045/046/047 fixed. Vote weights accumulate as integer hundredths (NaN-proof
clamp), making vote_tie exact; FindingFingerprint widened to 16 hex; cluster
IDs shape-validated (`C-` + 16 lowercase hex) and the `[2:]` slice replaced
with TrimPrefix. Adversarial review flagged that the workspace ledger and
decision memory would accept legacy 8-hex fingerprints and silently duplicate
records — both stores now fail closed with an explicit incompatibility error
(deliberate pre-release state break, no migration). Residuals routed to later
stages/backlog: quantization wording lands with the D-044 doc pass;
defaultRecommendation/gap-sort use unweighted counts while ties are weighted
(pre-existing; backlog). Tests: identity_test.go (exact weighted tie,
16-hex length, malformed cluster IDs). Full suite + check.sh green.
