# fx CLI reference (v0.0.6)

Validated against the v0.0.6 release source and binary on 2026-08-25. Upstream
docs: https://fx.sh/llms-full.txt.

## Binary

| Fact | Value |
| --- | --- |
| Version | `fx v0.0.6`, released 2026-08-25, `vercel-labs/fx`, Apache-2.0, Zig |
| Install | `curl -fsSL https://fx.sh/setup.sh \| bash`; `FX_INSTALL_DIR` overrides `~/.local/bin` |
| Upgrade | `fx upgrade [--channel stable\|dev] [--json]`; `FX_AUTO_UPGRADE=0` disables checks |
| State | `~/.fx/` |

`LocateBinary` searches PATH, then `$FX_INSTALL_DIR/fx`, `~/.local/bin/fx`,
`/opt/homebrew/bin/fx` on darwin, `/usr/local/bin/fx`, and `/usr/bin/fx`.

## Commands

| Command | Purpose | `--json` | SDK |
| --- | --- | --- | --- |
| `fx` | interactive session, needs a TTY | no | not wrapped |
| `fx ask <prompt>` | one noninteractive request | yes | `Ask`, `AskCtx`, `AskFromStdinCtx` |
| `fx acp` | ACP server over stdio | n/a | `StartACP` |
| `fx status` | effective config and runtime | yes | `Status` |
| `fx doctor` | local health checks | yes | `Doctor` |
| `fx models` | provider-aware model catalog | yes | `Models` |
| `fx permissions` | mode, rules, grants | yes | `Permissions` |
| `fx credits` | AI Gateway balance | yes | `Credits` |
| `fx usage [--period 24h\|7d\|30d]` | local token usage and spend | yes | `Usage` |
| `fx sessions [--all] [--limit 1-100] [--cursor c]` | saved sessions for this workspace | yes | `Sessions` |
| `fx session <last\|id>` / `--id <id>` | inspect one session | yes | `Session` |
| `fx session migrate <id> [--allow-large]` | rewrite to the current format | yes | `SessionMigrate` |
| `fx session recover <id>` | copy a corrupt session into a resumable one | yes | `SessionRecover` |
| `fx background [last\|<id>]` | persisted background commands | yes | `Background`, `BackgroundRecord` |
| `fx workspace [list\|add P\|remove P\|clear]` | additional directories | yes | `WorkspaceList` and friends |
| `fx upgrade` | replaces the installed binary | yes | `dangerous.UpgradeCheck` |
| `fx login/logout [vercel\|codex\|grok]` / `setup` / `provider <gateway\|codex\|grok>` / `teams` | provider and credential flows | varies | typed command builders and `LoginURL` |
| `fx pr` / `fx issue` | draft a PR or issue through `gh` | no | not wrapped |
| `fx replay <tape>` | replay a `--record` tape | yes | not wrapped |

## Global flags come first

`--record`, `--context-limit <name=bytes|off>` (repeatable), `--add-dir <path>`
(repeatable), `--no-additional-dirs`, `-c` / `--continue` / `--resume-last`,
`-r`, `--resume [last|<id>]`, `-h`, `-v`. They must precede the subcommand.

`--add-dir` and `--no-additional-dirs` are accepted only for interactive,
resume, `ask`, `acp`, `pr`, and `issue` launches.

`BuildAskArgs` therefore renders: leading globals, then `ask`, then the ask
flags, then always `--json`, then `--` and the prompt. The separator keeps a
prompt that starts with a dash from being read as a flag.

## fx ask

```
fx ask [--auto|--yolo] [--image PATH] [--json] [--quiet] [--prompt-permissions]
       [--no-save] [--no-color] [--resume <last|id>|--resume-id <id>]
       [--continue-recovery] [--] <prompt>
```

| Flag | Meaning | SDK field |
| --- | --- | --- |
| `--auto` | let fx resolve permission requests using its configured reviewer | `Auto` |
| `--yolo` | no permission checks | `Yolo` plus `AllowDangerousMode` |
| `--image PATH` | attach an image, repeatable, PNG/JPEG/GIF/WebP up to 20 MiB | `Images` |
| `--json` | one JSON object on stdout | always sent |
| `--quiet` | suppress assistant output | `Quiet` |
| `--no-save` | do not create a session; conflicts with resume | `NoSave` |
| `--no-color` | no colors or hyperlinks on a TTY | `NoColor` |
| `--resume <last\|id>` | continue a session | `Resume` |
| `--resume-id <id>` | continue a session by id | `ResumeID` |
| `--continue-recovery` | resume a paused model response | `ContinueRecovery` |

