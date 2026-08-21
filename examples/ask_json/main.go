// Command ask_json prints the full fx ask result as JSON, including the tool
// calls and the fx recovery state.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Obedience-Corp/vercel-fx-go/pkg/fx"
)

func main() {
	prompt := strings.Join(os.Args[1:], " ")
	if prompt == "" {
		prompt = "List the files in this directory and summarize them in one line."
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, askErr := client.AskCtx(ctx, prompt, &fx.AskOptions{
		Model:  os.Getenv("FX_MODEL"),
		NoSave: true,
	})
	if result != nil {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			exit(err)
		}
	}
	if askErr != nil {
		fmt.Fprintln(os.Stderr, "fx failed:", askErr)
		os.Exit(1)
	}
}

func exit(err error) {
	fmt.Fprintln(os.Stderr, "fx failed:", err)
	os.Exit(1)
}
