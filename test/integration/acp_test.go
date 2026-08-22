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
