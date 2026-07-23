# 03 — Domain deliberation scenarios (philosophy, ethics, science, coding)

Standing playtest cards for **how Tribunal deliberates on substantive issue
documents**: what defects panels raise, how independent findings cluster into
agreement, how dissent is recorded, and when disagreement routes to
arbitration. Companion to `02-consensus-scenarios.md` (vote arithmetic) and
`01-review-workflow.md` (CLI lifecycle).

## Scope and safety

- **Harness path (100 scenarios, default):** `go test` driver
  `internal/tribunal/app/domain_deliberation_playtest_test.go` uses
  `domain.ResolveVotes` / `domain.ClusterFindings` and five
  Review→Arbitrate loops via `adapters.FuncAdapter`. No network, no real
  provider CLIs, `t.TempDir()` only.
- **Live path (optional qualitative sample):** `tribunal review` against
  `/tmp/tribunal-playtest/corpus/{philosophy,ethics,science,coding}/*.md`
  with real `claude`/`codex`/`agy` adapters. Requires configured provider
  CLIs, burns tokens/time, and may degrade on schema/timeouts. Live results
  belong in a session log, not this card file.
- Prohibited: writing review results into this library; real credentials in
  documents; network workers beyond doctor-approved adapters.

## How to run

```bash
# Full 100 + clustering + 5 disagree→consensus e2e
go test ./internal/tribunal/app/ -run 'TestDomainDeliberation' -count=1

# Live sample (example)
./bin/tribunal review /tmp/tribunal-playtest/corpus/coding/C01.md \
  --state-root /tmp/tribunal-playtest/live-state --json --max-wall-time 20m
```

## Card inventory (100)

| Domain | IDs | Count | Characteristic planted defects |
|---|---|---:|---|
| Philosophy | P01–P25 | 25 | Overstrong metaphysical conclusions, is-ought leaps, modal bridges |
| Ethics | E01–E25 | 25 | Absolute duties, dismissed counterarguments, missing safeguards |
| Science | S01–S25 | 25 | Stats misreads, causal leaps, underpowered claims, bias |
| Coding | C01–C25 | 25 | Injection, secrets, races, fail-open auth, unsafe crypto |

Each ID is paired with a **vote pattern** (cycled across the 25) so every
domain exercises the same agreement space:

| Pattern | Expected outcome | What it probes |
|---|---|---|
| `unanimous_accept` | accepted / majority_accept | Clear panel agreement |
| `majority_accept` | accepted + dissent | Argue-then-agree with recorded dissent |
| `majority_reject` | rejected | Panel rejects overreach finding |
| `tie` | arbitration / vote_tie | Equal split → human path |
| `unanimous_reject` | rejected | Unanimous disagreement with the finding |
| `strict_split` | arbitration / category_requires_full_panel_unanimity | Security/citation strictness |
| `unevidenced_accept` | unverified-claim | Factual claim without evidence |
| `severity_dissent` | accepted + dissent | Agree on issue, disagree on severity |
| `abstain_heavy` | arbitration / insufficient_non_abstain_votes | Too few non-abstain votes |

## Extra cards (not in the 100 count)

| ID | Name | Goal |
|---|---|---|
| CL-P/E/S/C | Cross-reviewer clustering | Same quote+category clusters despite different issue prose |
| P/E/S/C/X-arb | Disagree→consensus | Full Review→Arbitrate accept on domain-flavored split |

## Observations to capture (live path)

For each live run, record:

1. Did reviewers **review the document** or **answer the question**?
2. Which planted defects were raised independently (agreement by content)?
3. Severity taxonomy consistency across adapters.
4. Schema validity / retry / timeout / degraded panel status.
5. Final decision outcomes and dissent text quality.
6. Domain differences (e.g. coding security consensus vs philosophy value splits).
