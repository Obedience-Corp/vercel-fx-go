package fx

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestInteractiveCommandsAreConfiguredNotRun(t *testing.T) {
	client := mockClient(t, "status")
	ctx := context.Background()
	tests := []struct {
		name string
		run  func() (string, error)
		verb string
	}{
		{name: "login", verb: "login", run: func() (string, error) {
			cmd, err := client.LoginCommand(ctx)
			return lastArg(cmd), err
		}},
		{name: "setup", verb: "setup", run: func() (string, error) {
			cmd, err := client.SetupCommand(ctx)
			return lastArg(cmd), err
		}},
		{name: "teams", verb: "teams", run: func() (string, error) {
			cmd, err := client.TeamsCommand(ctx)
			return lastArg(cmd), err
		}},
		{name: "logout", verb: "logout", run: func() (string, error) {
			cmd, err := client.LogoutCommand(ctx)
			return lastArg(cmd), err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.run()
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.verb {
				t.Fatalf("verb %q, want %q", got, tc.verb)
			}
		})
	}
}

func TestInteractiveCommandDisablesBrowserAndUpgrade(t *testing.T) {
	client := mockClient(t, "status")
	cmd, err := client.LoginCommand(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	env := strings.Join(cmd.Env, " ")
	if !strings.Contains(env, "FX_NO_OPEN_BROWSER=1") || !strings.Contains(env, "FX_AUTO_UPGRADE=0") {
		t.Fatal("interactive commands must disable the browser and the upgrade check")
	}
	if cmd.Dir != client.WorkingDir {
		t.Fatalf("dir %q, want %q", cmd.Dir, client.WorkingDir)
	}
}

func TestLoginURLCapturesTheAuthorizationURL(t *testing.T) {
	client := mockClient(t, "status")
	flow, err := client.LoginURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer flow.Close()
	if flow.URL != "https://vercel.com/oauth/device?user_code=MOCK-CODE" {
		t.Fatalf("url %q", flow.URL)
	}
	if err := flow.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestLoginURLHonorsCancellation(t *testing.T) {
	client := mockClient(t, "status")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.LoginURL(ctx)
	if err == nil {
		t.Fatal("expected an error for a canceled context")
	}
}

func lastArg(cmd *exec.Cmd) string {
	if cmd == nil || len(cmd.Args) == 0 {
		return ""
	}
	return cmd.Args[len(cmd.Args)-1]
}
