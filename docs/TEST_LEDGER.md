# Test ledger

| Area | Evidence | What it proves | Gap |
|---|---|---|---|
| Domain | `go test ./internal/tribunal/domain` | panel grammar, quorum/tie/strict category, all-category evidence cap, clustering, exact decision-memory match, persisted-object validation | larger property corpus remains useful |
| Documents | `go test ./internal/tribunal/documents` | lexical packet order, redaction, anchors, UTF-8 chunks, DOCX extraction, persisted packet hash/path/version tampering refusal | PDF malformed corpus is environment-dependent |
| Adapters/workers | `go test ./internal/tribunal/adapters` plus Gate 7 smoke | read-only argv, non-Git Codex calls, Agy durations, strict provider schemas, Claude envelope, recovery, output caps, allowlists, provenance, spelling/reference checks | vendor response quality and rate limits remain external |
| Storage | `go test ./internal/tribunal/storage` | external root, ULID/private dirs, journal ordering, cancellation, real subprocess lock contention | unsupported Windows lock stub is compile-only |
| Application | `go test ./internal/tribunal/app` | synthetic three-reviewer run, complete blind vote delivery, durable usage limit, no-duplicate checkpoint resume, strict nested reads, terminal-first publication, edit/revert transaction recovery and user-change refusal | exhaustive OS-kill matrix remains larger than deterministic fault seams |
| CLI | `go test ./internal/cli` | clean command surface, local HTTP panel, stable JSON, Git absent, external-only review state | interactive TTY arbitration is manually exercised |
| Full gate | `scripts/check.sh` | format, 800 lines, tests, vet, build, module integrity/tidy, vulnerability scan when installed | scanner absence is reported, not treated as verified |
| Race | `go test -race ./...` | concurrent review/vote, locks, HTTP tests, durable state under race detector | real provider rate limits are outside local control |
| Architecture | `scripts/check-architecture.sh` | registry completeness, allowed production Go dependencies, exception expiry, baseline accounting and health score | first baseline has no historical trend |
| Release | `go test . -run TestReleaseSmokePrecedesPublication` plus YAML parse | exact candidate handoff is ordered build → platform smoke → attest/publish | tag-triggered workflow was not executed in this campaign |

The authenticated three-family check is recorded in Gate 7 evidence. Claude
returned invalid contract output and was correctly isolated; this is evidence
of bounded degradation, not a claim of valid Claude review content.
