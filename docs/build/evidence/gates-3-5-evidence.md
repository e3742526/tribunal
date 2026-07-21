# Gates 3–5 evidence — 2026-07-21

Implemented the Tribunal module, package boundaries, complete public command
surface, deterministic packet/review/vote pipeline, external durability,
adapters/workers, edit/revert, reports, rubrics, personas, DOCX/PDF extraction,
status snapshot, CI/release identity, and removal of the isolated legacy code.

Validation:

- `go test ./...` — pass.
- `go vet ./...` — pass.
- `go build ./...` — pass.
- `go mod verify` — all modules verified.
- `scripts/check-go-file-lines.sh` — pass; largest source is under 800 lines.
- Source scan — no Git subprocess and no legacy runtime package/import/config.
- CLI local OpenAI-compatible E2E — pass with `PATH` empty; JSON stable,
  source unchanged, no review-time workspace writes.
- Real subprocess lock contention — pass.
- `govulncheck` — unavailable locally; explicit evidence gap.

Authenticated Codex/Claude/Agy reviews and PDF extraction were not invoked in
this gate; they remain bounded smoke gaps, not verified claims.
