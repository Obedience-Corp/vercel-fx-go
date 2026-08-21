// Command ask runs one fx request and prints the assistant output.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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
		fail(err)
	}
	client.WorkingDir = mustCwd()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := client.AskCtx(ctx, prompt, &fx.AskOptions{
		Model:  os.Getenv("FX_MODEL"),
		NoSave: true,
	})
	if result != nil && result.Recovery != nil {
		fmt.Fprintf(os.Stderr, "recovery: %s (attempt %d/%d)\n",
			result.Recovery.State, result.Recovery.Attempt, result.Recovery.AttemptLimit)
	}
	if err != nil {
		fail(err)
	}
	fmt.Println(result.Output)
}

func mustCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	return cwd
}

func fail(err error) {
	var fxErr *fx.Error
	if errors.As(err, &fxErr) && fxErr.IsRetryable() {
		fmt.Fprintf(os.Stderr, "fx failed but the call is retryable: %v\n", fxErr)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "fx failed:", err)
	os.Exit(1)
}
