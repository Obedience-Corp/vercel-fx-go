# vercel-fx-go

[![CI](https://github.com/Obedience-Corp/vercel-fx-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Obedience-Corp/vercel-fx-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/Obedience-Corp/vercel-fx-go/pkg/fx.svg)](https://pkg.go.dev/github.com/Obedience-Corp/vercel-fx-go/pkg/fx)

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

Compatibility target: fx v0.0.6. The SDK API is unstable until v1.0. Use
`fx.TestedFXVersion` for an exact runtime compatibility check.

This is an independent community project. It is not affiliated with or
endorsed by Vercel. Vercel and fx names and marks belong to their respective
owners.

## Requirements

- Go 1.24 or newer
- `fx` installed and authenticated:

```bash
curl -fsSL https://fx.sh/setup.sh | bash
fx login          # or export AI_GATEWAY_API_KEY
```

## Install

```bash
go get github.com/Obedience-Corp/vercel-fx-go@latest
```

## Quick start: one-shot ask

```go
client, err := fx.NewClientFromPath()
if err != nil {
    return err
}
client.WorkingDir = repoDir // fx treats the process cwd as the primary workspace

result, err := client.AskCtx(ctx, "Summarize README.md in one sentence.", &fx.AskOptions{
    Model:  "your-provider/your-model",
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

fx can invoke paid model and tool providers. Pricing, model availability, and
reviewer behavior are controlled by fx and the configured provider, not this
SDK. Validate those settings before running integration tests or autonomous
workflows.

Neither `fx ask --json` nor the ACP prompt result reports token counts. The only
per-session source is `~/.fx/sessions/<id>/usage-v2.json`, which
`fx.SessionUsage(ctx, id)` reads (read only, gated on the schema versions it was
written against).

## Permission modes

| Mode | Behavior |
| --- | --- |
| `ask` | unresolved sensitive calls reach the ACP `PermissionHandler` |
| `auto` | rules first, then fx decides unresolved requests using its configured reviewer |
| `yolo` | no permission checks. Requires `AllowDangerousMode` on `AskOptions` or `ACPConfig`. The `dangerous` subpackage adds an environment opt-in and a best-effort production guard |

fx v0.0.5 and newer do not provide a tool sandbox. All modes can execute host
processes with the permissions of the fx process; `yolo` additionally disables
permission checks. Use a disposable workspace or an external containment
boundary for untrusted prompts and repositories.

Yolo mode lives behind a second gate:

```go
import "github.com/Obedience-Corp/vercel-fx-go/pkg/fx/dangerous"

// FX_GO_ENABLE_DANGEROUS=i-accept-all-risks, and not GO_ENV/NODE_ENV=production
guarded, err := dangerous.Wrap(client)
result, err := guarded.Yolo(ctx, prompt, &fx.AskOptions{NoSave: true})
```

## Documentation

- `docs/ARCHITECTURE.md`: package layout and the process model
- `docs/CLI_REFERENCE.md`: the fx v0.0.6 surface the SDK wraps
- `docs/ACP.md`: the observed ACP wire protocol
- `docs/CONTRIBUTING.md`: gates, the mock binary, and the fixtures

## Development

Filesystem-mutating tests must run in an isolated container. The CI workflow
runs all gates in Debian-based Go containers.

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
