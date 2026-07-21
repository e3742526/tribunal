# Gate 8 evidence — 2026-07-21

Final local acceptance:

- `scripts/check.sh` — pass: formatting, 800-line limit, full tests, vet,
  build, module verification, and tidy-diff check.
- `go test -race ./...` — pass for every package.
- Detached clean worktree at Gate 7: README checkout build, `tribunal version
  --json`, `tribunal doctor --json`, and `scripts/check.sh` — pass.
- macOS arm64 release-style binary with version, commit, time, and clean
  provenance: archive, unpack, and `verify-install --json` — `verified`; the
  extracted binary hash matched its adjacent manifest.
- Cross-compiles — pass for darwin/amd64, linux/amd64, linux/arm64, and the
  unsupported Windows amd64 stub.
- Final scans found no predecessor runtime identity, legacy environment/config
  handling, Git subprocess, undocumented TODO/FIXME, or placeholder handler in
  the active source/documentation scope.
- Earlier Gate 6 repetition: storage and application suites passed ten times;
  real subprocess lock contention and external-sentinel symlink tests passed.
- Gate 7 authenticated smoke: Codex and Agy valid through blind voting,
  Claude invalid output isolated, quorum met, exit 2 arbitration continuation.

Explicit evidence gaps:

- `govulncheck`, GoReleaser, and `actionlint` are not installed on this host.
  The repository gate reports scanner absence instead of claiming a scan.
- Linux binaries cross-compile successfully but were not executed on this
  macOS host. Release CI owns Linux runtime/archive execution.
- Windows is intentionally unreleased; its lock implementation is compile-only
  until real multiprocess coverage exists.
- Claude was invoked and bounded correctly but did not return a valid Tribunal
  contract in this smoke. Its failure path is verified; valid content is not.

Temporary binaries, smoke inputs, state roots, and archive trees were confined
to `/tmp`; build/archive artifacts were moved to Trash after inspection. No
remote write, push, tag, release publication, or predecessor-state migration
occurred.
