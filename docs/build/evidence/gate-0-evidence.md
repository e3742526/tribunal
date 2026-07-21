# Gate 0 Evidence

## E0-1 — repository state (REQ-001)

Command from repository root: `git status --short && git rev-parse HEAD && git
branch --show-current && go version`.

Observed: no status output; HEAD
`9dc1982834eaaf871ec1220a5ad283a2137e0c96`; branch `main`; Go
`go1.26.5 darwin/arm64`.

## E0-2 — inherited baseline (orientation)

Command: `go test ./...`.

Observed: root no tests; `internal/cli` passed in 0.425s;
`internal/tagteam` passed in 107.873s; `internal/tui` passed in 0.506s.

## E0-3 — profile

Static inspection found existing AGENTS, README, docs index, CI, release,
Go module, CLI/core/TUI packages, and a 547-test ledger. Giles profile selected.

