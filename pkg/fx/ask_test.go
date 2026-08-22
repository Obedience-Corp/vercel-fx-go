package fx

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAskCtxRejectsBadInput(t *testing.T) {
	client := mockClient(t, "ask-success")
	tests := []struct {
		name    string
		run     func() error
		wantSub string
	}{
		{
			name:    "empty prompt",
			run:     func() error { _, err := client.AskCtx(context.Background(), "   ", nil); return err },
			wantSub: "prompt must not be empty",
		},
		{
			name:    "nil stdin reader",
			run:     func() error { _, err := client.AskFromStdinCtx(context.Background(), nil, nil); return err },
			wantSub: "stdin reader must not be nil",
		},
		{
			name: "blank stdin prompt",
			run: func() error {
				_, err := client.AskFromStdinCtx(context.Background(), strings.NewReader("\n"), nil)
				return err
			},
			wantSub: "stdin prompt must not be empty",
		},
		{
			name: "invalid options are rejected before spawning",
			run: func() error {
				_, err := client.AskCtx(context.Background(), "hi", &AskOptions{Yolo: true})
				return err
			},
			wantSub: "AllowDangerousMode",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fxErr := requireFxError(t, tc.run(), KindValidation)
			if !strings.Contains(fxErr.Message, tc.wantSub) {
				t.Fatalf("message %q does not contain %q", fxErr.Message, tc.wantSub)
			}
		})
	}
}

func TestAskCtxNonZeroExitStillReturnsResult(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		wantKind Kind
		check    func(t *testing.T, result *AskResult)
	}{
		{
			name: "provider outage", scenario: "ask-503-paused", wantKind: KindProviderUnavailable,
			check: func(t *testing.T, result *AskResult) {
				if result.Recovery == nil || !result.Recovery.Paused() {
					t.Fatalf("recovery %+v, want paused", result.Recovery)
				}
				if result.SessionID == "" {
					t.Fatal("a durable recovery must carry a session id")
				}
			},
		},
		{
			name: "unknown model", scenario: "ask-model-not-found", wantKind: KindModelNotFound,
			check: func(t *testing.T, result *AskResult) {
				if !strings.Contains(result.Output, "model_not_found") {
					t.Fatalf("output %q", result.Output)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := mockClient(t, tc.scenario)
			result, err := client.AskCtx(context.Background(), "ping", nil)
			if result == nil {
				t.Fatal("the result must be returned even when fx fails")
			}
			fxErr := requireFxError(t, err, tc.wantKind)
			if fxErr.ExitCode != 1 {
				t.Fatalf("exit code %d, want 1", fxErr.ExitCode)
			}
			tc.check(t, result)
		})
	}
}

