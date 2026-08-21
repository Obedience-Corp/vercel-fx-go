package fx

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func stopGracefully(cmd *exec.Cmd, waitDone <-chan error) error {
	select {
	case err := <-waitDone:
		return normalizeStopExit(err)
	default:
	}
	if cmd.Process == nil {
		return nil
	}
	signalErr := cmd.Process.Signal(syscall.SIGTERM)
	select {
	case err := <-waitDone:
		if isExpectedStopExit(err) {
			return nil
		}
		if signalErr != nil {
			return signalErr
		}
		return normalizeStopExit(err)
	case <-time.After(5 * time.Second):
		return killAndWait(cmd, waitDone)
	}
}

func killAndWait(cmd *exec.Cmd, waitDone <-chan error) error {
	var killErr error
	if cmd.Process != nil {
		killErr = cmd.Process.Kill()
	}
	err := <-waitDone
	if isExpectedStopExit(err) {
		return nil
	}
	if killErr != nil {
		return killErr
	}
	return normalizeStopExit(err)
}

func normalizeStopExit(err error) error {
	if isExpectedStopExit(err) {
		return nil
	}
	return err
}

func isExpectedStopExit(err error) bool {
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return false
	}
	switch status.Signal() {
	case syscall.SIGTERM, syscall.SIGKILL, syscall.SIGPIPE:
		return true
	}
	return false
}
