package fx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
)

// AskResult is the JSON object "fx ask --json" writes on stdout.
type AskResult struct {
	Output    string          `json:"output"`
	ExitCode  int             `json:"exit_code"`
	Model     string          `json:"model"`
	SessionID string          `json:"session_id"`
	Steps     int             `json:"steps"`
	ToolCalls []ToolCall      `json:"tool_calls"`
	Error     string          `json:"error,omitempty"`
	Recovery  *Recovery       `json:"recovery,omitempty"`
	Raw       json.RawMessage `json:"-"`
	Stderr    string          `json:"-"`
}

func (r *AskResult) failureMessage() string {
	if r == nil {
		return ""
	}
	if r.Error != "" {
		return firstLine(r.Error)
	}
	if r.Output != "" {
		return firstLine(r.Output)
	}
	if r.Recovery != nil {
		return firstLine(r.Recovery.Message)
	}
	return ""
}

// ToolCall is one entry of AskResult.ToolCalls. Fields fx adds beyond name and
// status are preserved in Extra.
type ToolCall struct {
	Name   string
	Status string
	Extra  map[string]json.RawMessage
}

// UnmarshalJSON decodes a tool call and keeps every unknown field.
func (t *ToolCall) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	t.Name = rawString(raw, "name")
	t.Status = rawString(raw, "status")
	delete(raw, "name")
	delete(raw, "status")
	if len(raw) > 0 {
		t.Extra = raw
	}
	return nil
}

// MarshalJSON re-emits the tool call including the preserved unknown fields.
func (t ToolCall) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(t.Extra)+2)
	for k, v := range t.Extra {
		out[k] = v
	}
	out["name"] = json.RawMessage(strconv.Quote(t.Name))
	out["status"] = json.RawMessage(strconv.Quote(t.Status))
	return json.Marshal(out)
}

