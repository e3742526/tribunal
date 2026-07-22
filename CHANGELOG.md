# Changelog

## Unreleased — 2026-07-22 defect campaign

Forty defects (D-030–D-069) repaired across eight staged commits; see
`docs/build/session-logs/defect-campaign-2026-07-22.md`. Behavior changes:

- Failed `--json` commands emit a schema-versioned error envelope instead of a
  zero-value result; unclassified runtime faults exit with new code 7.
- `recommend --rubric` no longer silently overrides `--kind`; the two are
  mutually exclusive and the rubric defaults to the configured kind.
- Finding fingerprints widened to 16 hex chars; pre-existing workspace
  ledgers, decision memory, and cluster artifacts are rejected with an
  explicit incompatibility error (pre-release state break, no migration).
- Anchors bind only provably unique spans; repeated quotes without isolating
  context are quarantined. In-flight pre-upgrade runs may quarantine such
  findings on resume.
- Markdown/text documents above the 16 MB extraction cap are now rejected
  (previously unbounded); raw document reads cap at 128 MB.
- Split packets use eight-digit chunk IDs, changing split-packet hashes.
- agy prompts above the platform argv cap fail closed pre-exec with guidance.
- Sub-second `limits.call_timeout`/`run_timeout` and non-positive
  `max_verification`/`max_arbitration` are configuration errors.
- Provider subprocess environments no longer receive the OpenAI-compatible
  API key; it is consumed in-process only.
- Torn journal tails are quarantined to `.corrupt` sidecars and repaired;
  ledger records unseen by a run that re-examined their packet item go stale.
- CI runs the race detector and requires govulncheck; `check.sh` uses
  `go mod tidy -diff` and no longer needs git; release publication re-runs
  converge instead of failing on an existing release.

## v0.1.0 — 2026-07-21

- Initial Tribunal release.
- Git-independent document packets for Markdown, text, DOCX, PDF, and folders.
- Independent pass-one reviews, blind voting, deterministic consensus, dissent,
  arbitration, ledgers, resume, and replay.
- Codex, Claude, Agy, and OpenAI-compatible adapters with bounded contracts.
- External durable state, locks, redaction, typed edit proposals, atomic apply,
  and hash-protected revert.
- Generic, manuscript, strategy, and governance rubrics; personas, workers,
  reports, status snapshots, bench, doctor, and release provenance checks.
