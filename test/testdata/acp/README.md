# ACP scripts and transcripts

Captured against fx v0.0.4 on 2026-08-21 unless noted.

Real transcripts copied from the design evidence package:

- `initialize-and-session-new.jsonl`: handshake, session/new, set_config_option, session/list, session/close
- `prompt-yolo-write.jsonl`: a turn under `FX_PERMISSION_MODE=yolo` (write_file executed)
- `prompt-auto-denied.jsonl`: a turn under the default `auto` mode (classifier denied the terminal tool)
- `prompt-ask-refused-503.jsonl`: a turn under `ask` that the provider outage refused
- `ask-mode-stall.jsonl`: a full `FX_PERMISSION_MODE=ask` session, including
  `session/set_mode` with `modeId: ask`, in which fx announced the write_file
  tool call as `status: pending` and then never sent `session/request_permission`
  and never returned a prompt result. Captured 2026-08-21. Four probes were run;
  three reached a tool call and all three stalled the same way.

Scripts the mock binary replays, composed from the transcripts above so one
session id is used throughout:

- `full-turn.jsonl`: handshake, session, set_config_option, one write turn, list, close
- `refused-503.jsonl`: handshake, session, one turn the provider outage refused
- `cancel.jsonl`: handshake, session, a turn with several chunks so a cancel can land mid stream
- `request-permission.UNVERIFIED.jsonl`: the permission handshake built from the
  ACP spec shape in design doc 02. fx v0.0.4 was never observed sending
  `session/request_permission`, so this script is not a capture. Replace it
  with a real transcript when a build emits one, and drop the marker from the
  filename when you do.
- `unknown-request.jsonl`: an agent request for a capability the client never
  advertised, so the SDK must answer JSON-RPC -32601
