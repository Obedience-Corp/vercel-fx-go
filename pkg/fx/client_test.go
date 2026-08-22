package fx

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func countEnvKey(env []string, key string) int {
	count := 0
	for _, entry := range env {
		if name, _, ok := strings.Cut(entry, "="); ok && name == key {
			count++
		}
	}
	return count
}

// TestEnvWith_PoisonedParentEnvIsOverridden runs a real child process with the
// exact env the SDK builds, so the guarantee does not rest on os/exec resolving
// duplicate keys last-wins.
func TestEnvWith_PoisonedParentEnvIsOverridden(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the probe shells out to /bin/sh")
	}
	t.Setenv("FX_AUTO_UPGRADE", "1")
	t.Setenv("FX_NO_OPEN_BROWSER", "0")
	t.Setenv("FX_MODEL", "poisoned/model")

	client := &Client{Env: []string{"FX_AUTO_UPGRADE=1"}}
	env := client.envWith(BuildEnv(&AskOptions{
		Model: "zai/glm-5.2",
		Env:   []string{"FX_NO_OPEN_BROWSER=0"},
	}))

	for _, key := range []string{"FX_AUTO_UPGRADE", "FX_NO_OPEN_BROWSER", "FX_MODEL"} {
		if got := countEnvKey(env, key); got != 1 {
			t.Fatalf("%s appears %d times in the built env, want exactly 1", key, got)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c",
		`printf '%s|%s|%s' "$FX_AUTO_UPGRADE" "$FX_NO_OPEN_BROWSER" "$FX_MODEL"`)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("probe process: %v", err)
	}
	if string(out) != "0|1|zai/glm-5.2" {
		t.Fatalf("child saw %q, want %q", out, "0|1|zai/glm-5.2")
	}
}

func TestEnvWithKeepsUnrelatedEntries(t *testing.T) {
	t.Setenv("FX_THEME", "dark")
	client := &Client{Env: []string{"FX_TRACE=1"}}
	env := client.envWith(BuildEnv(nil))
	if countEnvKey(env, "FX_THEME") != 1 || countEnvKey(env, "FX_TRACE") != 1 {
		t.Fatalf("unrelated FX_* entries were dropped: %v", env)
	}
	last := env[len(env)-2:]
	if last[0] != "FX_AUTO_UPGRADE=0" || last[1] != "FX_NO_OPEN_BROWSER=1" {
		t.Fatalf("managed pair is not last: %v", last)
	}
}

func TestWorkDirFallsBackToProcessCwd(t *testing.T) {
	client := &Client{}
	got, err := client.workDir("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Fatal("cmd.Dir must always be set")
	}
	if override, err := client.workDir("/explicit"); err != nil || override != "/explicit" {
		t.Fatalf("override %q err %v", override, err)
	}
	client.WorkingDir = "/client"
	if got, err := client.workDir(""); err != nil || got != "/client" {
		t.Fatalf("client dir %q err %v", got, err)
	}
}
