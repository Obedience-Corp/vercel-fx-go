package fx

import (
	"context"
	"encoding/json"
	"strings"
)

// StatusInfo is the effective fx configuration reported by "fx status".
type StatusInfo struct {
	Kind               string   `json:"kind"`
	Model              string   `json:"model"`
	ModelSource        string   `json:"model_source,omitempty"`
	ConnectedProviders []string `json:"connected_providers,omitempty"`
	UpdateChannel      string   `json:"update_channel"`
	BuildChannel       string   `json:"build_channel"`
	BuildRevision      string   `json:"build_revision"`
	MCPConfigError     string   `json:"mcp_config_error,omitempty"`
	Auth               string   `json:"auth"`
	AuthRefreshable    bool     `json:"auth_refreshable"`
	AuthExpired        bool     `json:"auth_expired"`
	AuthHelp           string   `json:"auth_help,omitempty"`
	Team               string   `json:"team"`
	PermissionMode     string   `json:"permission_mode"`
	// Sandbox is retained for decoding legacy status replies. Current fx
	// releases do not provide sandboxing or emit this field.
	Sandbox                 string `json:"sandbox"`
	Workspace               string `json:"workspace"`
	HistoryTurns            int    `json:"history_turns"`
	SessionPermissionGrants int    `json:"session_permission_grants"`
	AgentStepLimit          int    `json:"agent_step_limit"`
}

// DoctorReport is the "fx doctor" health summary.
type DoctorReport struct {
	Kind            string        `json:"kind"`
	OKCount         int           `json:"ok_count"`
	WarnCount       int           `json:"warn_count"`
	FailCount       int           `json:"fail_count"`
	Workspace       string        `json:"workspace"`
	Model           string        `json:"model"`
	ModelSource     string        `json:"model_source,omitempty"`
	Auth            string        `json:"auth"`
	AuthRefreshable bool          `json:"auth_refreshable"`
	Team            string        `json:"team"`
	PermissionMode  string        `json:"permission_mode"`
	AgentStepLimit  int           `json:"agent_step_limit"`
	Checks          []DoctorCheck `json:"checks"`
}

// DoctorCheck is one preflight check result.
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Healthy reports whether no doctor check failed.
func (d *DoctorReport) Healthy() bool { return d != nil && d.FailCount == 0 }

// ModelCatalog is the provider-aware "fx models" reply.
type ModelCatalog struct {
	Kind                string      `json:"kind"`
	Count               int         `json:"count"`
	ShownCount          int         `json:"shown_count"`
	MoreCount           int         `json:"more_count"`
	PrivateModelsHidden bool        `json:"private_models_hidden"`
	IDs                 []string    `json:"ids"`
	Models              []ModelInfo `json:"models,omitempty"`
}

// ModelInfo identifies a model and the provider route reported by fx.
type ModelInfo struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

// PermissionsInfo is the "fx permissions" reply. Rules and grants keep their
// raw JSON because their shape is configuration dependent.
type PermissionsInfo struct {
	Kind                   string            `json:"kind"`
	Mode                   string            `json:"mode"`
	GrantCount             int               `json:"grant_count"`
	GrantScope             string            `json:"grant_scope"`
	RuntimeGrantsAvailable bool              `json:"runtime_grants_available"`
	RulesScope             string            `json:"rules_scope"`
	Rules                  []json.RawMessage `json:"rules"`
	Grants                 []json.RawMessage `json:"grants"`
}

// CreditsInfo is the AI Gateway balance. fx emits the balance as a string.
type CreditsInfo struct {
	Kind    string          `json:"kind"`
	Balance string          `json:"balance"`
	Used    json.RawMessage `json:"used,omitempty"`
	Plan    json.RawMessage `json:"plan,omitempty"`
}

// UnmarshalJSON decodes credits, tolerating a numeric balance.
func (c *CreditsInfo) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Kind = rawString(raw, "kind")
	c.Balance = rawString(raw, "balance")
	if c.Balance == "" {
		c.Balance = rawLiteral(raw, "balance")
	}
	c.Used = rawNonNull(raw, "used")
	c.Plan = rawNonNull(raw, "plan")
	return nil
}

