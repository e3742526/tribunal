# ADR-0002: Content-addressed packets and external durable state

Accepted. Raw source hashes establish freshness; redacted/extracted packet
hashes establish model input. Workspace IDs derive from canonical directory
paths. State is external/private, writes are atomic, state transitions are
journaled before snapshots, and paths are revalidated before sensitive I/O.