func TestAskCtxSuccess(t *testing.T) {
	client, record := recordingClient(t, "ask-tool-write")
	result, err := client.AskCtx(context.Background(), "write hello.txt", &AskOptions{
		Model: "zai/glm-5.2", NoSave: true, Quiet: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Steps != 1 || len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "write_file" {
		t.Fatalf("result %+v", result)
	}
	if len(result.Raw) == 0 {
		t.Fatal("raw JSON must be retained")
	}
	got := record()
	wantArgv := []string{"ask", "--json", "--quiet", "--no-save", "--", "write hello.txt"}
	if strings.Join(got.Argv, " ") != strings.Join(wantArgv, " ") {
		t.Fatalf("argv %v, want %v", got.Argv, wantArgv)
	}
	if got.Env["FX_MODEL"] != "zai/glm-5.2" {
		t.Fatalf("FX_MODEL %q", got.Env["FX_MODEL"])
	}
	if got.Env["FX_AUTO_UPGRADE"] != "0" || got.Env["FX_NO_OPEN_BROWSER"] != "1" {
		t.Fatalf("mandatory env missing: %v", got.Env)
	}
	wantCwd, _ := filepath.EvalSymlinks(client.WorkingDir)
	gotCwd, _ := filepath.EvalSymlinks(got.Cwd)
	if gotCwd != wantCwd {
		t.Fatalf("cwd %q, want %q", gotCwd, wantCwd)
	}
}

func TestAskCtxQuietFlagIsPassed(t *testing.T) {
	client, record := recordingClient(t, "ask-success")
	if _, err := client.AskCtx(context.Background(), "ping", &AskOptions{Quiet: true, NoColor: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(record().Argv, " "); got != "ask --json --quiet --no-color -- ping" {
		t.Fatalf("argv %q", got)
	}
}

func TestAskFromStdinCtx(t *testing.T) {
	client, record := recordingClient(t, "ask-success")
	client.Env = append(client.Env, "FX_MOCK_READ_STDIN=1")
	result, err := client.AskFromStdinCtx(context.Background(), strings.NewReader("summarize this"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "PONG" {
		t.Fatalf("output %q", result.Output)
	}
	got := record()
	if strings.Join(got.Argv, " ") != "ask --json" {
		t.Fatalf("stdin mode must not pass a prompt: %v", got.Argv)
	}
	if got.Stdin != "summarize this" {
		t.Fatalf("stdin %q", got.Stdin)
	}
}

func TestAskCtxHonorsCancellation(t *testing.T) {
	client := mockClient(t, "ask-success")
	client.Env = append(client.Env, "FX_MOCK_SLEEP_MS=10000")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := client.AskCtx(ctx, "ping", nil)
	fxErr := requireFxError(t, err, KindInterrupted)
	if !errors.Is(fxErr, context.Canceled) {
		t.Fatalf("error must wrap context.Canceled, got %v", fxErr.Original)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancellation took %v", elapsed)
	}
}

func TestAskCtxHonorsTimeout(t *testing.T) {
	client := mockClient(t, "ask-success")
	client.Env = append(client.Env, "FX_MOCK_SLEEP_MS=10000")
	_, err := client.AskCtx(context.Background(), "ping", &AskOptions{Timeout: 200 * time.Millisecond})
	fxErr := requireFxError(t, err, KindInterrupted)
	if !errors.Is(fxErr, context.DeadlineExceeded) {
		t.Fatalf("error must wrap context.DeadlineExceeded, got %v", fxErr.Original)
	}
}

func TestAskCtxContextDoneBeforeSpawn(t *testing.T) {
	client := mockClient(t, "ask-success")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.AskCtx(ctx, "ping", nil)
	requireFxError(t, err, KindInterrupted)
}

func TestAskCtxNonJSONStdout(t *testing.T) {
	client := mockClient(t, "not-json")
	_, err := client.AskCtx(context.Background(), "ping", nil)
	fxErr := requireFxError(t, err, KindValidation)
	if !strings.Contains(fxErr.Message, "not JSON") {
		t.Fatalf("message %q", fxErr.Message)
	}
}

func TestAskCtxEmptyStdout(t *testing.T) {
	client := mockClient(t, "empty")
	_, err := client.AskCtx(context.Background(), "ping", nil)
	fxErr := requireFxError(t, err, KindValidation)
	if !strings.Contains(fxErr.Message, "no output") {
		t.Fatalf("message %q", fxErr.Message)
	}
}

func TestAskCtxMissingBinaryIsTransport(t *testing.T) {
	client := NewClient("/nonexistent/fx-binary")
	client.WorkingDir = t.TempDir()
	_, err := client.AskCtx(context.Background(), "ping", nil)
	requireFxError(t, err, KindTransport)
}

func TestAskRetriesRetryableFailures(t *testing.T) {
	client := mockClient(t, "ask-503-paused")
	policy := &RetryPolicy{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond, Multiplier: 2}
	result, err := client.AskCtx(context.Background(), "ping", &AskOptions{RetryPolicy: policy})
	requireFxError(t, err, KindProviderUnavailable)
	if result == nil {
		t.Fatal("the last result must survive the retry loop")
	}
}

func TestAskDoesNotRetryValidationFailures(t *testing.T) {
	client := mockClient(t, "ask-model-not-found")
	policy := &RetryPolicy{MaxAttempts: 3, InitialDelay: time.Hour}
	start := time.Now()
	_, err := client.AskCtx(context.Background(), "ping", &AskOptions{RetryPolicy: policy})
	requireFxError(t, err, KindModelNotFound)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("a non-retryable failure slept for %v", elapsed)
	}
}

func TestExecCommandSeamIsUsed(t *testing.T) {
	original := execCommand
	t.Cleanup(func() { execCommand = original })
	called := false
	execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		called = true
		return original(ctx, name, args...)
	}
	client := mockClient(t, "ask-success")
	if _, err := client.AskCtx(context.Background(), "ping", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("the execCommand seam was bypassed")
	}
}
