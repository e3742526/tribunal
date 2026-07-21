# Developer manual

## Dependency direction

The CLI and TUI are presentation only. Use cases live in `app`; pure consensus
and lifecycle rules in `domain`; content extraction and packet identity in
`documents`; external processes/HTTP in `adapters`; state durability in
`storage`; trusted loading in `config`.

Do not import CLI/TUI from an inward package. Do not add Git execution. Do not
let model adapters write documents. An editor adapter returns only an
`EditProposal`; host validation owns all mutation.

## Schemas

Artifacts start at schema version 1; findings are version 2. Readers reject
missing or unknown versions. Change a schema by adding an explicit migration
dispatcher or starting a new version, tests, and documentation—never infer a
version from field presence.

## Adding an adapter

Implement `adapters.Adapter`, bound output/time limits, restrict environment or
HTTP redirects, and register it in `app.DefaultRegistry`. Add request/argv
goldens for reviewer, voter, and editor roles. Provider output must pass a real
JSON Schema and semantic identity validation.

## Persistence changes

Journal a transition before replacing `state.json`. Use atomic writes with
file and parent-directory sync. Revalidate canonical paths after locking and
before sensitive reads/writes. Add fault and resume tests for every new durable
artifact.

## Testing

`scripts/check.sh` is the normal gate. Concurrency, locks, process handling,
network workers, state, or edit changes also require `go test -race ./...`.
The release gate includes clean-checkout and archive smoke tests on macOS and
Linux. Real provider checks are evidence only when installed credentials exist;
skips must remain explicit.
