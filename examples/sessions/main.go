// Command sessions lists the saved fx sessions for the current workspace.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Obedience-Corp/vercel-fx-go/pkg/fx"
)

func main() {
	client, err := fx.NewClientFromPath()
	if err != nil {
		exit(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		exit(err)
	}
	client.WorkingDir = cwd

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	list, err := client.Sessions(ctx, &fx.SessionsOptions{Limit: 10})
	if err != nil {
		exit(err)
	}
	fmt.Printf("%d saved sessions in %s\n", list.Count, cwd)
	for _, session := range list.Sessions {
		fmt.Printf("  %s  %s  (%d turns)\n", session.ID, session.Title, session.HistoryLen)
		usage, usageErr := fx.SessionUsage(ctx, session.ID)
		if usageErr != nil {
			continue
		}
		fmt.Printf("      tokens in=%d out=%d cost=%.4f billing=%s\n",
			usage.InputTokens, usage.OutputTokens, usage.TotalCost, usage.Billing)
	}
}

func exit(err error) {
	fmt.Fprintln(os.Stderr, "fx failed:", err)
	os.Exit(1)
}
