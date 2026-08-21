package fx

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func decodeFixture(t *testing.T, name string) *AskResult {
	t.Helper()
	var result AskResult
	if err := json.Unmarshal(readFixture(t, name), &result); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return &result
}

func TestClassifyFixtures(t *testing.T) {
	tests := []struct {
		name         string
		fixture      string
		exitCode     int
		stderr       string
		wantKind     Kind
		wantStatus   int
		wantRetry    bool
		wantRecovery string
	}{
		{
			name:     "provider outage paused after ten attempts",
			fixture:  "ask-503-paused.json",
			exitCode: 1,
			wantKind: KindProviderUnavailable, wantStatus: 503, wantRetry: true, wantRecovery: "paused",
		},
		{
			name:     "unknown model reported in output only",
			fixture:  "ask-model-not-found.json",
			exitCode: 1,
			wantKind: KindModelNotFound, wantStatus: 404,
		},
		{
			name:     "recovered success is not an error",
			fixture:  "ask-success.json",
			exitCode: 0,
			wantKind: "",
		},
		{
			name:     "tool write success is not an error",
			fixture:  "ask-tool-write.json",
			exitCode: 0,
			wantKind: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := decodeFixture(t, tc.fixture)
			err := Classify(result, tc.stderr, tc.exitCode, nil)
			if tc.wantKind == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected kind %s, got nil", tc.wantKind)
			}
			if err.Kind != tc.wantKind {
				t.Fatalf("kind %s, want %s", err.Kind, tc.wantKind)
			}
			if err.HTTPStatus != tc.wantStatus {
				t.Fatalf("http status %d, want %d", err.HTTPStatus, tc.wantStatus)
			}
			if err.IsRetryable() != tc.wantRetry {
				t.Fatalf("retryable %v, want %v", err.IsRetryable(), tc.wantRetry)
			}
			if tc.wantRecovery != "" && (err.Recovery == nil || err.Recovery.State != tc.wantRecovery) {
				t.Fatalf("recovery %+v, want state %s", err.Recovery, tc.wantRecovery)
			}
		})
	}
}

func TestClassifyFromStderrOnly(t *testing.T) {
	stderr := string(readFixture(t, "ask-503-stderr.txt"))
	err := Classify(&AskResult{}, stderr, 1, nil)
	if err == nil || err.Kind != KindProviderUnavailable {
		t.Fatalf("got %v, want provider_unavailable", err)
	}
	if err.HTTPStatus != 503 {
		t.Fatalf("http status %d, want 503", err.HTTPStatus)
	}
}

func TestClassifyKinds(t *testing.T) {
	tests := []struct {
		name     string
		result   *AskResult
		stderr   string
		exitCode int
		want     Kind
	}{
		{name: "no result and no failure", exitCode: 0, want: ""},
		{name: "interrupt", exitCode: 130, want: KindInterrupted},
		{name: "rate limited", stderr: "API request failed HTTP 429 rate_limited", exitCode: 1, want: KindRateLimit},
		{name: "unauthorized", stderr: "HTTP 401 unauthorized", exitCode: 1, want: KindAuth},
		{name: "sign in prompt", stderr: "Sign in with fx login first", exitCode: 1, want: KindAuth},
		{name: "permission denial", stderr: `{"error":{"type":"tool_permission_denied"}}`, exitCode: 1, want: KindPermissionBlocked},
		{name: "bare failure", stderr: "something went wrong", exitCode: 3, want: KindProcess},
		{name: "unmapped http code", stderr: "HTTP 418 teapot", exitCode: 1, want: KindUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Classify(tc.result, tc.stderr, tc.exitCode, nil)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil || err.Kind != tc.want {
				t.Fatalf("got %v, want kind %s", err, tc.want)
			}
		})
	}
}

func TestRetryDelayFromRecovery(t *testing.T) {
	err := &Error{Kind: KindProviderUnavailable, Recovery: &Recovery{DelaySeconds: 30}}
	if got := err.RetryDelay(); got != 30*time.Second {
		t.Fatalf("delay %v, want 30s", got)
	}
	if got := RetryDelay(error(err)); got != 30*time.Second {
		t.Fatalf("package delay %v, want 30s", got)
	}
	if !IsRetryable(error(err)) {
		t.Fatal("provider_unavailable must be retryable")
	}
	if IsRetryable(errors.New("plain")) {
		t.Fatal("a plain error must not be retryable")
	}
	if got := (&Error{Kind: KindValidation}).RetryDelay(); got != 0 {
		t.Fatalf("delay %v, want 0", got)
	}
}

func TestRecoveryAcceptsBothSpellings(t *testing.T) {
	tests := []struct {
		name string
		blob string
	}{
		{
			name: "ask json snake case",
			blob: `{"state":"paused","kind":"terminal_provider_error","cause":"provider_unavailable","action":"paused","required_action":"continue_later","attempt":10,"attempt_limit":10,"delay_seconds":30,"durable":true}`,
		},
		{
			name: "acp meta camel case",
			blob: `{"state":"paused","kind":"terminal_provider_error","cause":"provider_unavailable","action":"paused","requiredAction":"continue_later","attempt":10,"attemptLimit":10,"delaySeconds":30,"durable":true}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var rec Recovery
			if err := json.Unmarshal([]byte(tc.blob), &rec); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if rec.RequiredAction != "continue_later" || rec.AttemptLimit != 10 || rec.DelaySeconds != 30 {
				t.Fatalf("decoded %+v", rec)
			}
			if !rec.Paused() || !rec.Durable {
				t.Fatalf("decoded %+v", rec)
			}
		})
	}
}

func TestToolCallKeepsUnknownFields(t *testing.T) {
	var call ToolCall
	blob := `{"name":"write_file","status":"success","path":"hello.txt","bytes":2}`
	if err := json.Unmarshal([]byte(blob), &call); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if call.Name != "write_file" || call.Status != "success" {
		t.Fatalf("decoded %+v", call)
	}
	if string(call.Extra["path"]) != `"hello.txt"` || string(call.Extra["bytes"]) != "2" {
		t.Fatalf("extra %v", call.Extra)
	}
	roundTrip, err := json.Marshal(call)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back ToolCall
	if err := json.Unmarshal(roundTrip, &back); err != nil {
		t.Fatalf("decode round trip: %v", err)
	}
	if back.Name != call.Name || string(back.Extra["path"]) != `"hello.txt"` {
		t.Fatalf("round trip lost data: %+v", back)
	}
}
