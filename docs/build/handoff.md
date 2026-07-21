# Tribunal v0.1.0 handoff

The repository is intentionally left on `main` with local gate commits and no
remote changes. Start with:

```bash
git status --short
scripts/check.sh
go test -race ./...
```

Key records:

- product contract: `docs/INTENT.md`, `docs/SPEC.md`;
- architecture/contracts: `docs/ARCHITECTURE.md`, `docs/io-contract.md`;
- operator workflow: `README.md`, `docs/USER_MANUAL.md`;
- acceptance: `docs/build/final-qa.md` and Gate 8 evidence;
- known scope: `docs/build/risk-register.md` and `docs/TEST_LEDGER.md`.

No push, tag, release, or publication is authorized by this build. Those are
separate operations requiring explicit user direction.
