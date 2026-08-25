// Package fx is a Go SDK for the Vercel fx coding agent CLI.
//
// It wraps the installed fx binary: one-shot requests through "fx ask --json",
// a long-lived streaming session through the "fx acp" Agent Client Protocol
// server, and typed wrappers for the --json admin commands. The SDK never
// reads ~/.fx/auth.json and never writes ~/.fx/settings.json; it passes
// process overrides (FX_MODEL, FX_PERMISSION_MODE, FX_MAX_AGENT_STEPS)
// instead.
//
// The API and wire fixtures target fx v0.0.6. Call Version and compare it with
// TestedFXVersion when an application requires an exact compatibility check.
package fx
