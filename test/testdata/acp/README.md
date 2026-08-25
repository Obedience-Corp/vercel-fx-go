# ACP contract scripts

These are sanitized, synthetic replay scripts targeting fx v0.0.6. Shapes were
validated against the v0.0.6 release source and its ACP tests on 2026-08-25.
They intentionally contain no user paths, accounts, credentials, or real
session identifiers.

- `full-turn.jsonl`: handshake, session, set_config_option, one write turn, list, close
- `refused-503.jsonl`: handshake, session, one turn the provider outage refused
- `cancel.jsonl`: handshake, session, a turn with several chunks so a cancel can land mid stream
- `request-permission.jsonl`: the v0.0.6 permission handshake, including
  validated `rawInput` and the three options emitted by fx
- `unknown-request.jsonl`: an agent request for a capability the client never
  advertised, so the SDK must answer JSON-RPC -32601
