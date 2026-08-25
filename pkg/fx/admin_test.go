package fx

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAdminErrorEnvelope(t *testing.T) {
	client := mockClient(t, "session-missing")
	_, err := client.Session(context.Background(), "does-not-exist")
	fxErr := requireFxError(t, err, KindValidation)
	if !strings.Contains(fxErr.Message, "no saved sessions") {
		t.Fatalf("message %q", fxErr.Message)
	}
	if fxErr.ExitCode != 1 {
		t.Fatalf("exit code %d, want 1", fxErr.ExitCode)
	}
}

func TestAdminRejectsBadArguments(t *testing.T) {
	client := mockClient(t, "status")
	ctx := context.Background()
	tests := []struct {
		name    string
		run     func() error
		wantSub string
	}{
		{name: "bad usage period", run: func() error { _, err := client.Usage(ctx, "1y"); return err }, wantSub: "usage period"},
		{name: "empty session id", run: func() error { _, err := client.Session(ctx, ""); return err }, wantSub: "session id"},
		{name: "session limit too high", run: func() error {
			_, err := client.Sessions(ctx, &SessionsOptions{Limit: 500})
			return err
		}, wantSub: "between 1 and 100"},
		{name: "workspace add without a path", run: func() error { _, err := client.WorkspaceAdd(ctx, ""); return err }, wantSub: "directory path"},
		{name: "empty background id", run: func() error { _, err := client.BackgroundRecord(ctx, ""); return err }, wantSub: "must not be empty"},
		{name: "empty migrate id", run: func() error { _, err := client.SessionMigrate(ctx, "", false); return err }, wantSub: "session id"},
		{name: "empty recover id", run: func() error { _, err := client.SessionRecover(ctx, ""); return err }, wantSub: "session id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fxErr := requireFxError(t, tc.run(), KindValidation)
			if !strings.Contains(fxErr.Message, tc.wantSub) {
				t.Fatalf("message %q does not contain %q", fxErr.Message, tc.wantSub)
			}
		})
	}
}

