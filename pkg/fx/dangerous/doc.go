// Package dangerous exposes the fx modes that disable permission checks and
// the command that mutates the installed binary.
//
// Every entry point refuses unless FX_GO_ENABLE_DANGEROUS is set to
// "i-accept-all-risks", and refuses outright when GO_ENV or NODE_ENV is
// "production". Yolo mode turns off permission checks and the command
// sandbox for the spawned process; use it only in disposable workspaces.
//
// The production refusal inspects only GO_ENV and NODE_ENV, so it is a
// best-effort guard rather than a sandbox: a deployment that sets neither
// variable is not protected by it, and nothing here constrains what the agent
// does once yolo mode is on.
package dangerous
