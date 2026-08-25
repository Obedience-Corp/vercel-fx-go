package fx

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
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

func TestProviderCommands(t *testing.T) {
	client := mockClient(t, "status")
	ctx := context.Background()
	tests := []struct {
		name string
		run  func() (*exec.Cmd, error)
		want []string
	}{
		{name: "login codex", run: func() (*exec.Cmd, error) { return client.LoginProviderCommand(ctx, "codex") }, want: []string{"login", "codex"}},
		{name: "logout grok", run: func() (*exec.Cmd, error) { return client.LogoutProviderCommand(ctx, "grok") }, want: []string{"logout", "grok"}},
		{name: "select gateway", run: func() (*exec.Cmd, error) { return client.ProviderCommand(ctx, "gateway") }, want: []string{"provider", "gateway"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := tc.run()
			if err != nil {
				t.Fatal(err)
			}
			got := cmd.Args[len(cmd.Args)-2:]
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Fatalf("args %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProviderCommandsRejectUnknownProviders(t *testing.T) {
	client := mockClient(t, "status")
	if _, err := client.LoginProviderCommand(context.Background(), "gateway"); err == nil {
		t.Fatal("login accepted the ACP-only gateway provider name")
	}
	if _, err := client.ProviderCommand(context.Background(), "vercel"); err == nil {
		t.Fatal("provider selection accepted the auth-only vercel provider name")
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

func TestLoginURLContinuesDrainingAfterAuthorizationURL(t *testing.T) {
	client := mockClient(t, "status")
	client.Env = append(client.Env, "FX_MOCK_LOGIN_TAIL_BYTES=1048576")
	flow, err := client.LoginURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- flow.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
	case <-time.After(3 * time.Second):
		_ = flow.cmd.Process.Kill()
		t.Fatal("login blocked because an output pipe was not drained")
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

func TestInteractiveCommandHonorsCancellationBeforeConfiguration(t *testing.T) {
	client := mockClient(t, "status")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.LoginCommand(ctx)
	requireFxError(t, err, KindInterrupted)
}

func lastArg(cmd *exec.Cmd) string {
	if cmd == nil || len(cmd.Args) == 0 {
		return ""
	}
	return cmd.Args[len(cmd.Args)-1]
}
