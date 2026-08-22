# The fx ACP protocol as observed

Transport: `fx [leading globals] acp [--model <id>] [--log-file <abs path>]`,
newline-delimited JSON-RPC 2.0 over stdin and stdout, protocol version `1`
(Agent Client Protocol, https://agentclientprotocol.com).

The process cwd is the primary workspace. There is one active session and one
active prompt per connection, each input message is capped at 8 MiB, and stdout
is reserved for protocol frames. Diagnostics go to `--log-file` or
`FX_TRACE_LOG`. Initialization fails when no usable Gateway credential exists.

`--log-file` must be an absolute path. `ACPConfig.validate` enforces that,
because a relative path under the workspace made the server exit before
answering the first request.

Every transcript quoted here is in `test/testdata/acp/`.

## Handshake

Client:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,
 "clientCapabilities":{"fs":{"readTextFile":false,"writeTextFile":false},"terminal":false}}}
```

Agent, verbatim from fx v0.0.4:

```json
{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,
 "agentCapabilities":{"loadSession":true,
   "promptCapabilities":{"image":false,"audio":false,"embeddedContext":true},
   "mcpCapabilities":{"http":true,"sse":true},
   "sessionCapabilities":{"list":{},"resume":{},"close":{}}},
 "agentInfo":{"name":"fx","title":"fx","version":"0.0.4"},"authMethods":[]}}
```

`authMethods` is empty because authentication is the binary's job.
`embeddedContext:true` means `resource` and `resource_link` prompt blocks are
accepted; image and audio blocks are rejected.

The SDK advertises no client capabilities by default. fx uses its own tools and
sandbox, so advertising `fs` or `terminal` would only invite requests the SDK
would then have to refuse.

## Session lifecycle

| Method | Params | Result | SDK |
| --- | --- | --- | --- |
| `session/new` | `{"cwd","mcpServers"}` | `{"sessionId","configOptions":[...]}` then an `available_commands_update` | `NewSession` |
| `session/load` | `{"sessionId","cwd","mcpServers"}` | replays history as updates (docs, not probed) | `LoadSession` |
| `session/resume` | same | reconnects without replay (docs, not probed) | `ResumeSession` |
| `session/list` | `{}` | `{"sessions":[{"sessionId","cwd","updatedAt"}]}`, newest first | `ListSessions` |
| `session/close` | `{"sessionId"}` | `{}` | `CloseSession` |
| `session/cancel` | `{"sessionId"}`, a notification | the active prompt returns `stopReason: cancelled` | `Cancel` |
| `session/set_config_option` | `{"sessionId","configId":"model","value"}` | `{"configOptions":[...]}` | `SetModel` |
| `session/set_mode` | `{"sessionId","modeId":"ask"\|"code"}` | `null` | `SetMode` |

`session/new` returns the 230-entry model catalog inline as the `model` config
option. Session ids are the same ids `fx sessions` reports, and ACP sessions
show up in `fx sessions --json` for that workspace.

`available_commands_update` lists 17 slash commands: compact, undo, changes,
review, clear, reset, help, status, model, permissions, allowlist, rules,
settings, credits, mcp, skills, fast. They are commands for an interactive
shell, not tools; the SDK surfaces them as data.

## Prompt turn

```json
{"jsonrpc":"2.0","id":4,"method":"session/prompt","params":{"sessionId":"...",
 "prompt":[{"type":"text","text":"..."}]}}
```

The turn ends with `{"stopReason":"end_turn"}` (observed), `"refused"` (observed
after a provider outage exhausted recovery), or `cancelled`,
`max_turn_requests`, `max_tokens` per the spec. There is no `_meta` and there
are no token counts. grok's ACP puts tokens in `_meta`; fx does not.

### session/update kinds

All arrive as `params:{sessionId, update:{sessionUpdate: ...}}`.

| Kind | Fields observed |
| --- | --- |
| `agent_message_chunk` | `content:{type:"text",text}`. Whitespace-only chunks are common |
| `tool_call` | `toolCallId:"call_<24 hex>"`, `title`, `kind:"execute"\|"read"\|"edit"`, `status:"pending"`. No tool name and no `rawInput` in 0.0.4 |
| `tool_call_update` | `toolCallId`, `status`, `content:[{type:"content",content:{type:"text",text}}]` on terminal statuses only |
| `session_info_update` | `_meta.fx.modelResponseRecovery`, the fx retry state |
| `available_commands_update` | `availableCommands:[...]`, after `session/new` |

Spec'd but not yet seen from fx: `agent_thought_chunk`, `user_message_chunk`,
`plan`, `current_mode_update`, `config_option_update`. The SDK decodes them
generically and keeps `Raw` on every update, so an unknown kind passes through
with `Kind` set to the raw string instead of failing to decode.

Note that `content` has two shapes under the same key: a single object for
message chunks and an array for tool call updates. `SessionUpdateInner` has a
custom unmarshaler that resolves both into `Content` and `ToolContent`.

### Recovery

`session_info_update._meta.fx.modelResponseRecovery` is fx specific and not in
the ACP spec:

```json
{"state":"paused","kind":"terminal_provider_error","cause":"provider_unavailable",
 "action":"paused","requiredAction":"continue_later","attempt":10,
 "attemptLimit":10,"delaySeconds":0,"durable":true,"message":"..."}
