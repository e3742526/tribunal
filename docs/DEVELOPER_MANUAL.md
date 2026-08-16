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

## Adding a panel policy or catalog field

Panel composition is `domain.SelectPanel`, a pure function over a policy and an
operator-declared catalog. Keep it that way: a model must never rank peers,
choose a seat, or compose a panel, because the independence barrier would then
depend on trusting one panelist to pick the others. Built-in policies declare
seat shape only and name no model; `quality`, `reliability`, and `cost` stay
operator-declared priors, so do not ship vendor figures as defaults.

New policy or catalog fields need validation in `domain.ValidatePolicy` or
`domain.ValidateCandidate`, a rejection test, and a decision about precedence
against `--panel`, `TRIBUNAL_PANEL`, and a recorded resume/replay panel — the
recorded panel always wins so a catalog edit cannot repanel a frozen packet.
Apply configured context budgets after selection, never from catalog values
alone. Any shortfall — an unfilled optional seat, a bounded search, a derived
catalog, a family count below the policy — must reach `PanelSelection.Notes`
rather than being silently absorbed.

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