func TestAdminWrappersDecodeFixtures(t *testing.T) {
	ctx := context.Background()
	t.Run("status", func(t *testing.T) {
		got, err := mockClient(t, "status").Status(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Model != "gpt-5.4" || got.Auth != "ChatGPT subscription" || got.Team != "example-team" {
			t.Fatalf("status %+v", got)
		}
		if got.ModelSource != "Codex" || len(got.ConnectedProviders) != 2 {
			t.Fatalf("provider status %+v", got)
		}
		if !got.AuthRefreshable || got.AuthExpired {
			t.Fatalf("auth flags %+v", got)
		}
	})
	t.Run("doctor", func(t *testing.T) {
		got, err := mockClient(t, "doctor").Doctor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.OKCount != 7 || got.WarnCount != 1 || got.FailCount != 0 {
			t.Fatalf("counts %+v", got)
		}
		if !got.Healthy() {
			t.Fatal("a report with no failures must be healthy")
		}
		if len(got.Checks) != 8 || got.Checks[0].Name != "workspace" {
			t.Fatalf("checks %+v", got.Checks)
		}
	})
	t.Run("models", func(t *testing.T) {
		got, err := mockClient(t, "models").Models(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Count != 2 || len(got.IDs) != 2 {
			t.Fatalf("count %d ids %d", got.Count, len(got.IDs))
		}
		if len(got.Models) != 2 || got.Models[0].Source != "Codex" {
			t.Fatalf("models %+v", got.Models)
		}
		if got.PrivateModelsHidden {
			t.Fatal("fixture reports private models are not hidden")
		}
	})
	t.Run("permissions", func(t *testing.T) {
		got, err := mockClient(t, "permissions").Permissions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Mode != "auto" || got.GrantScope != "session" {
			t.Fatalf("permissions %+v", got)
		}
	})
	t.Run("credits keeps the balance a string", func(t *testing.T) {
		got, err := mockClient(t, "credits").Credits(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Balance != "5" {
			t.Fatalf("balance %q, want \"5\"", got.Balance)
		}
		if got.Used != nil || got.Plan != nil {
			t.Fatalf("null fields must decode to nil: used=%s plan=%s", got.Used, got.Plan)
		}
	})
	t.Run("usage", func(t *testing.T) {
		got, err := mockClient(t, "usage").Usage(ctx, "30d")
		if err != nil {
			t.Fatal(err)
		}
		if got.Period != "30d" || got.Totals.TotalTokens != 2511901 || got.Totals.RequestCount != 41 {
			t.Fatalf("usage %+v", got.Totals)
		}
		if len(got.Models) != 1 || got.Models[0].Model != "zai/glm-5.2" {
			t.Fatalf("models %+v", got.Models)
		}
		if got.Coverage.Status != "partial" || got.Coverage.FullWindow {
			t.Fatalf("coverage %+v", got.Coverage)
		}
	})
	t.Run("sessions", func(t *testing.T) {
		got, err := mockClient(t, "sessions").Sessions(ctx, &SessionsOptions{All: true, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if got.Count != 4 || len(got.Sessions) != 4 {
			t.Fatalf("sessions %+v", got)
		}
		if got.Sessions[1].Preview != nil {
			t.Fatal("a null preview must decode to nil")
		}
	})
	t.Run("session detail", func(t *testing.T) {
		got, err := mockClient(t, "session-detail").Session(ctx, "last")
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != "1700000000000-1700000000000000000-0000000000000001" {
			t.Fatalf("id %q", got.ID)
		}
	})
	t.Run("background", func(t *testing.T) {
		got, err := mockClient(t, "background").Background(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Count != 0 || len(got.Records) != 0 {
			t.Fatalf("background %+v", got)
		}
	})
	t.Run("workspace", func(t *testing.T) {
		got, err := mockClient(t, "workspace").WorkspaceList(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Action != "list" || got.Limit != 16 || got.Changed {
			t.Fatalf("workspace %+v", got)
		}
	})
}

func TestSessionsArgvRendering(t *testing.T) {
	tests := []struct {
		name string
		opts *SessionsOptions
		want string
	}{
		{name: "nil options", opts: nil, want: "sessions --json"},
		{name: "all", opts: &SessionsOptions{All: true}, want: "sessions --all --json"},
		{name: "limit and cursor", opts: &SessionsOptions{Limit: 25, Cursor: "c1"}, want: "sessions --limit 25 --cursor c1 --json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, record := recordingClient(t, "sessions")
			if _, err := client.Sessions(context.Background(), tc.opts); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(record().Argv, " "); got != tc.want {
				t.Fatalf("argv %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWorkspaceArgvRendering(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		run  func(c *Client) error
		want string
	}{
		{name: "list", run: func(c *Client) error { _, err := c.WorkspaceList(ctx); return err }, want: "workspace list --json"},
		{name: "add", run: func(c *Client) error { _, err := c.WorkspaceAdd(ctx, "/extra"); return err }, want: "workspace add /extra --json"},
		{name: "remove", run: func(c *Client) error { _, err := c.WorkspaceRemove(ctx, "/extra"); return err }, want: "workspace remove /extra --json"},
		{name: "clear", run: func(c *Client) error { _, err := c.WorkspaceClear(ctx); return err }, want: "workspace clear --json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, record := recordingClient(t, "workspace")
			if err := tc.run(client); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(record().Argv, " "); got != tc.want {
				t.Fatalf("argv %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	got, err := mockClient(t, "status").Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != TestedFXVersion {
		t.Fatalf("version %q", got)
	}
}

func TestAdminHonorsCancellation(t *testing.T) {
	client := mockClient(t, "status")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Status(ctx)
	requireFxError(t, err, KindInterrupted)
}

func TestRunJSONRejectsEmptyArgs(t *testing.T) {
	client := mockClient(t, "status")
	if err := client.RunJSON(context.Background(), nil); err == nil {
		t.Fatal("expected a validation error for an empty subcommand")
	} else {
		requireFxError(t, err, KindValidation)
	}
}

func TestRunJSONIsAnEscapeHatch(t *testing.T) {
	client := mockClient(t, "status")
	var raw json.RawMessage
	if err := client.RunJSON(context.Background(), &raw, "status", "--json"); err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("RunJSON returned no payload")
	}
}