```

It carries the same fields as the `recovery` block in `fx ask --json`, but
camelCased instead of snake_cased. `fx.Recovery` has one unmarshaler that
accepts both spellings, so the same struct serves both transports.

### Failure payloads inside tool results

`tool_call_update.content[].content.text` sometimes holds a JSON string:

```json
{"error":{"type":"tool_permission_denied","tool_name":"terminal",
 "message":"Permission denied by auto mode classifier","reason":"auto_denied",
 "denied":true,"approval_request_id":"<64 hex>","suggestion":"..."}}
```

Treat that as a denied tool result, not a stream error. The turn continues.

## Permission requests: a v0.0.4 caveat

The ACP spec has the agent send a request `session/request_permission` with
`{"sessionId","toolCall":{...},"options":[{"optionId","name","kind"}]}` where
kind is `allow_once`, `allow_always`, `reject_once`, or `reject_always`, and the
client answers `{"outcome":{"outcome":"selected","optionId":"..."}}` or
`{"outcome":{"outcome":"cancelled"}}`.

**fx v0.0.4 was never observed sending it.** Four probes were run on
2026-08-21 with `FX_PERMISSION_MODE=ask`. One never reached a tool call because
the upstream provider exhausted recovery first. The other three all reached the
same state: fx emitted `tool_call` with `status: pending` for a `write_file`
call, then sent nothing further for the rest of the ten minute probe window and
never returned a prompt result. One of those three first issued
`session/set_mode` with `modeId: ask` and got an accepted `null` result, so
selecting the mode explicitly does not change the outcome. The real transcript
of that run is `test/testdata/acp/ask-mode-stall.jsonl`.

The stall is not a provider artifact. In two of the three, the same session had
already logged `modelResponseRecovery` with `state: recovered` before the tool
call, so the model turn itself completed and only the permission step hung.

Consequences for the SDK:

- The handler path is implemented and tested, but against
  `test/testdata/acp/request-permission.UNVERIFIED.jsonl`, which is built from
  the spec shape rather than captured. Replace it with a real transcript when a
  build emits one.
- `PermissionRequest` keeps `Raw`, so an unexpected shape degrades to a deny
  with a reason rather than a decode failure.
- The default handler picks `reject_once`, or `reject_always` if that is the
  only rejection offered, or `cancelled` when nothing matches. It fails closed.
- A host that needs tool execution today should use `yolo` through the
  `dangerous` subpackage in a disposable workspace, or `auto` and accept the
  billed reviewer. Do not build a product flow on ACP `ask` mode until this is
  fixed upstream.

Mode interaction that was verified:

| Process `FX_PERMISSION_MODE` | Behavior |
| --- | --- |
| `yolo` | tools execute with no requests. Verified end to end |
| `auto` (default) | unresolved calls go to the reviewer; a denial is reported to the model as a failed tool call and the client is never asked. Verified |
| `ask` | stalls at a pending tool call. See above |

## Client-side methods

fx may call `fs/read_text_file`, `fs/write_text_file`, and
`terminal/create|output|wait_for_exit|kill|release`, but only when the client
advertised the matching capability during `initialize`. The SDK advertises none,
and answers any such request with JSON-RPC `-32601` method not found. That path
is covered by `test/testdata/acp/unknown-request.jsonl`.

## Differences from grok's ACP

| | grok `agent stdio` | fx `acp` |
| --- | --- | --- |
| `protocolVersion` | string `"2024-11-05"` | integer `1` |
| init `_meta` | `grokShell`, `modelState`, `mcpServers`, `availableCommands` | none; models arrive in `session/new.configOptions` |
| prompt result | `stopReason` plus `_meta` token counts | `stopReason` only |
| model switch | at init | `session/set_config_option` per session, or `--model` per process |
| session capabilities | none | `list`, `resume`, `close`, `loadSession` |
| recovery state | none | `session_info_update._meta.fx.modelResponseRecovery` |
