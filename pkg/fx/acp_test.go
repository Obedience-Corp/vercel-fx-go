package fx

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

const scriptSessionID = "1787339130226-1787339130226384000-5a0f14d5f9458ab7"

func startMockACP(t *testing.T, scenario string, cfg *ACPConfig) *ACPSession {
	t.Helper()
	client := mockClient(t, scenario)
	if cfg == nil {
		cfg = &ACPConfig{}
	}
	session, err := client.StartACP(context.Background(), cfg)
	if err != nil {
		t.Fatalf("start acp: %v", err)
	}
	return session
}

func drainUpdates(session *ACPSession) (*sync.WaitGroup, *[]SessionUpdate) {
	var wg sync.WaitGroup
	collected := &[]SessionUpdate{}
	var mu sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		for update := range session.Updates() {
			mu.Lock()
			*collected = append(*collected, update)
			mu.Unlock()
		}
	}()
	return &wg, collected
}

func TestACPStartRejectsBadConfig(t *testing.T) {
	defer goleak.VerifyNone(t)
	client := mockClient(t, "full-turn")
	tests := []struct {
		name    string
		cfg     *ACPConfig
		wantSub string
	}{
		{name: "yolo without acknowledgement", cfg: &ACPConfig{PermissionMode: PermissionYolo}, wantSub: "AllowDangerousMode"},
		{name: "relative log file", cfg: &ACPConfig{LogFile: "relative.log"}, wantSub: "absolute path"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.StartACP(context.Background(), tc.cfg)
			fxErr := requireFxError(t, err, KindValidation)
			if !strings.Contains(fxErr.Message, tc.wantSub) {
				t.Fatalf("message %q does not contain %q", fxErr.Message, tc.wantSub)
			}
		})
	}
}

func TestACPStartHonorsCanceledContext(t *testing.T) {
	defer goleak.VerifyNone(t)
	client := mockClient(t, "full-turn")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.StartACP(ctx, &ACPConfig{})
	requireFxError(t, err, KindInterrupted)
}

func TestACPStartMissingBinary(t *testing.T) {
	defer goleak.VerifyNone(t)
	client := NewClient("/nonexistent/fx-binary")
	client.WorkingDir = t.TempDir()
	_, err := client.StartACP(context.Background(), &ACPConfig{})
	requireFxError(t, err, KindTransport)
}

