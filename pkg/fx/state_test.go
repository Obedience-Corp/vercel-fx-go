package fx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeFxHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FX_HOME", home)
	return home
}

func writeSessionUsage(t *testing.T, home, id string, body []byte) {
	t.Helper()
	dir := filepath.Join(home, "sessions", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "usage-v2.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFxHomeHonorsEnv(t *testing.T) {
	home := fakeFxHome(t)
	got, err := FxHome()
	if err != nil {
		t.Fatal(err)
	}
	if got != home {
		t.Fatalf("home %q, want %q", got, home)
	}
}

func TestFxHomeDefaultsToDotFx(t *testing.T) {
	t.Setenv("FX_HOME", "")
	got, err := FxHome()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != ".fx" {
		t.Fatalf("home %q, want a .fx directory", got)
	}
}

func TestSessionUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		body    string
		wantSub string
		wantErr Kind
	}{
		{name: "empty id", id: "", wantSub: "session id", wantErr: KindValidation},
		{name: "parent traversal", id: "../outside", wantSub: "session id", wantErr: KindValidation},
		{name: "nested path", id: "nested/session", wantSub: "session id", wantErr: KindValidation},
		{name: "windows path", id: `nested\session`, wantSub: "session id", wantErr: KindValidation},
		{name: "oversized id", id: strings.Repeat("a", 256), wantSub: "session id", wantErr: KindValidation},
		{name: "missing file", id: "absent", wantSub: "usage-v2.json", wantErr: KindTransport},
		{
			name: "unsupported file schema", id: "s1",
			body:    `{"schema_version":2,"session_id":"s1","snapshot":{"schema_version":2}}`,
			wantSub: "usage file schema_version", wantErr: KindValidation,
		},
		{
			name: "unsupported snapshot schema", id: "s1",
			body:    `{"schema_version":1,"session_id":"s1","snapshot":{"schema_version":3}}`,
			wantSub: "usage snapshot schema_version", wantErr: KindValidation,
		},
		{
			name: "not json", id: "s1", body: "nope",
			wantSub: "decode", wantErr: KindValidation,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := fakeFxHome(t)
			if tc.body != "" {
				writeSessionUsage(t, home, tc.id, []byte(tc.body))
			}
			_, err := SessionUsage(context.Background(), tc.id)
			fxErr := requireFxError(t, err, tc.wantErr)
			if !strings.Contains(fxErr.Message, tc.wantSub) {
				t.Fatalf("message %q does not contain %q", fxErr.Message, tc.wantSub)
			}
		})
	}
}

func TestSessionUsageReadsRealCapture(t *testing.T) {
	home := fakeFxHome(t)
	id := "1787339138496-1787339138496206000-337bbe1c41adeae1"
	writeSessionUsage(t, home, id, readFixture(t, "session-usage-v2.json"))
	got, err := SessionUsage(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != 2 || got.Billing != "incomplete" {
		t.Fatalf("snapshot %+v", got)
	}
	if got.RequestCount != 0 || got.TotalCost != 0 {
		t.Fatalf("snapshot %+v", got)
	}
}

func TestSessionUsageHonorsCancellation(t *testing.T) {
	fakeFxHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := SessionUsage(ctx, "any")
	requireFxError(t, err, KindInterrupted)
}

func TestReadUsageLog(t *testing.T) {
	home := fakeFxHome(t)
	if err := os.WriteFile(filepath.Join(home, "usage.jsonl"), readFixture(t, "usage-jsonl-sample.jsonl"), 0o644); err != nil {
		t.Fatal(err)
	}
	all, err := ReadUsageLog(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("facts %d, want 2 (coverage rows are skipped)", len(all))
	}
	if all[0].Model != "zai/glm-5.2" || all[0].InputTokens != 19805 {
		t.Fatalf("fact %+v", all[0])
	}
	later, err := ReadUsageLog(context.Background(), time.UnixMilli(1787204186000))
	if err != nil {
		t.Fatal(err)
	}
	if len(later) != 1 || later[0].ID != "gen_01M0ETQD9F7YT564Q3G1387TRX" {
		t.Fatalf("filtered %+v", later)
	}
}

func TestReadUsageLogMissingFile(t *testing.T) {
	fakeFxHome(t)
	_, err := ReadUsageLog(context.Background(), time.Time{})
	requireFxError(t, err, KindTransport)
}
