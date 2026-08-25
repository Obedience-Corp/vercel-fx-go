package fx

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	mockOnce sync.Once
	mockDir  string
	mockPath string
	mockErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if mockDir != "" {
		_ = os.RemoveAll(mockDir)
	}
	os.Exit(code)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, this, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller for repo root")
	}
	return filepath.Join(filepath.Dir(this), "..", "..")
}

func mockBin(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	mockOnce.Do(func() {
		mockDir, mockErr = os.MkdirTemp("", "vercel-fx-go-mock-")
		if mockErr != nil {
			return
		}
		out := filepath.Join(mockDir, "fx-mock")
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
		t.Fatalf("build mock binary: %v", mockErr)
	}
	return mockPath
}

func testdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "test", "testdata")
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func mockClient(t *testing.T, scenario string) *Client {
	t.Helper()
	client := NewClient(mockBin(t))
	client.WorkingDir = t.TempDir()
	client.Env = []string{
		"FX_MOCK_TESTDATA=" + testdataDir(t),
		"FX_MOCK_SCENARIO=" + scenario,
	}
	return client
}

type mockRecord struct {
	Argv  []string          `json:"argv"`
	Env   map[string]string `json:"env"`
	Cwd   string            `json:"cwd"`
	Stdin string            `json:"stdin"`
}

func recordingClient(t *testing.T, scenario string) (*Client, func() mockRecord) {
	t.Helper()
	client := mockClient(t, scenario)
	path := filepath.Join(t.TempDir(), "record.json")
	client.Env = append(client.Env, "FX_MOCK_RECORD="+path)
	return client, func() mockRecord {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read mock record: %v", err)
		}
		var out mockRecord
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatalf("decode mock record: %v", err)
		}
		return out
	}
}

func requireFxError(t *testing.T, err error, kind Kind) *Error {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a %s error, got nil", kind)
	}
	fxErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *fx.Error, got %T: %v", err, err)
	}
	if fxErr.Kind != kind {
		t.Fatalf("expected kind %s, got %s (%v)", kind, fxErr.Kind, fxErr)
	}
	return fxErr
}
