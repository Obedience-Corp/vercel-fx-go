# Architecture

## Shape

The SDK is a process wrapper. It never speaks to the AI Gateway itself: it
spawns the installed `fx` binary, passes overrides through argv and the
environment, and decodes what fx writes to stdout.

```
your code
   |
   |  fx.Client
   v
exec.CommandContext ---> fx ask --json        (one shot, JSON on stdout)
                    \--> fx acp               (long lived, JSON-RPC over stdio)
                    \--> fx <cmd> --json      (admin reads)
```

## Packages

| Path | Contents |
| --- | --- |
| `pkg/fx` | the whole client surface |
| `pkg/fx/dangerous` | yolo mode and `fx upgrade`, behind an explicit opt-in |
| `test/mockfx` | a Go binary that impersonates fx for the tests |
| `test/testdata` | sanitized fx v0.0.6 contract fixtures and mock scripts |
| `test/integration` | `-tags=integration`, mock lane by default |
| `examples` | small programs that compile against the public API |

Inside `pkg/fx`:

| File | Responsibility |
| --- | --- |
| `client.go` | `Client`, the `execCommand` seam, cwd and env plumbing |
| `locate.go` | finding the fx binary |
| `options.go` | `AskOptions`, validation, deep clone |
| `args.go` | `BuildAskArgs`, `BuildACPArgs`, `BuildEnv` |
| `ask.go` | `Ask`, `AskResult`, `ToolCall`, `Recovery` |
| `errors.go` | `Error`, `Kind`, `Classify`, retry predicates |
| `retry.go` | the opt-in `RetryPolicy` |
| `admin.go` | `runJSON` and the read-only command wrappers |
| `sessions.go` | session, background, and workspace wrappers |
| `state.go` | read-only helpers over `~/.fx` |
| `login.go` | provider-aware interactive command builders and `LoginURL` |
| `acp.go` | the ACP wire types |
| `acp_session.go` | `StartACP`, the read pump, `Call`, `Notify`, `Close` |
| `acp_prompt.go` | session lifecycle, `Prompt`, `CollectPrompt` |
| `acp_handlers.go` | `PermissionHandler` and the agent request dispatch |
| `process.go` | graceful stop: SIGTERM, then SIGKILL after five seconds |

## Process model

Every call builds its own `exec.Cmd`, so `Client` is safe for concurrent use.
`cmd.Dir` is always set, falling back to the process working directory, because
fx treats the cwd as the primary workspace and an unset cwd would silently
change which files the agent can reach.

The environment is layered so the safety overrides cannot be defeated:

```
os.Environ()  +  Client.Env  +  AskOptions.Env  +  typed overrides  +  FX_AUTO_UPGRADE=0, FX_NO_OPEN_BROWSER=1
```

The last two entries win because a later `KEY=value` shadows an earlier one.

## Result handling

`fx ask --json` writes its JSON object even when it exits non-zero, so the SDK
parses stdout first and classifies afterwards. `AskCtx` returns
`(result, nil)` on exit code zero and `(result, *Error)` otherwise, which lets a
caller read `Recovery` off a failure instead of losing it.

`Classify` looks at, in order: `recovery.cause`, an `HTTP <code>` pattern in the
result error, output, recovery message, or stderr, a permission denial marker,
an authentication marker, exit code 130, then a generic process failure.

## ACP transport

`StartACP` spawns `fx acp`, then runs three goroutines: a read pump over stdout,
a bounded stderr drain, and one that owns `cmd.Wait`. The read pump routes
responses through a pending map keyed by request id, pushes `session/update`
notifications onto a buffered channel, and dispatches agent requests to a
handler on a separate goroutine so a slow handler never stalls the stream.

Channel closing is guarded by a read-write mutex rather than by ordering alone,
so a handler that returns during shutdown cannot send on a closed channel.
`Close` does not wait for user callbacks that ignore cancellation. SDK-owned
goroutines are checked with `goleak`.

## Error type

One error type, `*fx.Error`, carries `Kind`, the exit code, the HTTP status when
one was printed, the fx `Recovery` block, stderr, and the wrapped cause. It
implements `Unwrap`, so `errors.Is` and `errors.As` reach the original error.

Constructors are internal helpers (`validationError`, `transportError`,
`processError`); non-test code never calls `fmt.Errorf`.

## Dependencies

Standard library only in non-test code. `go.uber.org/goleak` is imported from
test files only.
