# Test ledger

| Area | Evidence | What it proves | Gap |
|---|---|---|---|
| Domain | `go test ./internal/tribunal/domain` | panel grammar, quorum/tie/strict category, clustering, exact decision-memory match | larger property corpus remains useful |
| Documents | `go test ./internal/tribunal/documents` | lexical packet order, redaction, anchors, UTF-8 chunks, DOCX extraction | PDF malformed corpus is environment-dependent |
| Adapters/workers | `go test ./internal/tribunal/adapters` | read-only argv, Claude envelope, schema recovery, output caps, domain allowlist, provenance, spelling/reference checks | authenticated vendor calls are not in unit CI |
| Storage | `go test ./internal/tribunal/storage` | external root, ULID/private dirs, journal ordering, cancellation, real subprocess lock contention | unsupported Windows lock stub is compile-only |
| Application | `go test ./internal/tribunal/app` | synthetic three-reviewer run, pass-1 durability barrier, complete artifact set, edit/apply/revert and user-change refusal | exhaustive syscall fault matrix is not complete |
| CLI | `go test ./internal/cli` | clean command surface, local HTTP panel, stable JSON, Git absent, external-only review state | interactive TTY arbitration is manually exercised |
| Full gate | `scripts/check.sh` | format, 800 lines, tests, vet, build, module integrity/tidy, vulnerability scan when installed | scanner absence is reported, not treated as verified |
| Race | `go test -race ./...` | concurrent review/vote, locks, HTTP tests, durable state under race detector | real provider rate limits are outside local control |

Skipped real vendor checks must be recorded in `docs/build/evidence/`; their
absence is never reported as verified behavior.
