# Gate 6 audit evidence — 2026-07-21

The adversarial audit entered D-006 through D-015 and fixed each with focused
regression coverage. High-risk findings included packet identity omissions,
identity-bearing blind IDs, bypassable redirects, non-monotonic run IDs,
missing mutation locks, and symlink-following lock/journal paths.

Validation after fixes:

- `go test ./...` — pass.
- `go vet ./...` — pass.
- `go test -race ./...` — pass for all packages.
- `go test ./internal/tribunal/storage ./internal/tribunal/app -count=10` — pass
  (`storage` 2.138s, `app` 22.577s).
- Real subprocess lock contention and symlink external-sentinel tests — pass.
- Local Poppler PDF fixture — pass on this macOS host.
- CLI local HTTP/Git-absent test — remains pass.

The local host still lacks `govulncheck`. Authenticated vendor execution and
cross-platform archive smoke are Gate 8 evidence items.
