//go:build integration

package integration

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/Obedience-Corp/vercel-fx-go/pkg/fx"
	"github.com/Obedience-Corp/vercel-fx-go/pkg/fx/dangerous"
)

const realModel = "zai/glm-5.2"

var (
	mockOnce sync.Once
	mockPath string
	mockErr  error
)

func realLane() bool { return os.Getenv("INTEGRATION_REAL") == "1" }

func repoRoot(t *testing.T) string {
	t.Helper()
	_, this, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve the repo root")
	}
	return filepath.Join(filepath.Dir(this), "..", "..")
}

func locateMock(t *testing.T) string {
	t.Helper()
	if env := os.Getenv("FX_MOCK_BIN"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	root := repoRoot(t)
	mockOnce.Do(func() {
		out := filepath.Join(root, "test", "mockfx", "bin", "fx-mock")
		cmd := exec.Command("go", "build", "-o", out, "./test/mockfx")
		cmd.Dir = root
		if combined, err := cmd.CombinedOutput(); err != nil {
			mockErr = err
			t.Logf("go build mock:\n%s", combined)
			return
		}
		mockPath = out
	})
	if mockErr != nil {
		t.Fatalf("build mock: %v", mockErr)
	}
	return mockPath
}

// scratchRepo makes a disposable git repository. fx treats the process cwd as
// the primary workspace, and its doctor checks expect git metadata.
func scratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	steps := [][]string{
		{"git", "init", "-q"},
		{"git", "add", "-A"},
		{"git", "-c", "user.email=scratch@local", "-c", "user.name=scratch", "commit", "-qm", "init"},
	}
	for _, step := range steps {
		cmd := exec.Command(step[0], step[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", step, err, out)
		}
	}
	return dir
}

func newClient(t *testing.T, scenario string) *fx.Client {
	t.Helper()
	workdir := scratchRepo(t)
	if realLane() {
		path := os.Getenv("FX_TEST_BIN")
		var err error
		if path == "" {
			path, err = exec.LookPath("fx")
		}
		if err != nil {
			located, locateErr := fx.LocateBinary()
			if locateErr != nil {
				t.Skip("INTEGRATION_REAL=1 but fx was not found")
			}
			path = located
		}
		client := fx.NewClient(path)
		client.WorkingDir = workdir
		return client
	}
	client := fx.NewClient(locateMock(t))
	client.WorkingDir = workdir
	client.Env = []string{
		"FX_MOCK_TESTDATA=" + filepath.Join(repoRoot(t), "test", "testdata"),
		"FX_MOCK_SCENARIO=" + scenario,
	}
	return client
}

func enableDangerous(t *testing.T) {
	t.Helper()
	t.Setenv(dangerous.EnableEnv, dangerous.EnableValue)
	t.Setenv("GO_ENV", "")
	t.Setenv("NODE_ENV", "")
}

// skipIfProviderDown turns a transient upstream outage into a skip.
func skipIfProviderDown(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	var fxErr *fx.Error
	if !errors.As(err, &fxErr) {
		return
	}
	if fxErr.Kind == fx.KindProviderUnavailable || fxErr.Kind == fx.KindRateLimit {
		t.Skipf("upstream provider unavailable, skipping: %v", fxErr)
	}
}

func logRecovery(t *testing.T, recovery *fx.Recovery) {
	t.Helper()
	if recovery == nil {
		return
	}
	t.Logf("fx recovery: state=%s kind=%s attempt=%d/%d durable=%v",
		recovery.State, recovery.Kind, recovery.Attempt, recovery.AttemptLimit, recovery.Durable)
}
