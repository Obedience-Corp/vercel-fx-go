package fx

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestACPPermissionHandlerAllows(t *testing.T) {
	defer goleak.VerifyNone(t)
	replyPath := filepath.Join(t.TempDir(), "reply.json")
	var seen *PermissionRequest
	var mu sync.Mutex
	client := mockClient(t, "request-permission")
	client.Env = append(client.Env, "FX_MOCK_PERM_REPLY="+replyPath)
	session, err := client.StartACP(context.Background(), &ACPConfig{
		PermissionHandler: func(_ context.Context, req *PermissionRequest) (PermissionOutcome, error) {
			mu.Lock()
			seen = req
			mu.Unlock()
			option, ok := req.OptionByKind(PermissionAllowOnce)
			if !ok {
				return PermissionOutcome{Outcome: OutcomeCancelled}, nil
			}
			return PermissionOutcome{Outcome: OutcomeSelected, OptionID: option.OptionID}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	ctx := context.Background()
	if _, err := session.Initialize(ctx, ClientCapabilities{}, nil); err != nil {
		t.Fatal(err)
	}
	created, err := session.NewSession(ctx, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	collected, err := session.CollectPrompt(ctx, created.SessionID, []PromptBlock{TextBlock("write perm.txt")})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if collected.StopReason != StopEndTurn {
		t.Fatalf("stop reason %q", collected.StopReason)
	}
	if collected.Text != "DONE" {
		t.Fatalf("text %q", collected.Text)
	}
	if len(collected.ToolCalls) != 1 || collected.ToolCalls[0].Status != "completed" {
		t.Fatalf("tool calls %+v", collected.ToolCalls)
	}

	mu.Lock()
	request := seen
	mu.Unlock()
	if request == nil {
		t.Fatal("the permission handler was never called")
	}
	if request.ToolCall == nil || request.ToolCall.Kind != "edit" || request.ToolCall.Title != "Writing" {
		t.Fatalf("tool call %+v", request.ToolCall)
	}
	if string(request.ToolCall.RawInput) != `{"path":"perm.txt","content":"hi"}` {
		t.Fatalf("raw input %s", request.ToolCall.RawInput)
	}
	if len(request.Options) != 3 {
		t.Fatalf("options %+v", request.Options)
	}
	if len(request.Raw) == 0 {
		t.Fatal("the raw request must be retained")
	}
	assertPermissionReply(t, replyPath, "selected", "allow_once")
}

func TestACPDefaultPermissionHandlerRejects(t *testing.T) {
	defer goleak.VerifyNone(t)
	replyPath := filepath.Join(t.TempDir(), "reply.json")
	client := mockClient(t, "request-permission")
	client.Env = append(client.Env, "FX_MOCK_PERM_REPLY="+replyPath)
	session, err := client.StartACP(context.Background(), &ACPConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	ctx := context.Background()
	if _, err := session.Initialize(ctx, ClientCapabilities{}, nil); err != nil {
		t.Fatal(err)
	}
	created, err := session.NewSession(ctx, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.CollectPrompt(ctx, created.SessionID, []PromptBlock{TextBlock("write perm.txt")}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	assertPermissionReply(t, replyPath, "selected", "reject_once")
}

func TestACPCloseDoesNotWaitForBlockedPermissionHandler(t *testing.T) {
	defer goleak.VerifyNone(t)
	started := make(chan struct{})
	release := make(chan struct{})
	client := mockClient(t, "request-permission")
	client.Env = append(client.Env, "FX_MOCK_PERM_TIMEOUT_MS=100")
	session, err := client.StartACP(context.Background(), &ACPConfig{
		PermissionHandler: func(_ context.Context, _ *PermissionRequest) (PermissionOutcome, error) {
			close(started)
			<-release
			return PermissionOutcome{Outcome: OutcomeCancelled}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := session.Initialize(ctx, ClientCapabilities{}, nil); err != nil {
		t.Fatal(err)
	}
	created, err := session.NewSession(ctx, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	promptDone := make(chan error, 1)
	go func() {
		_, promptErr := session.CollectPrompt(ctx, created.SessionID, []PromptBlock{TextBlock("write perm.txt")})
		promptDone <- promptErr
	}()
	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- session.Close() }()
	select {
	case closeErr := <-closeDone:
		if closeErr != nil {
			t.Fatalf("close: %v", closeErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close waited for a permission handler that ignored cancellation")
	}
	close(release)
	select {
	case <-promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not return after Close")
	}
}

func assertPermissionReply(t *testing.T, path, wantOutcome, wantOption string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the mock recorded no permission reply: %v", err)
	}
	var reply struct {
		Result PermissionResponse `json:"result"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		t.Fatalf("decode reply %s: %v", data, err)
	}
	if reply.Result.Outcome.Outcome != wantOutcome || reply.Result.Outcome.OptionID != wantOption {
		t.Fatalf("reply %+v, want %s/%s", reply.Result.Outcome, wantOutcome, wantOption)
	}
}

func TestDefaultPermissionHandlerFallsBackToCancelled(t *testing.T) {
	tests := []struct {
		name        string
		req         *PermissionRequest
		wantOutcome string
		wantOption  string
	}{
		{
			name:        "reject once offered",
			req:         &PermissionRequest{Options: []PermissionOption{{OptionID: "r1", Kind: PermissionRejectOnce}}},
			wantOutcome: OutcomeSelected, wantOption: "r1",
		},
		{
			name:        "only reject always offered",
			req:         &PermissionRequest{Options: []PermissionOption{{OptionID: "ra", Kind: PermissionRejectAlways}}},
			wantOutcome: OutcomeSelected, wantOption: "ra",
		},
		{
			name:        "no options at all",
			req:         &PermissionRequest{},
			wantOutcome: OutcomeCancelled,
		},
		{
			name:        "only allow options offered",
			req:         &PermissionRequest{Options: []PermissionOption{{OptionID: "a1", Kind: PermissionAllowOnce}}},
			wantOutcome: OutcomeCancelled,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outcome, err := DefaultPermissionHandler(context.Background(), tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Outcome != tc.wantOutcome || outcome.OptionID != tc.wantOption {
				t.Fatalf("outcome %+v, want %s/%s", outcome, tc.wantOutcome, tc.wantOption)
			}
		})
	}
}

func TestACPUnknownServerRequestIsRefused(t *testing.T) {
	defer goleak.VerifyNone(t)
	replyPath := filepath.Join(t.TempDir(), "reply.json")
	client := mockClient(t, "unknown-request")
	client.Env = append(client.Env, "FX_MOCK_PERM_REPLY="+replyPath)
	session, err := client.StartACP(context.Background(), &ACPConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	ctx := context.Background()
	if _, err := session.Initialize(ctx, ClientCapabilities{}, nil); err != nil {
		t.Fatal(err)
	}
	created, err := session.NewSession(ctx, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.CollectPrompt(ctx, created.SessionID, []PromptBlock{TextBlock("read a file")}); err != nil {
		t.Fatalf("collect: %v", err)
	}
	data, readErr := os.ReadFile(replyPath)
	if readErr != nil {
		t.Fatalf("the SDK never answered the unadvertised request: %v", readErr)
	}
	var reply struct {
		Error *RPCError `json:"error"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		t.Fatalf("decode reply %s: %v", data, err)
	}
	if reply.Error == nil || reply.Error.Code != -32601 {
		t.Fatalf("reply %s, want a method-not-found error", data)
	}
	if !strings.Contains(reply.Error.Message, "fs/read_text_file") {
		t.Fatalf("message %q", reply.Error.Message)
	}
}
