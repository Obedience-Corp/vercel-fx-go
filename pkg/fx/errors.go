package fx

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Kind classifies an fx failure.
type Kind string

// Error kinds returned by Classify and the SDK helpers.
const (
	KindAuth                Kind = "auth"
	KindModelNotFound       Kind = "model_not_found"
	KindProviderUnavailable Kind = "provider_unavailable"
	KindRateLimit           Kind = "rate_limit"
	KindPermissionBlocked   Kind = "permission_blocked"
	KindInterrupted         Kind = "interrupted"
	KindValidation          Kind = "validation"
	KindProcess             Kind = "process"
	KindTransport           Kind = "transport"
	KindUnknown             Kind = "unknown"
)

// Error is the single error type returned by every SDK call.
type Error struct {
	Kind       Kind
	Message    string
	ExitCode   int
	HTTPStatus int
	Recovery   *Recovery
	Stderr     string
	Original   error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return string(e.Kind)
	}
	return string(e.Kind) + ": " + e.Message
}

// Unwrap exposes the wrapped cause to errors.Is and errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Original
}

// IsRetryable reports whether retrying the same call could succeed.
func (e *Error) IsRetryable() bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case KindProviderUnavailable, KindRateLimit, KindTransport:
		return true
	}
	return false
}

// RetryDelay is the delay fx recommends before the next attempt.
func (e *Error) RetryDelay() time.Duration {
	if e == nil || e.Recovery == nil || e.Recovery.DelaySeconds <= 0 {
		return 0
	}
	return time.Duration(e.Recovery.DelaySeconds) * time.Second
}

// IsRetryable reports whether err is a retryable *Error.
func IsRetryable(err error) bool {
	var fxErr *Error
	if errors.As(err, &fxErr) {
		return fxErr.IsRetryable()
	}
	return false
}

// RetryDelay returns the recommended delay carried by err, or zero.
func RetryDelay(err error) time.Duration {
	var fxErr *Error
	if errors.As(err, &fxErr) {
		return fxErr.RetryDelay()
	}
	return 0
}

func validationError(message string) *Error {
	return &Error{Kind: KindValidation, Message: message}
}

func validationErrorWith(message string, cause error) *Error {
	return &Error{Kind: KindValidation, Message: message, Original: cause}
}

func transportError(message string, cause error) *Error {
	return &Error{Kind: KindTransport, Message: message, Original: cause}
}

func processError(message string, exitCode int, stderr string, cause error) *Error {
	return &Error{Kind: KindProcess, Message: message, ExitCode: exitCode, Stderr: stderr, Original: cause}
}

var httpStatusRe = regexp.MustCompile(`HTTP\s+(\d{3})`)

// Classify turns an fx ask outcome into a typed error, or nil when it succeeded.
func Classify(result *AskResult, stderr string, exitCode int, err error) *Error {
	if exitCode == 0 && err == nil {
		return nil
	}
	out := &Error{ExitCode: exitCode, Stderr: stderr, Original: err}
	text := classifyText(result, stderr)
	if result != nil {
		out.Recovery = result.Recovery
		out.Message = result.failureMessage()
	}
	if out.Message == "" {
		out.Message = firstLine(stderr)
	}
	if out.Message == "" {
		out.Message = "fx exited with code " + strconv.Itoa(exitCode)
	}
	kind, status := classifyKind(out.Recovery, text, exitCode)
	out.Kind = kind
	out.HTTPStatus = status
	return out
}

func classifyKind(rec *Recovery, text string, exitCode int) (Kind, int) {
	if kind, ok := recoveryKind(rec); ok {
		if _, status := httpKind(text); status != 0 {
			return kind, status
		}
		return kind, 0
	}
	if kind, status := httpKind(text); kind != "" {
		return kind, status
	}
	if isPermissionBlocked(text) {
		return KindPermissionBlocked, 0
	}
	if isAuthFailure(text) {
		return KindAuth, 0
	}
	if exitCode == 130 {
		return KindInterrupted, 0
	}
	return KindProcess, 0
}

func recoveryKind(rec *Recovery) (Kind, bool) {
	if rec == nil {
		return "", false
	}
	switch rec.Cause {
	case "provider_unavailable":
		return KindProviderUnavailable, true
	case "rate_limited", "rate_limit":
		return KindRateLimit, true
	}
	return "", false
}

func httpKind(text string) (Kind, int) {
	m := httpStatusRe.FindStringSubmatch(text)
	if m == nil {
		return "", 0
	}
	code, err := strconv.Atoi(m[1])
	if err != nil {
		return "", 0
	}
	switch {
	case code == 401 || code == 403:
		return KindAuth, code
	case code == 404 && strings.Contains(text, "model_not_found"):
		return KindModelNotFound, code
	case code == 429:
		return KindRateLimit, code
	case code >= 500:
		return KindProviderUnavailable, code
	}
	return KindUnknown, code
}

func isPermissionBlocked(text string) bool {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "tool_permission_denied") {
		return true
	}
	return strings.Contains(lower, "permission") &&
		(strings.Contains(lower, "denied") || strings.Contains(lower, "unresolved") || strings.Contains(lower, "blocked"))
}

func isAuthFailure(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range []string{"sign in", "credential", "unauthorized", "not authenticated", "fx login"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func classifyText(result *AskResult, stderr string) string {
	parts := make([]string, 0, 4)
	if result != nil {
		if result.Error != "" {
			parts = append(parts, result.Error)
		}
		if result.Output != "" {
			parts = append(parts, result.Output)
		}
		if result.Recovery != nil && result.Recovery.Message != "" {
			parts = append(parts, result.Recovery.Message)
		}
	}
	if stderr != "" {
		parts = append(parts, stderr)
	}
	return strings.Join(parts, "\n")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
