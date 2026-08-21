//go:build integration

// Package integration exercises the SDK against a real process.
//
// The default lane runs against the mock fx binary. Set INTEGRATION_REAL=1 to
// run the same flows against the installed fx CLI; that lane needs an
// authenticated fx and bills the configured model.
package integration
