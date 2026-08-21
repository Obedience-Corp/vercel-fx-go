# vercel-fx-go

Go SDK for the Vercel `fx` coding agent CLI (https://fx.sh, `vercel-labs/fx`).

It wraps the installed `fx` binary the same way `grok-go-sdk` wraps `grok` and
`claude-code-go` wraps `claude`: one-shot requests through `fx ask --json`, a
long-lived streaming session through the `fx acp` Agent Client Protocol server,
and typed wrappers for the `--json` admin commands (status, doctor, models,
permissions, credits, usage, sessions, background, workspace).

Standard library only. The SDK never reads `~/.fx/auth.json` or writes
`~/.fx/settings.json`; it asks the binary and sets process overrides
(`FX_MODEL`, `FX_PERMISSION_MODE`, `FX_MAX_AGENT_STEPS`).

Status: pre-release scaffold. The first implementation lands on the
`feat/sdk-v0.1.0` branch; see the design package
`workflow/design/vercel-fx-go-sdk-and-obey-provider/` in the obey campaign.

## Requirements

- Go 1.24 or newer
- `fx` installed (`curl -fsSL https://fx.sh/setup.sh | bash`) and authenticated (`fx login` or `AI_GATEWAY_API_KEY`)

## Private module

This repository is private. Consumers need:

```bash
export GOPRIVATE=github.com/Obedience-Corp/*
git config --global url."git@github.com:".insteadOf "https://github.com/"
go get github.com/Obedience-Corp/vercel-fx-go@latest
```

## License

Apache-2.0. See `LICENSE`.
