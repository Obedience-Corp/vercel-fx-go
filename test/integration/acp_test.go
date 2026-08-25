//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/vercel-fx-go/pkg/fx"
	"github.com/Obedience-Corp/vercel-fx-go/pkg/fx/dangerous"
)

func TestACPTurn(t *testing.T) {
	enableDangerous(t)
	client := newClient(t, "full-turn")
	guarded, err := dangerous.Wrap(client)
	if err != nil {
		t.Fatal(err)
	}
	timeout := 60 * time.Second
	prompt := "write acp_yolo.txt"
	if realLane() {
		timeout = 12 * time.Minute
		prompt = "Reply with exactly the word PONG and nothing else. Do not use any tools."
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	session, err := guarded.StartACP(ctx, &fx.ACPConfig{Model: modelForLane()})
	if err != nil {
		t.Fatalf("start acp: %v", err)
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			t.Errorf("close acp: %v", closeErr)
		}
	}()

	init, err := session.Initialize(ctx, fx.ClientCapabilities{}, nil)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if init.ProtocolVersion != fx.ACPProtocolVersion {
		t.Fatalf("protocol version %d, want %d", init.ProtocolVersion, fx.ACPProtocolVersion)
	}
	t.Logf("acp agent: %s %s, loadSession=%v", init.AgentInfo.Name, init.AgentInfo.Version, init.AgentCapabilities.LoadSession)

	created, err := session.NewSession(ctx, client.WorkingDir, nil)
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	if created.SessionID == "" {
		t.Fatal("session/new returned no session id")
	}

	collected, err := session.CollectPrompt(ctx, created.SessionID, []fx.PromptBlock{fx.TextBlock(prompt)})
	if collected != nil {
		logRecovery(t, collected.Recovery)
	}
	skipIfProviderDown(t, err)
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if collected.StopReason == fx.StopRefused {
		t.Skipf("fx refused the turn after exhausting recovery: %+v", collected.Recovery)
	}
	if collected.StopReason != fx.StopEndTurn {
		t.Fatalf("stop reason %q", collected.StopReason)
	}
	if realLane() && !strings.Contains(strings.ToUpper(collected.Text), "PONG") {
		t.Fatalf("text %q does not contain PONG", collected.Text)
	}
	t.Logf("acp turn: stop=%s text=%q tools=%d", collected.StopReason, collected.Text, len(collected.ToolCalls))
}

func TestACPHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := newClient(t, "initialize-and-session-new")
	session, err := client.StartACP(ctx, &fx.ACPConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	initialized, err := session.Initialize(ctx, fx.ClientCapabilities{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if initialized.AgentInfo.Version != fx.TestedFXVersion {
		t.Fatalf("ACP agent version %q, want %q", initialized.AgentInfo.Version, fx.TestedFXVersion)
	}
	created, err := session.NewSession(ctx, client.WorkingDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"provider": false, "model": false, "mode": false}
	for _, option := range created.ConfigOptions {
		if _, ok := want[option.ID]; ok {
			want[option.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("session/new omitted %q config option", id)
		}
	}
	if created.Modes == nil || len(created.Modes.Available) == 0 {
		t.Error("session/new omitted ACP modes")
	}
}

func modelForLane() string {
	if realLane() {
		return realModel
	}
	return ""
}

func TestACPSessionListing(t *testing.T) {
	if realLane() {
		t.Skip("the session listing script is a mock-lane assertion")
	}
	session, err := newClient(t, "full-turn").StartACP(context.Background(), &fx.ACPConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	ctx := context.Background()
	if _, err := session.Initialize(ctx, fx.ClientCapabilities{}, nil); err != nil {
		t.Fatal(err)
	}
	created, err := session.NewSession(ctx, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.SetModel(ctx, created.SessionID, "zai/glm-5.2"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.CollectPrompt(ctx, created.SessionID, []fx.PromptBlock{fx.TextBlock("write acp_yolo.txt")}); err != nil {
		t.Fatal(err)
	}
	sessions, err := session.ListSessions(ctx)
	if err != nil {
		t.Fatalf("session/list: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatal("session/list returned nothing")
	}
	if err := session.CloseSession(ctx, sessions[0].SessionID); err != nil {
		t.Fatalf("session/close: %v", err)
	}
}
