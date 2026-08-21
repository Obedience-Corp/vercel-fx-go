// Command acp_stream runs one ACP turn and streams the updates as they arrive.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Obedience-Corp/vercel-fx-go/pkg/fx"
)

func main() {
	prompt := strings.Join(os.Args[1:], " ")
	if prompt == "" {
		prompt = "Reply with exactly the word PONG and nothing else."
	}
	client, err := fx.NewClientFromPath()
	if err != nil {
		exit(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		exit(err)
	}
	client.WorkingDir = cwd

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	session, err := client.StartACP(ctx, &fx.ACPConfig{
		Model:             os.Getenv("FX_MODEL"),
		PermissionHandler: promptOnStderr,
	})
	if err != nil {
		exit(err)
	}
	defer session.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		stream(session)
	}()

	runTurn(ctx, session, cwd, prompt)
	if err := session.Close(); err != nil {
		exit(err)
	}
	wg.Wait()
}

func runTurn(ctx context.Context, session *fx.ACPSession, cwd, prompt string) {
	if _, err := session.Initialize(ctx, fx.ClientCapabilities{}, nil); err != nil {
		exit(err)
	}
	created, err := session.NewSession(ctx, cwd, nil)
	if err != nil {
		exit(err)
	}
	result, err := session.PromptText(ctx, created.SessionID, prompt)
	if err != nil {
		exit(err)
	}
	fmt.Fprintf(os.Stderr, "\nstop reason: %s\n", result.StopReason)
}

func stream(session *fx.ACPSession) {
	for update := range session.Updates() {
		switch update.Update.Kind {
		case fx.UpdateAgentMessageChunk:
			fmt.Print(update.Update.Text())
		case fx.UpdateAgentThoughtChunk:
			fmt.Fprint(os.Stderr, update.Update.Text())
		case fx.UpdateToolCall:
			fmt.Fprintf(os.Stderr, "\n[tool %s %s]\n", update.Update.ToolKind, update.Update.Title)
		case fx.UpdateToolCallUpdate:
			fmt.Fprintf(os.Stderr, "\n[tool %s] %s\n", update.Update.Status, update.Update.ToolText())
		case fx.UpdateSessionInfo:
			if recovery := update.Update.Recovery; recovery != nil {
				fmt.Fprintf(os.Stderr, "\n[recovery %s attempt %d/%d]\n", recovery.State, recovery.Attempt, recovery.AttemptLimit)
			}
		}
	}
}

// promptOnStderr rejects every request. A real host would ask its user.
func promptOnStderr(_ context.Context, req *fx.PermissionRequest) (fx.PermissionOutcome, error) {
	title := "a tool"
	if req.ToolCall != nil {
		title = req.ToolCall.Title
	}
	fmt.Fprintf(os.Stderr, "\n[permission requested for %s, rejecting]\n", title)
	return fx.DefaultPermissionHandler(context.Background(), req)
}

func exit(err error) {
	fmt.Fprintln(os.Stderr, "fx failed:", err)
	os.Exit(1)
}
