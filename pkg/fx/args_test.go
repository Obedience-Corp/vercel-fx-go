package fx

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildAskArgs(t *testing.T) {
	steps := 12
	tests := []struct {
		name   string
		prompt string
		opts   *AskOptions
		want   []string
	}{
		{
			name:   "nil options still forces json and separates the prompt",
			prompt: "hello",
			opts:   nil,
			want:   []string{"ask", "--json", "--", "hello"},
		},
		{
			name:   "prompt starting with a dash stays a prompt",
			prompt: "--not-a-flag",
			opts:   &AskOptions{},
			want:   []string{"ask", "--json", "--", "--not-a-flag"},
		},
		{
			name:   "stdin mode omits the prompt and the separator",
			prompt: "",
			opts:   &AskOptions{Quiet: true},
			want:   []string{"ask", "--json", "--quiet"},
		},
		{
			name:   "globals lead the subcommand",
			prompt: "go",
			opts: &AskOptions{
				AddDirs:          []string{"/a", "/b"},
				NoAdditionalDirs: true,
				ContextLimits:    map[string]string{"tool": "off", "file": "2048"},
			},
			want: []string{
				"--add-dir", "/a", "--add-dir", "/b", "--no-additional-dirs",
				"--context-limit", "file=2048", "--context-limit", "tool=off",
				"ask", "--json", "--", "go",
			},
		},
		{
			name:   "session and toggle flags",
			prompt: "go",
			opts: &AskOptions{
				Auto: true, Images: []string{"a.png", "b.png"},
				NoColor: true, Resume: "last", ContinueRecovery: true,
				MaxAgentSteps: &steps,
			},
			want: []string{
				"ask", "--auto", "--image", "a.png", "--image", "b.png", "--json",
				"--no-color", "--resume", "last", "--continue-recovery", "--", "go",
			},
		},
		{
			name:   "yolo flag is rendered when the caller accepted the risk",
			prompt: "go",
			opts:   &AskOptions{Yolo: true, AllowDangerousMode: true, NoSave: true},
			want:   []string{"ask", "--yolo", "--json", "--no-save", "--", "go"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildAskArgs(tc.prompt, tc.opts)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v\nwant %v", got, tc.want)
			}
		})
	}
}

func TestBuildACPArgs(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ACPConfig
		want []string
	}{
		{name: "nil config", cfg: nil, want: []string{"acp"}},
		{
			name: "model and log file",
			cfg:  &ACPConfig{Model: "zai/glm-5.2", LogFile: "/tmp/fx.log"},
			want: []string{"acp", "--model", "zai/glm-5.2", "--log-file", "/tmp/fx.log"},
		},
		{
			name: "globals lead acp too",
			cfg:  &ACPConfig{AddDirs: []string{"/w"}, NoAdditionalDirs: true},
			want: []string{"--add-dir", "/w", "--no-additional-dirs", "acp"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildACPArgs(tc.cfg)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBuildEnv(t *testing.T) {
	steps := 7
	tests := []struct {
		name string
		opts *AskOptions
		want []string
	}{
		{
			name: "nil options still disable upgrades and the browser",
			opts: nil,
			want: []string{"FX_AUTO_UPGRADE=0", "FX_NO_OPEN_BROWSER=1"},
		},
		{
			name: "typed overrides precede the mandatory pair",
			opts: &AskOptions{Model: "zai/glm-5.2", PermissionMode: PermissionAsk, MaxAgentSteps: &steps},
			want: []string{
				"FX_MODEL=zai/glm-5.2", "FX_PERMISSION_MODE=ask", "FX_MAX_AGENT_STEPS=7",
				"FX_AUTO_UPGRADE=0", "FX_NO_OPEN_BROWSER=1",
			},
		},
		{
			name: "passthrough env cannot override the mandatory pair",
			opts: &AskOptions{Env: []string{"FX_AUTO_UPGRADE=1", "FX_TRACE=1"}},
			want: []string{"FX_AUTO_UPGRADE=1", "FX_TRACE=1", "FX_AUTO_UPGRADE=0", "FX_NO_OPEN_BROWSER=1"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildEnv(tc.opts)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			if last := strings.Join(got[len(got)-2:], " "); last != "FX_AUTO_UPGRADE=0 FX_NO_OPEN_BROWSER=1" {
				t.Fatalf("mandatory env is not last: %q", last)
			}
		})
	}
}