func TestACPPromptRejectsBadArguments(t *testing.T) {
	defer goleak.VerifyNone(t)
	session := startMockACP(t, "full-turn", nil)
	defer session.Close()
	ctx := context.Background()
	tests := []struct {
		name    string
		run     func() error
		wantSub string
	}{
		{name: "empty session id", run: func() error { _, err := session.Prompt(ctx, "", []PromptBlock{TextBlock("hi")}); return err }, wantSub: "session id"},
		{name: "no blocks", run: func() error { _, err := session.Prompt(ctx, "s", nil); return err }, wantSub: "at least one block"},
		{name: "blank text", run: func() error { _, err := session.PromptText(ctx, "s", " "); return err }, wantSub: "must not be empty"},
		{name: "bad mode", run: func() error { return session.SetMode(ctx, "s", "wild") }, wantSub: "must be \"ask\" or \"code\""},
		{name: "empty cwd", run: func() error { _, err := session.NewSession(ctx, "", nil); return err }, wantSub: "cwd must not be empty"},
		{name: "empty close id", run: func() error { return session.CloseSession(ctx, "") }, wantSub: "session id"},
		{name: "empty model", run: func() error { _, err := session.SetModel(ctx, "s", ""); return err }, wantSub: "must not be empty"},
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

func TestACPFullConversation(t *testing.T) {
	defer goleak.VerifyNone(t)
	session := startMockACP(t, "full-turn", nil)
	wg, collected := drainUpdates(session)
	ctx := context.Background()

	init, err := session.Initialize(ctx, ClientCapabilities{}, nil)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if init.ProtocolVersion != ACPProtocolVersion {
		t.Fatalf("protocol version %d", init.ProtocolVersion)
	}
	if init.AgentInfo.Name != "fx" || init.AgentInfo.Version != "0.0.4" {
		t.Fatalf("agent info %+v", init.AgentInfo)
	}
	if !init.AgentCapabilities.LoadSession || !init.AgentCapabilities.PromptCapabilities.EmbeddedContext {
		t.Fatalf("capabilities %+v", init.AgentCapabilities)
	}
	if init.AgentCapabilities.PromptCapabilities.Image {
		t.Fatal("fx v0.0.4 rejects image prompt blocks")
	}
	if init.AgentCapabilities.SessionCapabilities.List == nil {
		t.Fatal("session list capability must be advertised")
	}

	created, err := session.NewSession(ctx, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	if created.SessionID != scriptSessionID {
		t.Fatalf("session id %q", created.SessionID)
	}
	if len(created.ConfigOptions) == 0 || created.ConfigOptions[0].ID != "model" {
		t.Fatalf("config options %+v", created.ConfigOptions)
	}
	if created.ConfigOptions[0].CurrentValue != "zai/glm-5.2" {
		t.Fatalf("current model %q", created.ConfigOptions[0].CurrentValue)
	}

	options, err := session.SetModel(ctx, created.SessionID, "zai/glm-5.2")
	if err != nil {
		t.Fatalf("set_config_option: %v", err)
	}
	if len(options.ConfigOptions) == 0 {
		t.Fatal("set_config_option must echo the options")
	}

	result, err := session.PromptText(ctx, created.SessionID, "write acp_yolo.txt")
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if result.StopReason != StopEndTurn {
		t.Fatalf("stop reason %q", result.StopReason)
	}

	sessions, err := session.ListSessions(ctx)
	if err != nil {
		t.Fatalf("session/list: %v", err)
	}
	if len(sessions) != 1 || sessions[0].UpdatedAt == "" {
		t.Fatalf("sessions %+v", sessions)
	}
	if err := session.CloseSession(ctx, sessions[0].SessionID); err != nil {
		t.Fatalf("session/close: %v", err)
	}
	if session.PID() == 0 {
		t.Fatal("PID must report the acp process")
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	wg.Wait()

	kinds := map[SessionUpdateKind]int{}
	for _, update := range *collected {
		kinds[update.Update.Kind]++
	}
	if kinds[UpdateAvailableCommands] == 0 || kinds[UpdateToolCall] == 0 || kinds[UpdateToolCallUpdate] < 2 {
		t.Fatalf("update kinds %v", kinds)
	}
	if kinds[UpdateSessionInfo] == 0 {
		t.Fatal("the recovery update must reach the caller")
	}
}

func TestACPCloseIsIdempotent(t *testing.T) {
	defer goleak.VerifyNone(t)
	session := startMockACP(t, "full-turn", nil)
	if err := session.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	select {
	case <-session.Done():
	default:
		t.Fatal("Done must be closed after Close")
	}
}

func TestACPCollectPrompt(t *testing.T) {
	defer goleak.VerifyNone(t)
	session := startMockACP(t, "full-turn", nil)
	defer session.Close()
	ctx := context.Background()
	if _, err := session.Initialize(ctx, ClientCapabilities{}, nil); err != nil {
		t.Fatal(err)
	}
	created, err := session.NewSession(ctx, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.SetModel(ctx, created.SessionID, "zai/glm-5.2"); err != nil {
		t.Fatal(err)
	}
	collected, err := session.CollectPrompt(ctx, created.SessionID, []PromptBlock{TextBlock("write acp_yolo.txt")})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if collected.StopReason != StopEndTurn {
		t.Fatalf("stop reason %q", collected.StopReason)
	}
	if len(collected.ToolCalls) != 1 {
		t.Fatalf("tool calls %+v", collected.ToolCalls)
	}
	call := collected.ToolCalls[0]
	if call.Title != "Writing" || call.Kind != "edit" || call.Status != "completed" {
		t.Fatalf("tool call %+v", call)
	}
	if !strings.Contains(call.Text, "wrote acp_yolo.txt") {
		t.Fatalf("tool text %q", call.Text)
	}
	if collected.Recovery == nil || collected.Recovery.Attempt != 6 {
		t.Fatalf("recovery %+v", collected.Recovery)
	}
}

func TestACPRefusedTurnCarriesRecovery(t *testing.T) {
	defer goleak.VerifyNone(t)
	session := startMockACP(t, "refused-503", nil)
	defer session.Close()
	ctx := context.Background()
	if _, err := session.Initialize(ctx, ClientCapabilities{}, nil); err != nil {
		t.Fatal(err)
	}
	created, err := session.NewSession(ctx, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	collected, err := session.CollectPrompt(ctx, created.SessionID, []PromptBlock{TextBlock("write acp_ask.txt")})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if collected.StopReason != StopRefused {
		t.Fatalf("stop reason %q, want refused", collected.StopReason)
	}
	if collected.Recovery == nil || !collected.Recovery.Paused() {
		t.Fatalf("recovery %+v, want paused", collected.Recovery)
	}
	if collected.Recovery.Cause != "provider_unavailable" || collected.Recovery.RequiredAction != "continue_later" {
		t.Fatalf("recovery %+v", collected.Recovery)
	}
}

func TestACPPromptCancellationStopsTheProcess(t *testing.T) {
	defer goleak.VerifyNone(t)
	client := mockClient(t, "cancel")
	client.Env = append(client.Env, "FX_MOCK_ACP_DELAY_MS=300")
	session, err := client.StartACP(context.Background(), &ACPConfig{})
	if err != nil {
		t.Fatal(err)
	}
	backgroundCtx := context.Background()
	if _, err := session.Initialize(backgroundCtx, ClientCapabilities{}, nil); err != nil {
		t.Fatal(err)
	}
	created, err := session.NewSession(backgroundCtx, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	wg, _ := drainUpdates(session)

	pid := session.PID()
	ctx, cancel := context.WithTimeout(backgroundCtx, 400*time.Millisecond)
	defer cancel()
	_, err = session.Prompt(ctx, created.SessionID, []PromptBlock{TextBlock("take your time")})
	fxErr := requireFxError(t, err, KindInterrupted)
	if fxErr.Original == nil {
		t.Fatal("the cancellation must wrap the context error")
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	wg.Wait()
	if processAlive(pid) {
		t.Fatalf("the acp process %d survived Close", pid)
	}
}

func TestSessionUpdateDecodesBothContentShapes(t *testing.T) {
	tests := []struct {
		name     string
		blob     string
		wantText string
		wantTool string
	}{
		{
			name:     "message chunk uses a single content object",
			blob:     `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hello"}}`,
			wantText: "hello",
		},
		{
			name:     "tool call update uses a content array",
			blob:     `{"sessionUpdate":"tool_call_update","toolCallId":"c1","status":"completed","content":[{"type":"content","content":{"type":"text","text":"wrote a.txt"}}]}`,
			wantTool: "wrote a.txt",
		},
		{
			name: "unknown kind passes through with the raw payload",
			blob: `{"sessionUpdate":"something_new","field":1}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var update SessionUpdateInner
			if err := json.Unmarshal([]byte(tc.blob), &update); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if update.Text() != tc.wantText {
				t.Fatalf("text %q, want %q", update.Text(), tc.wantText)
			}
			if update.ToolText() != tc.wantTool {
				t.Fatalf("tool text %q, want %q", update.ToolText(), tc.wantTool)
			}
			if len(update.Raw) == 0 {
				t.Fatal("the raw update must be retained")
			}
		})
	}
}

func TestPromptBlockBuilders(t *testing.T) {
	tests := []struct {
		name  string
		block PromptBlock
		want  string
	}{
		{name: "text", block: TextBlock("hi"), want: `{"type":"text","text":"hi"}`},
		{name: "resource", block: ResourceBlock("file:///a", "body"), want: `{"type":"resource","resource":{"uri":"file:///a","text":"body"}}`},
		{name: "resource link", block: ResourceLinkBlock("file:///a", "a"), want: `{"type":"resource_link","uri":"file:///a","name":"a"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.block)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
