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

func TestAskRoundTrip(t *testing.T) {
	enableDangerous(t)
	client := newClient(t, "ask-success")
	guarded, err := dangerous.Wrap(client)
	if err != nil {
		t.Fatal(err)
	}
	timeout := 45 * time.Second
	if realLane() {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, askErr := guarded.Yolo(ctx, "Reply with exactly the word PONG and nothing else.", &fx.AskOptions{
		Model:   realModel,
		NoSave:  true,
		NoColor: true,
	})
	if result != nil {
		logRecovery(t, result.Recovery)
	}
	skipIfProviderDown(t, askErr)
	if askErr != nil {
		t.Fatalf("ask failed: %v", askErr)
	}
	if result.Model != realModel {
		t.Fatalf("model %q, want %q", result.Model, realModel)
	}
	if result.SessionID != "" {
		t.Fatalf("--no-save must leave the session id empty, got %q", result.SessionID)
	}
	if !strings.Contains(strings.ToUpper(result.Output), "PONG") {
		t.Fatalf("output %q does not contain PONG", result.Output)
	}
}

func TestAskFailureStillReturnsTheResult(t *testing.T) {
	if realLane() {
		t.Skip("the failure fixture is a mock-lane assertion")
	}
	client := newClient(t, "ask-503-paused")
	result, err := client.AskCtx(context.Background(), "ping", nil)
	if result == nil {
		t.Fatal("the result must survive a non-zero exit")
	}
	var fxErr *fx.Error
	if !asFxError(err, &fxErr) {
		t.Fatalf("err %T %v", err, err)
	}
	if fxErr.Kind != fx.KindProviderUnavailable || !fxErr.IsRetryable() {
		t.Fatalf("error %+v", fxErr)
	}
	if result.Recovery == nil || !result.Recovery.Paused() {
		t.Fatalf("recovery %+v", result.Recovery)
	}
}
