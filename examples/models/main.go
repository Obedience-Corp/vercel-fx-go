// Command models prints the fx status and the reachable model catalog.
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

	status, err := client.Status(ctx)
	if err != nil {
		exit(err)
	}
	fmt.Printf("model=%s source=%s auth=%s team=%s permission_mode=%s\n",
		status.Model, status.ModelSource, status.Auth, status.Team, status.PermissionMode)

	catalog, err := client.Models(ctx)
	if err != nil {
		exit(err)
	}
	fmt.Printf("%d models reachable\n", catalog.Count)
	for i, id := range catalog.IDs {
		if i == 20 {
			fmt.Printf("  ... and %d more\n", catalog.Count-20)
			break
		}
		fmt.Println("  " + id)
	}
}

func exit(err error) {
	fmt.Fprintln(os.Stderr, "fx failed:", err)
	os.Exit(1)
}