// Recovery is the fx internal retry state. It arrives snake_cased in ask JSON
// and camelCased in the ACP session_info_update meta.
type Recovery struct {
	State          string `json:"state,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Cause          string `json:"cause,omitempty"`
	Action         string `json:"action,omitempty"`
	RequiredAction string `json:"required_action,omitempty"`
	Message        string `json:"message,omitempty"`
	Attempt        int    `json:"attempt,omitempty"`
	AttemptLimit   int    `json:"attempt_limit,omitempty"`
	DelaySeconds   int    `json:"delay_seconds,omitempty"`
	Durable        bool   `json:"durable,omitempty"`
}

// UnmarshalJSON accepts both the snake_case and camelCase spellings fx emits.
func (r *Recovery) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.State = rawString(raw, "state")
	r.Kind = rawString(raw, "kind")
	r.Cause = rawString(raw, "cause")
	r.Action = rawString(raw, "action")
	r.RequiredAction = rawString(raw, "required_action", "requiredAction")
	r.Message = rawString(raw, "message")
	r.Attempt = rawInt(raw, "attempt")
	r.AttemptLimit = rawInt(raw, "attempt_limit", "attemptLimit")
	r.DelaySeconds = rawInt(raw, "delay_seconds", "delaySeconds")
	r.Durable = rawBool(raw, "durable")
	return nil
}

// Paused reports whether fx stopped the turn and needs an explicit resume.
func (r *Recovery) Paused() bool {
	return r != nil && r.State == "paused"
}

type askInput struct {
	prompt    string
	stdin     []byte
	fromStdin bool
}

const maxStdinPromptBytes = 8 * 1024 * 1024

// Ask runs one "fx ask" with a background context.
func (c *Client) Ask(prompt string, opts *AskOptions) (*AskResult, error) {
	return c.AskCtx(context.Background(), prompt, opts)
}

// AskCtx runs one "fx ask". It returns the parsed result even when fx fails,
// so callers can inspect Recovery on the returned *Error.
func (c *Client) AskCtx(ctx context.Context, prompt string, opts *AskOptions) (*AskResult, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, validationError("prompt must not be empty; use AskFromStdinCtx for stdin input")
	}
	result, err := c.ask(ctx, askInput{prompt: prompt}, opts)
	if err != nil {
		return result, err
	}
	return result, nil
}

// AskFromStdin pipes a prompt into "fx ask" with a background context.
func (c *Client) AskFromStdin(r io.Reader, opts *AskOptions) (*AskResult, error) {
	return c.AskFromStdinCtx(context.Background(), r, opts)
}

// AskFromStdinCtx pipes a prompt into "fx ask" instead of passing it in argv.
func (c *Client) AskFromStdinCtx(ctx context.Context, r io.Reader, opts *AskOptions) (*AskResult, error) {
	if r == nil {
		return nil, validationError("stdin reader must not be nil")
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, &Error{Kind: KindInterrupted, Message: "context done before reading stdin prompt", Original: ctxErr}
	}
	data, readErr := io.ReadAll(io.LimitReader(r, maxStdinPromptBytes+1))
	if readErr != nil {
		return nil, validationErrorWith("read prompt from stdin", readErr)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, &Error{Kind: KindInterrupted, Message: "context done while reading stdin prompt", Original: ctxErr}
	}
	if len(data) > maxStdinPromptBytes {
		return nil, validationError("stdin prompt exceeds fx 8 MiB limit")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, validationError("stdin prompt must not be empty")
	}
	result, err := c.ask(ctx, askInput{stdin: data, fromStdin: true}, opts)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (c *Client) ask(ctx context.Context, in askInput, opts *AskOptions) (*AskResult, *Error) {
	prepared, prepErr := c.prepareAsk(opts)
	if prepErr != nil {
		return nil, prepErr
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, &Error{Kind: KindInterrupted, Message: "context done before fx ask", Original: ctxErr}
	}
	dir, dirErr := c.workDir(prepared.WorkingDirectory)
	if dirErr != nil {
		return nil, dirErr
	}
	args := BuildAskArgs(in.prompt, prepared)
	env := BuildEnv(prepared)
	return c.askWithRetry(ctx, args, env, dir, in, prepared)
}

func (c *Client) askWithRetry(ctx context.Context, args, env []string, dir string, in askInput, opts *AskOptions) (*AskResult, *Error) {
	policy := opts.RetryPolicy
	attempts := policy.attempts()
	var lastResult *AskResult
	var lastErr *Error
	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := c.askOnce(ctx, args, env, dir, in, opts.Timeout)
		if err == nil {
			return result, nil
		}
		lastResult, lastErr = result, err
		if attempt == attempts || !err.IsRetryable() {
			break
		}
		if sleepErr := sleepCtx(ctx, policy.delayFor(attempt, err)); sleepErr != nil {
			return lastResult, &Error{Kind: KindInterrupted, Message: "context done between fx ask retries", Original: sleepErr}
		}
	}
	return lastResult, lastErr
}

func (c *Client) askOnce(ctx context.Context, args, env []string, dir string, in askInput, timeout time.Duration) (*AskResult, *Error) {
	runCtx, cancel := contextWithTimeout(ctx, timeout)
	defer cancel()
	var stdin *bytes.Reader
	if in.fromStdin {
		stdin = bytes.NewReader(in.stdin)
	}
	outcome := c.runCommand(runCtx, args, env, dir, stdin)
	if ctxErr := runCtx.Err(); ctxErr != nil {
		return nil, &Error{Kind: KindInterrupted, Message: "fx ask canceled", ExitCode: outcome.exitCode, Stderr: outcome.stderr, Original: ctxErr}
	}
	if outcome.exitCode < 0 && outcome.err != nil {
		return nil, transportError("run fx ask", outcome.err)
	}
	result, parseErr := parseAskResult(outcome.stdout)
	if parseErr != nil {
		parseErr.ExitCode = outcome.exitCode
		parseErr.Stderr = outcome.stderr
		return nil, parseErr
	}
	result.Stderr = outcome.stderr
	return result, Classify(result, outcome.stderr, outcome.exitCode, outcome.err)
}

func parseAskResult(stdout []byte) (*AskResult, *Error) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return nil, validationError("fx ask produced no output on stdout")
	}
	var result AskResult
	if err := json.Unmarshal(trimmed, &result); err != nil {
		return nil, validationErrorWith("fx ask stdout is not JSON: "+truncate(trimmed, 400), err)
	}
	result.Raw = append(json.RawMessage(nil), trimmed...)
	return &result, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
