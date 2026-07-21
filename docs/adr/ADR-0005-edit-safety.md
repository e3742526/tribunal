# ADR-0005: Typed scoped edit proposals

Accepted. Editors emit replacement hunks bound to accepted findings and source
hashes. The host enforces local/section/document scopes, rejects stale or
escaping targets, writes atomically, preserves originals, and revalidates before
revert. Binary extracted formats are never edited.

