# Troubleshooting

## Exit 3: degraded panel

Run `tribunal transcript <input> --json`. Inspect each review `status.json` and
raw output under the external run directory. `doctor` distinguishes missing
provider CLIs from malformed output. Tribunal requires at least two valid
reviewers and a majority of the configured panel.

## Packet exceeds context

Rerun with `--split`. The smallest configured reviewer budget controls one
deterministic UTF-8/section-safe chunk map shared by the entire panel.

## PDF unavailable

Install Poppler so `pdftotext -v` succeeds, then run `tribunal doctor`. DOCX
uses built-in ZIP/XML extraction and needs no external tool.

## State-root refusal

The state root cannot contain, or be contained by, the reviewed document root.
Choose an external path with `--state-root`; the default is normally safe.

## Edit stale or out of scope

Rebuild the review if the live file hash changed. Use proposal-only mode to
inspect offsets. Document-wide hunks require explicit confirmation; PDF/DOCX
cannot be edited. Revert refusal means later user changes exist and are being
protected.

## Unknown schema version

Use the Tribunal version that created the artifact or export the data with that
version. Readers fail closed rather than guessing older layouts.
