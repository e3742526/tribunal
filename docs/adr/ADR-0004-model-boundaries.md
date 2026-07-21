# ADR-0004: Isolated model adapters

Accepted. Application-owned ports support Codex, Claude, Agy, and direct
OpenAI-compatible calls. Reviewers have isolated working directories,
restricted environment, read-only/no-tool flags, byte/time limits, schema
contracts, and a pass-1 durability barrier. Provider serialization fails closed.

