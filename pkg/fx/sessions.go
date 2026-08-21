package fx

import (
	"context"
	"encoding/json"
	"strconv"
)

// SessionRecord is one entry of "fx sessions --json".
type SessionRecord struct {
	ID                   string  `json:"id"`
	Title                string  `json:"title"`
	Preview              *string `json:"preview"`
	WorkspaceRoot        string  `json:"workspace_root"`
	OriginWorkspaceRoot  string  `json:"origin_workspace_root"`
	CreatedAtMS          int64   `json:"created_at_ms"`
	UpdatedAtMS          int64   `json:"updated_at_ms"`
	HistoryLen           int     `json:"history_len"`
	ConversationLanguage string  `json:"conversation_language"`
}

// SessionList is the "fx sessions" reply, scoped to the primary workspace.
type SessionList struct {
	Kind     string          `json:"kind"`
	Count    int             `json:"count"`
	Sessions []SessionRecord `json:"sessions"`
}

// SessionDetail is the "fx session" reply. History entries keep their raw JSON.
type SessionDetail struct {
	Kind                 string            `json:"kind"`
	ID                   string            `json:"id"`
	CreatedAtMS          int64             `json:"created_at_ms"`
	UpdatedAtMS          int64             `json:"updated_at_ms"`
	HistoryLen           int               `json:"history_len"`
	ConversationLanguage string            `json:"conversation_language"`
	History              []json.RawMessage `json:"history"`
}

// SessionOpResult is the reply of a session maintenance command. The full
// payload is preserved because its shape varies by fx version.
type SessionOpResult struct {
	Kind string          `json:"kind"`
	ID   string          `json:"id,omitempty"`
	Raw  json.RawMessage `json:"-"`
}

// BackgroundList is the "fx background" reply. Records keep their raw JSON.
type BackgroundList struct {
	Kind    string            `json:"kind"`
	Count   int               `json:"count"`
	Records []json.RawMessage `json:"records"`
}

// WorkspaceInfo is the "fx workspace" reply.
type WorkspaceInfo struct {
	Kind                  string   `json:"kind"`
	Action                string   `json:"action"`
	Changed               bool     `json:"changed"`
	PrimaryDirectory      string   `json:"primary_directory"`
	SavedSuppressed       bool     `json:"saved_suppressed"`
	Limit                 int      `json:"limit"`
	AdditionalDirectories []string `json:"additional_directories"`
}

// SessionsOptions filters the session listing.
type SessionsOptions struct {
	All    bool
	Limit  int
	Cursor string
}

func (o *SessionsOptions) args() ([]string, *Error) {
	args := []string{"sessions"}
	if o == nil {
		return append(args, "--json"), nil
	}
	if o.All {
		args = append(args, "--all")
	}
	if o.Limit != 0 {
		if o.Limit < 1 || o.Limit > 100 {
			return nil, validationError("sessions limit must be between 1 and 100")
		}
		args = append(args, "--limit", strconv.Itoa(o.Limit))
	}
	if o.Cursor != "" {
		args = append(args, "--cursor", o.Cursor)
	}
	return append(args, "--json"), nil
}

// Sessions lists saved sessions for the client working directory.
func (c *Client) Sessions(ctx context.Context, opts *SessionsOptions) (*SessionList, error) {
	args, err := opts.args()
	if err != nil {
		return nil, err
	}
	var out SessionList
	if runErr := c.runJSON(ctx, &out, args...); runErr != nil {
		return nil, runErr
	}
	return &out, nil
}

// Session inspects one saved session by id, or "last" for the newest.
func (c *Client) Session(ctx context.Context, id string) (*SessionDetail, error) {
	if id == "" {
		return nil, validationError("session id must not be empty")
	}
	args := []string{"session", "--id", id, "--json"}
	if id == "last" {
		args = []string{"session", "last", "--json"}
	}
	var out SessionDetail
	if err := c.runJSON(ctx, &out, args...); err != nil {
		return nil, err
	}
	return &out, nil
}

// SessionMigrate rewrites a saved session into the current storage format.
func (c *Client) SessionMigrate(ctx context.Context, id string, allowLarge bool) (*SessionOpResult, error) {
	if id == "" {
		return nil, validationError("session id must not be empty")
	}
	args := []string{"session", "migrate", id}
	if allowLarge {
		args = append(args, "--allow-large")
	}
	return c.sessionOp(ctx, append(args, "--json"))
}

// SessionRecover copies a corrupt session into a new resumable one.
func (c *Client) SessionRecover(ctx context.Context, id string) (*SessionOpResult, error) {
	if id == "" {
		return nil, validationError("session id must not be empty")
	}
	return c.sessionOp(ctx, []string{"session", "recover", id, "--json"})
}

func (c *Client) sessionOp(ctx context.Context, args []string) (*SessionOpResult, error) {
	var raw json.RawMessage
	if err := c.runJSON(ctx, &raw, args...); err != nil {
		return nil, err
	}
	out := SessionOpResult{Raw: raw}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, validationErrorWith("decode fx session reply", err)
	}
	out.Raw = raw
	return &out, nil
}

// Background lists persisted background command records.
func (c *Client) Background(ctx context.Context) (*BackgroundList, error) {
	var out BackgroundList
	if err := c.runJSON(ctx, &out, "background", "--json"); err != nil {
		return nil, err
	}
	return &out, nil
}

// BackgroundRecord inspects one background record by id, or "last".
func (c *Client) BackgroundRecord(ctx context.Context, idOrLast string) (*BackgroundList, error) {
	if idOrLast == "" {
		return nil, validationError("background record id must not be empty")
	}
	var out BackgroundList
	if err := c.runJSON(ctx, &out, "background", idOrLast, "--json"); err != nil {
		return nil, err
	}
	return &out, nil
}

// WorkspaceList lists the additional directories of the primary workspace.
func (c *Client) WorkspaceList(ctx context.Context) (*WorkspaceInfo, error) {
	return c.workspaceOp(ctx, "list", "")
}

// WorkspaceAdd adds an additional directory to the primary workspace.
func (c *Client) WorkspaceAdd(ctx context.Context, path string) (*WorkspaceInfo, error) {
	return c.workspaceOp(ctx, "add", path)
}

// WorkspaceRemove removes an additional directory from the primary workspace.
func (c *Client) WorkspaceRemove(ctx context.Context, path string) (*WorkspaceInfo, error) {
	return c.workspaceOp(ctx, "remove", path)
}

// WorkspaceClear removes every additional directory.
func (c *Client) WorkspaceClear(ctx context.Context) (*WorkspaceInfo, error) {
	return c.workspaceOp(ctx, "clear", "")
}

func (c *Client) workspaceOp(ctx context.Context, action, path string) (*WorkspaceInfo, error) {
	args := []string{"workspace", action}
	if action == "add" || action == "remove" {
		if path == "" {
			return nil, validationError("workspace " + action + " needs a directory path")
		}
		args = append(args, path)
	}
	var out WorkspaceInfo
	if err := c.runJSON(ctx, &out, append(args, "--json")...); err != nil {
		return nil, err
	}
	return &out, nil
}
