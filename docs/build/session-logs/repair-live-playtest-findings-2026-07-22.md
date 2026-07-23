# Repair: live domain-playtest findings L-01..L-05 — 2026-07-22

Skill: `repair-defect-patchset` (supplied, enumerated finding set).
Source: `playtest-domain-deliberation-2026-07-22.md` (Grok's live
multi-provider pass; artifacts committed at 62bf310) plus the uncommitted
run logs under `/tmp/tribunal-playtest/` used as ground-truth evidence.
Baseline: `main` @ 62bf310, `scripts/check.sh` green, full suite green.

## Gate 1 — Validation and classification

| ID | Ledger | P | Cx | Domain | Verified how |
|---|---|---|---|---|---|
| L-01 | D-074 | P0 | med | reliability | Real `raw.json`/`retry-raw.json`/`status.json` from run 01KY62WK…: semantically strong Claude output in a wholly foreign shape; `retry-raw.json` fenced with `schema_version: "1.0"`. Root cause: neither prompt nor retry ever contains the contract the system prompt references; only adapter-native channels carry it (claude's `--json-schema` evidently not honored; agy receives no schema channel at all) |
| L-02 | D-075/D-076 | P0 | med | data integrity | All 32 quarantined findings across 8 live runs used the exact bare-filename alias (`X.md` for `artifact:X.md`); S01 lost 6/6 findings from both surviving reviewers and summarized clean |
| L-03 | D-077 | P1 | low | UX/reporting | Live C01/C08 finals: strict security disputes at A2/R0 with 2 of 3 valid |
| L-04 | D-078 | P1 | low | UX/prompt | Smoke trolley run: role confusion confirmed in session log |
| L-05 | — | P3 | low | docs (methodology) | E04/E20 near-identical secondary findings from shared boilerplate |

All five accepted as real and in scope; none already fixed, none feature
work.

## Gate 2 — Grouping and interaction analysis

- Stage A (L-01 + L-04): `app/prompts.go`, `app/review.go` (both retry
  sites), `adapters/contracts.go`. Shared surface: prompt text.
- Stage B (L-02): `documents/anchors.go`, `app/review.go` (validatePass),
  `app/finalize.go`/`summaryFor`. Disjoint functions from Stage A's
  review.go edits.
- Stage C (L-03): `domain/types.go` + `consensus.go`, `app/report.go`,
  `cli/review_commands.go`. Lands after B (report.go touched by both).
- Stage D (L-05): scenario-library README note + record closure.

Interactions: prompt growth (~2.5 KB) stays far under the agy argv caps;
the new dispute `Context` field follows the `MemoryHint` omitempty
precedent (old finals readable; new finals unreadable by old binaries —
accepted pre-release posture); alias resolution deliberately leaves the
`item_sha256` binding untouched so aliasing cannot weaken packet
integrity.

## Per-stage results

### Stage A — 6f14a14 (L-01, L-04 / D-074, D-078)

Review and vote prompts embed `ProviderReviewSchema`/`ProviderVoteSchema`
plus structure skeletons; `contractRetryNotice` names the exact validation
error and points back at the embedded contract; `coerceSchemaVersionStrings`
adds a fail-soft extra candidate (numeric-string `schema_version` → integer,
whole-string `strconv.ParseFloat`, integral values only, marked repaired);
reviewer system prompt gains the review-not-perform guard. Tests:
`contract_coercion_test.go`, `prompt_contract_test.go`.

### Stage B — b7dfb0d (L-02 / D-075, D-076)

`findItem` resolves exact ID, then `artifact:`+name, then logical path;
`ResolveAnchor` canonicalizes the anchor to the resolved item ID before the
unchanged hash check. `validatePass` records `findings_quarantined`;
`summaryFor` names the quarantine count. Tests: `anchor_alias_test.go`
(incl. hash-binding-preserved and unknown-item-still-fails),
`quarantine_visibility_test.go`.

### Stage C — ebdc827 (L-03 / D-077)

`ArbitrationDispute.Context` (advisory, never parsed) populated by
`disputeContext` for `category_requires_full_panel_unanimity`,
`insufficient_non_abstain_votes`, and `unanimity_not_reached`; rendered in
report.md and the interactive prompt; plain vote ties gain no gloss.
Tests: `dispute_context_test.go` over the live C01/C08 geometry.

### Stage D — this commit (L-05)

Corpus-construction caveat added to `docs/test_scenarios/README.md`;
ledger rows D-074–D-078; CHANGELOG section; this session log.

## Behavioral equivalence

Baseline = the full pre-existing suite (green at 62bf310), including the
byte-equality blind-ballot assertions, the 51-card consensus playtest, and
Grok's 109 domain-deliberation tests — all green unmodified after every
stage. The only intentional behavior deltas are the defect paths: prompt
text content, alias acceptance (previously an error path), quarantine
visibility strings/reason codes, and the additive dispute field.
`summaryFor` gained a parameter; its clean-path output is asserted
unchanged.

## Completeness verification against the live artifacts

- L-02: replayed every quarantined finding from the real C08 and S01 runs
  (their actual `packet.json` + `final.json`) through the fixed resolver:
  **10/10 recovered, 0 still quarantined** — including the entire lost S01
  science consensus. Hash bindings all passed, confirming the diagnosis
  (right hash, wrong ID spelling).
- L-01: the coercion accepts the observed `"1.0"` deviation in isolation;
  the primary repair (contract in prompt, error-naming retry) is
  live-provider-dependent and cannot be proven from recorded logs — the
  recorded outputs would rightly still fail (their shape is wrong beyond
  coercion). Live re-run of the 8-doc stratified sample is the follow-up
  acceptance test.
- L-03: context text verified against the exact live A2/R0 geometry.
- L-04: guard asserted in the prompt; live behavior provider-dependent.

## Verification

`scripts/check.sh` green after every stage; `go test -race -count=1 ./...`
green at closeout; adversarial review of the combined diff (Gate 8) —
see closeout note appended below.

## Residual risks and follow-ups

- Live re-run of the stratified 8-document sample to confirm 3-of-3 panels
  and reduced false arbitration (provider access required).
- The editor-role prompt does not embed the edit contract (codex-only path
  today, `--output-schema` honored; same-class hardening available if an
  editor family ever ignores its schema channel).
- agy still has no adapter-native schema channel; it now receives the
  contract via prompt text like every other family.
- Deeper Claude-shape normalization (flat finding objects → contract
  shape) was deliberately NOT implemented: field-name remapping is
  semantic guesswork, and the prompt-side fix addresses the cause.
