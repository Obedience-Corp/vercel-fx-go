// Package dangerous exposes the fx modes that disable permission checks and
// the command that mutates the installed binary.
//
// Every entry point refuses unless FX_GO_ENABLE_DANGEROUS is set to
// "i-accept-all-risks", and refuses outright when GO_ENV or NODE_ENV is
// "production". Yolo mode turns off permission checks and the command
// sandbox for the spawned process; use it only in disposable workspaces.
package dangerous