`--prompt-permissions` is not wrapped: it prompts on stderr and only when stdin
is a TTY, which an SDK caller does not have.

The prompt comes from argv, or from stdin when no prompt arguments are given.
`AskFromStdinCtx` uses the stdin form.

Stdout carries the assistant markdown or the JSON object. All progress and
diagnostics go to stderr.

### No model flag

There is no `--model`. The model comes from `FX_MODEL`, settings, and the active
provider. Set `AskOptions.Model` explicitly when deterministic selection is
required.

### Result JSON

```json
{"output":"PONG","exit_code":0,"model":"provider/model","session_id":"","steps":0,
 "tool_calls":[],"recovery":{"state":"recovered","kind":"auto_recovered",
 "attempt":5,"attempt_limit":10,"delay_seconds":0,"durable":false,
 "message":"recovered, succeeded on attempt 5/10"}}
```

- `recovery` is undocumented but present after any internal retry. `durable` is
  true only when a session was saved, which makes the turn resumable with
  `--resume <id> --continue-recovery`.
- The docs say failures "can include an `error` field". It was not observed in
  0.0.6, so the SDK decodes `error` as optional and falls back to `output`.
- `tool_calls[]` always has `name` and `status`. Some tools add fields, so
  `ToolCall` keeps every unknown key in `Extra`.
- The process exit code mirrors `exit_code`, and `130` means interrupted. The
  JSON object is still written on failure, so parse stdout before classifying.
- fx retries internally ten times with 1/2/4/8/16/30/30/30s backoff on HTTP 503
  before pausing recovery. An SDK-level `RetryPolicy` is therefore off by
  default.

## Environment variables

The SDK exposes the documented user-facing set as typed options plus a generic
`Env []string` passthrough:

`AI_GATEWAY_API_KEY`, `VERCEL_OIDC_TOKEN`, `FX_MODEL`, `FX_PERMISSION_MODE`
(`ask|auto|yolo`), `FX_MAX_AGENT_STEPS`, `FX_THEME`, `FX_SOUND`,
`FX_AUTO_UPGRADE`, `FX_NO_OPEN_BROWSER`, `FX_RECORD`, `FX_RECORD_INPUT`,
`FX_TRACE`, `FX_TRACE_LOG`, `FX_TRACE_SCOPES`, `FX_TRACE_STDERR`.

Overrides affect the spawned process only and are never written back to
settings.

## Permission model

- Approval-bearing tools: `write_file`, `edit_file`, `delete_file`,
  `rename_file`, `copy_file`, `create_folder`, `run_command`, `open_file`,
  `install_skill`, `vision`, plus any path outside the workspace for every file
  tool. Read, list, glob, and grep inside the workspace never ask.
- Modes: ACP `ask` delegates unresolved requests to the client, `auto` applies
  rules and the configured reviewer, and `yolo` disables permission checks.
- fx v0.0.5 and newer provide no tool sandbox. Commands execute as host
  processes with the permissions and filesystem access of fx in every mode.
- Rules live in `~/.fx/settings.json` under `permission` and are managed with
  `/allowlist` in the interactive shell. There is no CLI verb, and the SDK never
  writes settings.

## On-disk state

```
~/.fx/
  settings.json     never written by the SDK
  auth.json         never read by the SDK
  usage.jsonl       ReadUsageLog reads the generation facts
  sessions/<id>/usage-v2.json   SessionUsage reads this, schema gated
<workspace>/.fx.json   max_agent_steps, max_tool_result_bytes, context
```

Session ids look like `<created_ms>-<created_ns>-<16 hex>`, for example
`1787339138496-1787339138496206000-337bbe1c41adeae1`.

## Error envelopes

Every `--json` command carries a `kind`. Failures come back as
`{"kind":"<cmd>","error":"...","code":"PascalCaseCode"}`. `NoSavedSessions` was
the code observed. The SDK maps these onto `Kind` and keeps the message.

## Gaps worth knowing

| Capability | fx v0.0.6 |
| --- | --- |
| streaming JSON from the one-shot command | no, ACP only |
| model flag on the one-shot command | no, `FX_MODEL` only |
| system or append prompt flag | none, prompt text or `AGENTS.md` only |
| allowed or denied tools flag | none, settings rules only |
| max turns | `FX_MAX_AGENT_STEPS` only |
| cwd flag | none, the process cwd is the workspace |
| token usage in the result | none, read the session `usage-v2.json` |
| structured output schema | none |
| images | `--image`, ask only, not ACP |