// UsageTotals is a token and spend rollup.
type UsageTotals struct {
	TotalTokens      int     `json:"total_tokens"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	ReasoningTokens  int     `json:"reasoning_tokens"`
	RequestCount     int     `json:"request_count"`
	Spend            float64 `json:"spend"`
}

// UsageCoverage says how much of the requested window fx actually observed.
type UsageCoverage struct {
	Status      string `json:"status"`
	StartedAtMS int64  `json:"started_at_ms"`
	FullWindow  bool   `json:"full_window"`
}

// UsageModel is the per-model breakdown of a usage report.
type UsageModel struct {
	Model  string      `json:"model"`
	Totals UsageTotals `json:"totals"`
}

// UsageReport is the "fx usage" reply. It covers this machine only.
type UsageReport struct {
	Kind           string        `json:"kind"`
	SchemaVersion  int           `json:"schema_version"`
	Period         string        `json:"period"`
	SnapshotTimeMS int64         `json:"snapshot_time_ms"`
	WindowStartMS  int64         `json:"window_start_ms"`
	Coverage       UsageCoverage `json:"coverage"`
	Completeness   string        `json:"completeness"`
	Totals         UsageTotals   `json:"totals"`
	Models         []UsageModel  `json:"models"`
}

// Version returns the version string printed by "fx --version".
func (c *Client) Version(ctx context.Context) (string, error) {
	dir, dirErr := c.workDir("")
	if dirErr != nil {
		return "", dirErr
	}
	outcome := c.runCommand(ctx, []string{"--version"}, c.adminEnv(), dir, nil)
	if outcome.exitCode != 0 || outcome.err != nil {
		return "", processError("fx --version failed", outcome.exitCode, outcome.stderr, outcome.err)
	}
	return strings.TrimSpace(string(outcome.stdout)), nil
}

// Status reports the effective configuration for the client working directory.
func (c *Client) Status(ctx context.Context) (*StatusInfo, error) {
	var out StatusInfo
	if err := c.runJSON(ctx, &out, "status", "--json"); err != nil {
		return nil, err
	}
	return &out, nil
}

// Doctor runs the local health checks.
func (c *Client) Doctor(ctx context.Context) (*DoctorReport, error) {
	var out DoctorReport
	if err := c.runJSON(ctx, &out, "doctor", "--json"); err != nil {
		return nil, err
	}
	return &out, nil
}

// Models lists the model ids the current team can reach.
func (c *Client) Models(ctx context.Context) (*ModelCatalog, error) {
	var out ModelCatalog
	if err := c.runJSON(ctx, &out, "models", "--json"); err != nil {
		return nil, err
	}
	return &out, nil
}

// Permissions reports the permission mode, rules, and grants.
func (c *Client) Permissions(ctx context.Context) (*PermissionsInfo, error) {
	var out PermissionsInfo
	if err := c.runJSON(ctx, &out, "permissions", "--json"); err != nil {
		return nil, err
	}
	return &out, nil
}

// Credits reports the AI Gateway credit balance.
func (c *Client) Credits(ctx context.Context) (*CreditsInfo, error) {
	var out CreditsInfo
	if err := c.runJSON(ctx, &out, "credits", "--json"); err != nil {
		return nil, err
	}
	return &out, nil
}

// Usage reports local token usage and spend. Period is "24h", "7d", or "30d".
func (c *Client) Usage(ctx context.Context, period string) (*UsageReport, error) {
	args := []string{"usage"}
	if period != "" {
		if err := validateUsagePeriod(period); err != nil {
			return nil, err
		}
		args = append(args, "--period", period)
	}
	var out UsageReport
	if err := c.runJSON(ctx, &out, append(args, "--json")...); err != nil {
		return nil, err
	}
	return &out, nil
}

func validateUsagePeriod(period string) *Error {
	switch period {
	case "24h", "7d", "30d":
		return nil
	}
	return validationError("usage period must be \"24h\", \"7d\", or \"30d\"")
}

func (c *Client) adminEnv() []string {
	return BuildEnv(c.DefaultOptions)
}

// RunJSON runs an arbitrary fx subcommand that supports --json and decodes the
// reply into out. It is the escape hatch for commands the SDK does not wrap.
func (c *Client) RunJSON(ctx context.Context, out any, args ...string) error {
	if err := c.runJSON(ctx, out, args...); err != nil {
		return err
	}
	return nil
}

func (c *Client) runJSON(ctx context.Context, out any, args ...string) *Error {
	if len(args) == 0 {
		return validationError("no fx subcommand was given")
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return &Error{Kind: KindInterrupted, Message: "context done before fx " + args[0], Original: ctxErr}
	}
	dir, dirErr := c.workDir("")
	if dirErr != nil {
		return dirErr
	}
	outcome := c.runCommand(ctx, args, c.adminEnv(), dir, nil)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return &Error{Kind: KindInterrupted, Message: "fx " + args[0] + " canceled", Stderr: outcome.stderr, Original: ctxErr}
	}
	if outcome.exitCode < 0 && outcome.err != nil {
		return transportError("run fx "+args[0], outcome.err)
	}
	return decodeJSONReply(outcome, args[0], out)
}

func decodeJSONReply(outcome commandOutcome, command string, out any) *Error {
	trimmed := strings.TrimSpace(string(outcome.stdout))
	if trimmed == "" {
		return processError("fx "+command+" produced no output on stdout", outcome.exitCode, outcome.stderr, outcome.err)
	}
	var envelope struct {
		Kind  string `json:"kind"`
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return validationErrorWith("fx "+command+" stdout is not JSON: "+truncate([]byte(trimmed), 400), err)
	}
	if envelope.Error != "" {
		return &Error{Kind: mapCode(envelope.Code, envelope.Error), Message: envelope.Error, ExitCode: outcome.exitCode, Stderr: outcome.stderr}
	}
	if outcome.exitCode != 0 {
		return processError("fx "+command+" exited non-zero", outcome.exitCode, outcome.stderr, outcome.err)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal([]byte(trimmed), out); err != nil {
		return validationErrorWith("decode fx "+command+" reply", err)
	}
	return nil
}

func mapCode(code, message string) Kind {
	lower := strings.ToLower(code + " " + message)
	switch {
	case strings.Contains(lower, "auth") || strings.Contains(lower, "credential") || strings.Contains(lower, "sign in"):
		return KindAuth
	case strings.Contains(lower, "notfound") || strings.Contains(lower, "nosaved") || strings.Contains(lower, "not found"):
		return KindValidation
	case strings.Contains(lower, "invalid") || strings.Contains(lower, "unsupported"):
		return KindValidation
	}
	return KindProcess
}
