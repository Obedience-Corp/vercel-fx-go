// Command usage prints the local fx token usage and the credit balance.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Obedience-Corp/vercel-fx-go/pkg/fx"
)

func main() {
	period := "30d"
	if len(os.Args) > 1 {
		period = os.Args[1]
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	report, err := client.Usage(ctx, period)
	if err != nil {
		exit(err)
	}
	fmt.Printf("usage over %s (coverage %s, %s)\n", report.Period, report.Coverage.Status, report.Completeness)
	fmt.Printf("  requests=%d in=%d out=%d cached=%d spend=%.4f\n",
		report.Totals.RequestCount, report.Totals.InputTokens,
		report.Totals.OutputTokens, report.Totals.CacheReadTokens, report.Totals.Spend)
	for _, model := range report.Models {
		fmt.Printf("  %-28s requests=%d in=%d out=%d\n",
			model.Model, model.Totals.RequestCount, model.Totals.InputTokens, model.Totals.OutputTokens)
	}

	credits, err := client.Credits(ctx)
	if err != nil {
		exit(err)
	}
	fmt.Printf("gateway balance: %s\n", credits.Balance)
}

func exit(err error) {
	fmt.Fprintln(os.Stderr, "fx failed:", err)
	os.Exit(1)
}
