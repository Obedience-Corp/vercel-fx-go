// Package fx is a Go SDK for the Vercel fx coding agent CLI.
//
// It wraps the installed fx binary: one-shot requests through "fx ask --json",
// a long-lived streaming session through the "fx acp" Agent Client Protocol
// server, and typed wrappers for the --json admin commands. The SDK never
// reads ~/.fx/auth.json and never writes ~/.fx/settings.json; it passes
// process overrides (FX_MODEL, FX_PERMISSION_MODE, FX_MAX_AGENT_STEPS)
// instead.
//
// Known fx v0.0.4 limitation: in ACP "ask" mode fx never emits
// session/request_permission and the turn stalls after tool_call pending, so a
// headless consumer should use yolo through the dangerous subpackage in a
// disposable workspace, or auto and accept the billed openai/gpt-5.4 reviewer.
// See docs/ACP.md.
package fx
