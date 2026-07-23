# Tribunal Playtest Scenario Library — Consensus and Voting

Standing, user-facing playtest cards exercising Tribunal's core value
proposition: an independent panel reviews a document, votes blind on
clustered findings, and reaches a recorded consensus (accepted / rejected /
arbitration / degraded / unverified-claim). Each pass runs these cards
against a trusted sandbox and records results in a separate playtest report;
do not write pass results into this library.

## Scope and safety

- Test environment: local checkout, `go test` driver using the in-process
  `adapters.FuncAdapter` fake in place of real provider CLIs, plus
  `t.TempDir()` for all state roots and document workspaces. No network
  calls, no real provider credentials, no writes outside the Go test
  sandbox.
- Disposable data/home requirements: each scenario gets its own
  `t.TempDir()` document directory and state root; nothing persists between
  scenarios.
- Test credentials or external sandbox requirements: none — `FuncAdapter`
  supplies scripted reviewer/voter responses deterministically.
- Prohibited or out-of-scope actions: no shelling out to real `codex`/
  `claude`/`agy` binaries, no writing into the reviewed document workspace,
  no network egress.
- Cleanup expectations: `t.TempDir()` cleans up automatically; no manual
  cleanup required.

## How to run a pass

1. Build the harness: `internal/tribunal/app/consensus_playtest_test.go`
   (driver) reads cards in `docs/test_scenarios/02-consensus-scenarios.md`
   at a conceptual level — the actual runnable cards are Go-table-driven
   scenario structs in the driver file, one per card ID below, so behavior
   changes in the real domain/app code are exercised, not re-implemented.
2. Run with `go test ./internal/tribunal/app/ -run TestConsensusPlaytest -v`.
3. Record Pass / Fail / Partial / Blocked / Not applicable / Not executed,
   actual results, evidence, and findings in the playtest report.
4. Preserve these cards unchanged so later passes remain comparable.

## Card format

Each card has a stable ID, name, goal, category, preconditions, exact panel
composition and scripted per-reviewer/per-voter responses, steps, expected
consensus outcome, and observations to capture.

## Files and order

| Order | File | Surface or workflow | Dependencies |
|---|---|---|---|
| 1 | `01-review-workflow.md` | CLI-level review lifecycle (launch, save/persist, relaunch/resume, settings, navigation, errors) | none |
| 2 | `02-consensus-scenarios.md` | Panel voting and consensus resolution (happy path, ties, splits, strict-category unanimity, quorum, weighted votes, arbitration, decision memory) | none |
| 3 | `03-domain-deliberation.md` | Domain-themed deliberation (philosophy, ethics, science, coding): agreement, dissent, clustering, disagree→consensus | none |

## Pass shapes

| Pass | Files/cards | Intent |
|---|---|---|
| Smoke | 01-C1, 02-S01..S05 | Confirm review lifecycle and core consensus math work |
| Full | all files, all cards (50+ consensus scenarios) | Complete exploratory pass across the consensus decision space, including 5+ disagree-then-consensus (arbitration) cases |
| Domain | 03 (100 domain cards + clustering + 5 arb) | Substantive issue documents across philosophy/ethics/science/coding; how panels agree, dissent, and arbitrate |

## Required coverage

- [x] First launch or initial empty state — 01-C1
- [x] Primary happy-path workflow — 01-C2, 02-S01
- [x] Primary workflow with invalid input — 01-C4
- [x] Save or persistence behavior — 01-C3
- [x] Delete, remove, cancel, or undo behavior — 01-C6 (findings defer)
- [x] Settings or preferences — 01-C5 (panel/weight config)
- [x] Navigation across major screens — 01-C7 (status/transcript/explain)
- [x] Close and relaunch behavior — 01-C3 (resume)
- [x] Interrupted or stopped workflow — 01-C8
- [ ] File import/export — not applicable; Tribunal reviews documents in
      place and never imports/exports a project file format beyond the
      review's own JSON artifacts, which 01-C3/01-C7 already cover.
- [x] Error recovery — 01-C4, 01-C8
- [x] Edge or boundary input — 02 quorum/tie/unanimity cards

## Corpus construction caveats

When building live document corpora (as in the 03 domain pass), vary the
structural boilerplate between documents. Shared skeletons — an identical
"minority views" paragraph, an identically empty risk register — draw
near-identical secondary findings across documents (playtest finding L-05,
2026-07-22 domain pass) that can overshadow the per-document planted defect
you are trying to measure. Template-seam findings are legitimate
integrity/structure signal, but for claim-level discrimination measurements,
give each document distinct scaffolding so agreement statistics reflect the
unique claim, not the copy-pasted frame.
