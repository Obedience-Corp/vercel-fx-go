package dangerous

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Obedience-Corp/vercel-fx-go/pkg/fx"
)

func enable(t *testing.T) {
	t.Helper()
	t.Setenv(EnableEnv, EnableValue)
	t.Setenv("GO_ENV", "")
	t.Setenv("NODE_ENV", "")
}

func TestGateRefusesWithoutOptIn(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		wantErr error
	}{
		{
			name:    "opt in missing",
			setup:   func(t *testing.T) { t.Setenv(EnableEnv, "") },
			wantErr: ErrNotEnabled,
		},
		{
			name:    "opt in has the wrong value",
			setup:   func(t *testing.T) { t.Setenv(EnableEnv, "yes-please") },
			wantErr: ErrNotEnabled,
		},
		{
			name: "GO_ENV is production",
			setup: func(t *testing.T) {
				t.Setenv(EnableEnv, EnableValue)
				t.Setenv("GO_ENV", "production")
			},
			wantErr: ErrProduction,
		},
		{
			name: "NODE_ENV is production",
			setup: func(t *testing.T) {
				t.Setenv(EnableEnv, EnableValue)
				t.Setenv("GO_ENV", "")
				t.Setenv("NODE_ENV", "production")
			},
			wantErr: ErrProduction,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			if _, err := NewDangerousClient("/bin/echo"); !errors.Is(err, tc.wantErr) {
				t.Fatalf("NewDangerousClient err %v, want %v", err, tc.wantErr)
			}
			if _, err := AskOptions(nil); !errors.Is(err, tc.wantErr) {
				t.Fatalf("AskOptions err %v, want %v", err, tc.wantErr)
			}
			if _, err := ACPConfig(nil); !errors.Is(err, tc.wantErr) {
				t.Fatalf("ACPConfig err %v, want %v", err, tc.wantErr)
			}
			if err := Enabled(); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Enabled err %v, want %v", err, tc.wantErr)
			}
			if _, err := Wrap(fx.NewClient("/bin/echo")); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Wrap err %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestWrapRejectsNilClient(t *testing.T) {
	enable(t)
	_, err := Wrap(nil)
	if err == nil || !strings.Contains(err.Error(), "must not be nil") {
		t.Fatalf("err %v", err)
	}
}

func TestGateAllowsWhenAcknowledged(t *testing.T) {
	enable(t)
	if err := Enabled(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	client, err := NewDangerousClient("/bin/echo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.Unwrap() == nil {
		t.Fatal("Unwrap must expose the underlying client")
	}
}

func TestAskOptionsEnablesYolo(t *testing.T) {
	enable(t)
	caller := &fx.AskOptions{Model: "zai/glm-5.2", NoSave: true}
	got, err := AskOptions(caller)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Yolo || got.PermissionMode != fx.PermissionYolo || !got.AllowDangerousMode {
		t.Fatalf("options %+v", got)
	}
	if got.Model != "zai/glm-5.2" || !got.NoSave {
		t.Fatalf("caller options were dropped: %+v", got)
	}
	if caller.Yolo || caller.AllowDangerousMode {
		t.Fatal("the caller options were mutated")
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("the produced options must validate: %v", err)
	}
	args := fx.BuildAskArgs("hi", got)
	if strings.Join(args, " ") != "ask --yolo --json --no-save -- hi" {
		t.Fatalf("argv %v", args)
	}
	env := strings.Join(fx.BuildEnv(got), " ")
	if !strings.Contains(env, "FX_PERMISSION_MODE=yolo") {
		t.Fatalf("env %q", env)
	}
}

func TestACPConfigEnablesYolo(t *testing.T) {
	enable(t)
	caller := &fx.ACPConfig{Model: "zai/glm-5.2"}
	got, err := ACPConfig(caller)
	if err != nil {
		t.Fatal(err)
	}
	if got.PermissionMode != fx.PermissionYolo || !got.AllowDangerousMode {
		t.Fatalf("config %+v", got)
	}
	if caller.AllowDangerousMode {
		t.Fatal("the caller config was mutated")
	}
	if got.Model != "zai/glm-5.2" {
		t.Fatalf("caller config was dropped: %+v", got)
	}
}

func mockBinPath(t *testing.T) string {
	t.Helper()
	_, this, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(this), "..", "..", "..")
	out := filepath.Join(t.TempDir(), "fx-mock")
	cmd := exec.Command("go", "build", "-o", out, "./test/mockfx")
	cmd.Dir = repo
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Logf("go build:\n%s", combined)
		t.Fatalf("build mock: %v", err)
	}
	return out
}

func TestYoloAgainstMock(t *testing.T) {
	enable(t)
	_, this, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(this), "..", "..", "..")
	client, err := NewDangerousClient(mockBinPath(t))
	if err != nil {
		t.Fatal(err)
	}
	inner := client.Unwrap()
	inner.WorkingDir = t.TempDir()
	inner.Env = []string{
		"FX_MOCK_TESTDATA=" + filepath.Join(repo, "test", "testdata"),
		"FX_MOCK_SCENARIO=ask-tool-write",
	}
	result, err := client.Yolo(context.Background(), "write hello.txt", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "write_file" {
		t.Fatalf("result %+v", result)
	}
}

func TestUpgradeCheckIsGated(t *testing.T) {
	t.Setenv(EnableEnv, "")
	client := &Client{}
	if _, err := client.UpgradeCheck(context.Background()); !errors.Is(err, ErrNotEnabled) {
		t.Fatalf("err %v, want %v", err, ErrNotEnabled)
	}
}
