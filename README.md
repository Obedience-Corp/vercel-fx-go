# vercel-fx-go

Go SDK for the Vercel `fx` coding agent CLI (https://fx.sh, `vercel-labs/fx`).

It wraps the installed `fx` binary the same way `grok-go-sdk` wraps `grok` and
`claude-code-go` wraps `claude`: one-shot requests through `fx ask --json`, a
long-lived streaming session through the `fx acp` Agent Client Protocol server,
and typed wrappers for the `--json` admin commands (status, doctor, models,
permissions, credits, usage, sessions, background, workspace).

Standard library only. The SDK never reads `~/.fx/auth.json` and never writes
`~/.fx/settings.json`; it sets process overrides (`FX_MODEL`,
`FX_PERMISSION_MODE`, `FX_MAX_AGENT_STEPS`) instead, and every spawned process
gets `FX_AUTO_UPGRADE=0` and `FX_NO_OPEN_BROWSER=1` so an SDK call can never
upgrade your install or open a browser.

Status: v0.1.0. The public API is unstable until v1.0.

## Requirements

- Go 1.24 or newer
- `fx` installed and authenticated:

```bash
curl -fsSL https://fx.sh/setup.sh | bash
fx login          # or export AI_GATEWAY_API_KEY
```

## Install

This repository is private, so consumers need `GOPRIVATE` and an SSH rewrite:

```bash
export GOPRIVATE=github.com/Obedience-Corp/*
git config --global url."git@github.com:".insteadOf "https://github.com/"
go get github.com/Obedience-Corp/vercel-fx-go@latest
```

CI that builds a consumer needs the same two settings plus a read token or a
deploy key for this repository.

## Quick start: one-shot ask

```go
client, err := fx.NewClientFromPath()
if err != nil {
    return err
}
client.WorkingDir = repoDir // fx treats the process cwd as the primary workspace

result, err := client.AskCtx(ctx, "Summarize README.md in one sentence.", &fx.AskOptions{
    Model:  "zai/glm-5.2",
    NoSave: true,
})
if result != nil && result.Recovery != nil {
    log.Printf("fx recovery: %s attempt %d/%d", result.Recovery.State,
        result.Recovery.Attempt, result.Recovery.AttemptLimit)
}
if err != nil {
    var fxErr *fx.Error
    if errors.As(err, &fxErr) && fxErr.IsRetryable() {
        time.Sleep(fxErr.RetryDelay())
    }
    return err
}
fmt.Println(result.Output)
```

`AskCtx` returns the parsed result even when fx exits non-zero, so a failure
still gives you `Recovery`, the session id, and the tool calls. `Recovery.Paused()`
means fx gave up after ten internal retries and the turn can be continued later
with `Resume` plus `ContinueRecovery`.

`fx ask` has no `--model` flag. Set `AskOptions.Model` and the SDK exports
`FX_MODEL` for that process only.

## Quick start: streaming ACP session

```go
session, err := client.StartACP(ctx, &fx.ACPConfig{
    Model:             "zai/glm-5.2",
    PermissionHandler: myHandler, // default: reject_once
})
if err != nil {
    return err
}
defer session.Close()

go func() {
    for update := range session.Updates() {
        if update.Update.Kind == fx.UpdateAgentMessageChunk {
            fmt.Print(update.Update.Text())
        }
    }
}()

if _, err := session.Initialize(ctx, fx.ClientCapabilities{}, nil); err != nil {
    return err
}
created, err := session.NewSession(ctx, repoDir, nil)
if err != nil {
    return err
}
result, err := session.PromptText(ctx, created.SessionID, "Add a test for parseArgs.")
```

Drain `Updates()` while a prompt is active. `CollectPrompt` is the convenience
for callers that do not want to stream: it runs the turn and returns the
aggregated text, thoughts, tool calls, and recovery state.

`Close()` sends SIGTERM and escalates to SIGKILL after five seconds.

## Cost notes

The GLM promotional window covers `zai/glm-5.2` only. These paths bill you
separately, whichever model you selected:

- `AskOptions.Auto` (`fx ask --auto`) and the ACP `code` session mode run every
  unresolved permission request through the `openai/gpt-5.4` reviewer.
- `zai/glm-5.2-fast` is a different, billed model. The `-fast` variants are not
  in the promotion.
- Images fall back to `google/gemini-2.5-flash` for vision.
- Web search bills Perplexity calls through the AI Gateway.

Neither `fx ask --json` nor the ACP prompt result reports token counts. The only
per-session source is `~/.fx/sessions/<id>/usage-v2.json`, which
`fx.SessionUsage(ctx, id)` reads (read only, gated on the schema versions it was
written against).

## Permission modes

| Mode | Behavior |
| --- | --- |
| `ask` | unresolved sensitive calls should reach the client. See the caveat below. |
| `auto` | the default: rules first, then the billed reviewer model |
| `yolo` | no permission checks and no sandbox. Requires `AllowDangerousMode` on `AskOptions` or `ACPConfig`. The `dangerous` subpackage is the recommended path because it additionally checks `FX_GO_ENABLE_DANGEROUS=i-accept-all-risks` and refuses when `GO_ENV` or `NODE_ENV` is `production`, which is a best-effort guard and not a sandbox |

Known fx v0.0.4 limitation: in ACP `ask` mode the SDK never observed a
`session/request_permission` request. Four probes were run on 2026-08-21; three
got past the upstream outage to a real tool call, including one that first
issued `session/set_mode` with `modeId: ask`. In all three, fx announced the
tool call with `status: pending` and then never asked the client and never
ended the turn. The handler path is implemented and tested against a
spec-shaped script; see `docs/ACP.md`.

Yolo mode lives behind a second gate:

```go
import "github.com/Obedience-Corp/vercel-fx-go/pkg/fx/dangerous"

// FX_GO_ENABLE_DANGEROUS=i-accept-all-risks, and not GO_ENV/NODE_ENV=production
guarded, err := dangerous.Wrap(client)
result, err := guarded.Yolo(ctx, prompt, &fx.AskOptions{NoSave: true})
```

## Documentation

- `docs/ARCHITECTURE.md`: package layout and the process model
- `docs/CLI_REFERENCE.md`: the fx v0.0.4 surface the SDK wraps
- `docs/ACP.md`: the observed ACP wire protocol
- `docs/CONTRIBUTING.md`: gates, the mock binary, and the fixtures

## Development

```bash
just lint              # gofmt check and go vet
just test all          # unit tests
just test race         # race detector
just test integration  # integration tests against the mock binary
just test integration-real  # the same flows against the real fx (bills the model)
just build all         # library, dangerous, examples, mock
```

## License

Apache-2.0. See `LICENSE`.
