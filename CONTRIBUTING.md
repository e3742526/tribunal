# Contributing

Tribunal requires Go 1.23 or newer. Inspect `AGENTS.md`, `docs/ARCHITECTURE.md`,
the relevant ADRs, nearby implementation, and tests before changing behavior.

```bash
scripts/check.sh
go test -race ./...
```

Keep dependencies inward, schemas explicitly versioned, review operations
read-only, and state outside document workspaces. Adapter changes require argv
or HTTP golden tests. Persistence and edit changes require failure-path tests.
Pull requests should state the behavior change, exact validation run, evidence
gaps, and residual risks.
