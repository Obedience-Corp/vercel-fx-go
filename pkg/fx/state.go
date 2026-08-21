package fx

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// FxHome returns the fx state directory: $FX_HOME when set, else ~/.fx.
func FxHome() (string, error) {
	if dir := os.Getenv("FX_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", transportError("resolve home directory", err)
	}
	return filepath.Join(home, ".fx"), nil
}

// ModelUsage is the per-model rollup inside a session usage snapshot.
type ModelUsage struct {
	Model            string  `json:"model"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	ReasoningTokens  int     `json:"reasoning_tokens"`
	RequestCount     int     `json:"request_count"`
	TotalCost        float64 `json:"total_cost"`
}

// UsageSnapshot is the token accounting fx keeps per session. It is the only
// per-session token source; neither ask nor ACP report tokens.
type UsageSnapshot struct {
	SchemaVersion    int          `json:"schema_version"`
	Billing          string       `json:"billing"`
	InputTokens      int          `json:"input_tokens"`
	OutputTokens     int          `json:"output_tokens"`
	CacheReadTokens  int          `json:"cache_read_tokens"`
	CacheWriteTokens int          `json:"cache_write_tokens"`
	ReasoningTokens  int          `json:"reasoning_tokens"`
	RequestCount     int          `json:"request_count"`
	TotalCost        float64      `json:"total_cost"`
	Models           []ModelUsage `json:"models"`
}

type usageFile struct {
	SchemaVersion int           `json:"schema_version"`
	SessionID     string        `json:"session_id"`
	Snapshot      UsageSnapshot `json:"snapshot"`
}

// Supported on-disk schema versions for a session usage snapshot.
const (
	usageFileSchema     = 1
	usageSnapshotSchema = 2
)

// SessionUsage reads sessions/<id>/usage-v2.json. It never writes and refuses
// schema versions it was not written against.
func SessionUsage(ctx context.Context, id string) (*UsageSnapshot, error) {
	if id == "" {
		return nil, validationError("session id must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, &Error{Kind: KindInterrupted, Message: "context done before reading session usage", Original: err}
	}
	home, err := FxHome()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, "sessions", id, "usage-v2.json")
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, transportError("read "+path, readErr)
	}
	var parsed usageFile
	if jsonErr := json.Unmarshal(data, &parsed); jsonErr != nil {
		return nil, validationErrorWith("decode "+path, jsonErr)
	}
	return validateUsageSchema(&parsed)
}

func validateUsageSchema(parsed *usageFile) (*UsageSnapshot, *Error) {
	if parsed.SchemaVersion != usageFileSchema {
		return nil, validationError("unsupported usage file schema_version; the SDK reads version 1")
	}
	if parsed.Snapshot.SchemaVersion != usageSnapshotSchema {
		return nil, validationError("unsupported usage snapshot schema_version; the SDK reads version 2")
	}
	snapshot := parsed.Snapshot
	return &snapshot, nil
}

// GenerationFact is one billable generation recorded in usage.jsonl.
type GenerationFact struct {
	ID                     string  `json:"id"`
	CreatedAtMS            int64   `json:"created_at_ms"`
	Model                  string  `json:"model"`
	InputTokens            int     `json:"input_tokens"`
	OutputTokens           int     `json:"output_tokens"`
	CacheReadTokens        int     `json:"cache_read_tokens"`
	CacheWriteTokens       int     `json:"cache_write_tokens"`
	ReasoningTokens        int     `json:"reasoning_tokens"`
	BillableWebSearchCalls int     `json:"billable_web_search_calls"`
	TotalCost              float64 `json:"total_cost"`
}

// ReadUsageLog streams the generation facts recorded at or after since. A zero
// time returns every record.
func ReadUsageLog(ctx context.Context, since time.Time) ([]GenerationFact, error) {
	if err := ctx.Err(); err != nil {
		return nil, &Error{Kind: KindInterrupted, Message: "context done before reading the usage log", Original: err}
	}
	home, err := FxHome()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, "usage.jsonl")
	file, openErr := os.Open(path)
	if openErr != nil {
		return nil, transportError("open "+path, openErr)
	}
	defer file.Close()
	return scanUsageLog(ctx, file, since)
}

func scanUsageLog(ctx context.Context, file *os.File, since time.Time) ([]GenerationFact, error) {
	cutoff := int64(0)
	if !since.IsZero() {
		cutoff = since.UnixMilli()
	}
	var facts []GenerationFact
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return facts, &Error{Kind: KindInterrupted, Message: "context done while reading the usage log", Original: err}
		}
		var entry struct {
			Kind string          `json:"kind"`
			Fact *GenerationFact `json:"fact"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Kind != "generation" || entry.Fact == nil || entry.Fact.CreatedAtMS < cutoff {
			continue
		}
		facts = append(facts, *entry.Fact)
	}
	if err := scanner.Err(); err != nil {
		return facts, transportError("read usage log", err)
	}
	return facts, nil
}
