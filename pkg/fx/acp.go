package fx

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// ACPProtocolVersion is the Agent Client Protocol version fx v0.0.4 speaks.
const ACPProtocolVersion = 1

// RPCMessage is one newline-delimited JSON-RPC 2.0 frame.
type RPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`

	Raw json.RawMessage `json:"-"`
}

// IsRequest reports whether the frame expects a response.
func (m *RPCMessage) IsRequest() bool { return m.Method != "" && len(m.ID) > 0 }

// IsNotification reports whether the frame is a one-way notification.
func (m *RPCMessage) IsNotification() bool { return m.Method != "" && len(m.ID) == 0 }

// IsResponse reports whether the frame answers an earlier request.
func (m *RPCMessage) IsResponse() bool { return m.Method == "" && len(m.ID) > 0 }

// RPCError is the JSON-RPC error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return "rpc error " + strconv.Itoa(e.Code) + ": " + e.Message
}

// ClientCapabilities is what the SDK advertises during initialize.
type ClientCapabilities struct {
	FS       FSCapabilities `json:"fs"`
	Terminal bool           `json:"terminal"`
}

// FSCapabilities declares client-side filesystem methods the agent may call.
type FSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

// ClientInfo identifies the calling client. fx v0.0.4 ignores it.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeParams is the initialize request payload.
type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         *ClientInfo        `json:"clientInfo,omitempty"`
}

// InitializeResult is the agent handshake reply.
type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
	AgentInfo         AgentInfo         `json:"agentInfo"`
	AuthMethods       []AuthMethod      `json:"authMethods"`
}

// AgentCapabilities is what fx reports it supports.
type AgentCapabilities struct {
	LoadSession         bool                `json:"loadSession"`
	PromptCapabilities  PromptCapabilities  `json:"promptCapabilities"`
	MCPCapabilities     MCPCapabilities     `json:"mcpCapabilities"`
	SessionCapabilities SessionCapabilities `json:"sessionCapabilities"`
}

// PromptCapabilities lists the prompt block types the agent accepts.
type PromptCapabilities struct {
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
	EmbeddedContext bool `json:"embeddedContext"`
}

// MCPCapabilities lists the MCP transports the agent accepts.
type MCPCapabilities struct {
	HTTP bool `json:"http"`
	SSE  bool `json:"sse"`
}

// SessionCapabilities is present for each session method the agent supports.
type SessionCapabilities struct {
	List   *struct{} `json:"list,omitempty"`
	Resume *struct{} `json:"resume,omitempty"`
	Close  *struct{} `json:"close,omitempty"`
}

// AgentInfo identifies the agent binary.
type AgentInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

// AuthMethod is an authentication route the agent offers. fx reports none.
type AuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPServerSpec is one MCP server the client supplies to a session.
type MCPServerSpec struct {
	Name    string            `json:"name"`
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// NewSessionParams creates or loads a session in a workspace.
type NewSessionParams struct {
	SessionID  string          `json:"sessionId,omitempty"`
	CWD        string          `json:"cwd"`
	MCPServers []MCPServerSpec `json:"mcpServers"`
}

// NewSessionResult carries the new session id and its config options.
type NewSessionResult struct {
	SessionID     string         `json:"sessionId"`
	ConfigOptions []ConfigOption `json:"configOptions,omitempty"`
}

// ConfigOption is one adjustable session setting, such as the model.
type ConfigOption struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Category     string               `json:"category"`
	Type         string               `json:"type"`
	CurrentValue string               `json:"currentValue"`
	Options      []ConfigOptionChoice `json:"options,omitempty"`
}

// ConfigOptionChoice is one selectable value of a ConfigOption.
type ConfigOptionChoice struct {
	Value string `json:"value"`
	Name  string `json:"name"`
}

// ConfigOptionsResult is the reply to session/set_config_option.
type ConfigOptionsResult struct {
	ConfigOptions []ConfigOption `json:"configOptions"`
}

// SetConfigOptionParams selects a value for one session config option.
type SetConfigOptionParams struct {
	SessionID string `json:"sessionId"`
	ConfigID  string `json:"configId"`
	Value     string `json:"value"`
}

// SetModeParams switches a session between the "ask" and "code" modes.
type SetModeParams struct {
	SessionID string `json:"sessionId"`
	ModeID    string `json:"modeId"`
}

// SessionIDParams is the payload of the session methods that take only an id.
type SessionIDParams struct {
	SessionID string `json:"sessionId"`
}

// SessionSummary is one entry of the session/list reply.
type SessionSummary struct {
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	UpdatedAt string `json:"updatedAt"`
}

// ListSessionsResult is the session/list reply.
type ListSessionsResult struct {
	Sessions []SessionSummary `json:"sessions"`
}

// PromptBlock is one content block of a prompt turn.
type PromptBlock struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	URI      string            `json:"uri,omitempty"`
	Name     string            `json:"name,omitempty"`
	Resource *EmbeddedResource `json:"resource,omitempty"`
}

// EmbeddedResource is inline file context attached to a prompt.
type EmbeddedResource struct {
	URI      string `json:"uri"`
	Text     string `json:"text,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
}

// TextBlock builds a plain text prompt block.
func TextBlock(text string) PromptBlock {
	return PromptBlock{Type: "text", Text: text}
}

// ResourceBlock builds an embedded context block carrying file contents.
func ResourceBlock(uri, text string) PromptBlock {
	return PromptBlock{Type: "resource", Resource: &EmbeddedResource{URI: uri, Text: text}}
}

// ResourceLinkBlock builds a link to a file the agent may read itself.
func ResourceLinkBlock(uri, name string) PromptBlock {
	return PromptBlock{Type: "resource_link", URI: uri, Name: name}
}

// PromptParams is the session/prompt payload.
type PromptParams struct {
	SessionID string        `json:"sessionId"`
	Prompt    []PromptBlock `json:"prompt"`
}

// PromptResult ends a turn. fx reports no token counts.
type PromptResult struct {
	StopReason string `json:"stopReason"`
}

// Stop reasons a prompt turn can end with.
const (
	StopEndTurn          = "end_turn"
	StopRefused          = "refused"
	StopCancelled        = "cancelled"
	StopMaxTurnRequests  = "max_turn_requests"
	StopMaxTokens        = "max_tokens"
	StopPermissionDenied = "permission_denied"
)

// SessionUpdateKind names a session/update notification variant.
type SessionUpdateKind string

// Session update kinds observed from fx plus the ones the ACP spec defines.
const (
	UpdateAgentMessageChunk SessionUpdateKind = "agent_message_chunk"
	UpdateAgentThoughtChunk SessionUpdateKind = "agent_thought_chunk"
	UpdateUserMessageChunk  SessionUpdateKind = "user_message_chunk"
	UpdateToolCall          SessionUpdateKind = "tool_call"
	UpdateToolCallUpdate    SessionUpdateKind = "tool_call_update"
	UpdateSessionInfo       SessionUpdateKind = "session_info_update"
	UpdateAvailableCommands SessionUpdateKind = "available_commands_update"
	UpdatePlan              SessionUpdateKind = "plan"
	UpdateCurrentMode       SessionUpdateKind = "current_mode_update"
	UpdateConfigOption      SessionUpdateKind = "config_option_update"
)

// SessionUpdate is a session/update notification.
type SessionUpdate struct {
	SessionID string             `json:"sessionId"`
	Update    SessionUpdateInner `json:"update"`
	Raw       json.RawMessage    `json:"-"`
}

// SessionUpdateInner is the payload of a session/update notification. Unknown
// kinds pass through with Kind set to the raw string and Raw retained.
type SessionUpdateInner struct {
	Kind              SessionUpdateKind `json:"sessionUpdate"`
	ToolCallID        string            `json:"toolCallId,omitempty"`
	Status            string            `json:"status,omitempty"`
	Title             string            `json:"title,omitempty"`
	ToolKind          string            `json:"kind,omitempty"`
	AvailableCommands []AgentCommand    `json:"availableCommands,omitempty"`
	CurrentModeID     string            `json:"currentModeId,omitempty"`
	Entries           []PlanEntry       `json:"entries,omitempty"`
	Meta              json.RawMessage   `json:"_meta,omitempty"`

	Content     *ContentBlock     `json:"-"`
	ToolContent []ToolCallContent `json:"-"`
	Recovery    *Recovery         `json:"-"`
	Raw         json.RawMessage   `json:"-"`
}

// UnmarshalJSON decodes an update, resolving the two shapes of "content" and
// the fx recovery meta.
func (u *SessionUpdateInner) UnmarshalJSON(data []byte) error {
	type alias SessionUpdateInner
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*u = SessionUpdateInner(tmp)
	u.Raw = append(json.RawMessage(nil), data...)
	var probe struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &probe); err == nil {
		u.decodeContent(probe.Content)
	}
	u.Recovery = decodeRecoveryMeta(u.Meta)
	return nil
}

func (u *SessionUpdateInner) decodeContent(raw json.RawMessage) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return
	}
	if trimmed[0] == '[' {
		var items []ToolCallContent
		if err := json.Unmarshal(trimmed, &items); err == nil {
			u.ToolContent = items
		}
		return
	}
	var block ContentBlock
	if err := json.Unmarshal(trimmed, &block); err == nil {
		u.Content = &block
	}
}

// Text returns the streamed text of a message or thought chunk.
func (u SessionUpdateInner) Text() string {
	if u.Content == nil {
		return ""
	}
	return u.Content.Text
}

// ToolText joins the text of every terminal tool result block.
func (u SessionUpdateInner) ToolText() string {
	var out bytes.Buffer
	for _, item := range u.ToolContent {
		if item.Content == nil {
			continue
		}
		out.WriteString(item.Content.Text)
	}
	return out.String()
}

// ContentBlock is a typed piece of streamed content.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ToolCallContent wraps one result block of a tool call update.
type ToolCallContent struct {
	Type    string        `json:"type"`
	Content *ContentBlock `json:"content,omitempty"`
}

// PlanEntry is one item of a plan update.
type PlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority,omitempty"`
	Status   string `json:"status,omitempty"`
}

// AgentCommand is a slash command the agent offers. These are not tools.
type AgentCommand struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Input       *AgentCommandInput `json:"input,omitempty"`
}

// AgentCommandInput is the input hint of a slash command.
type AgentCommandInput struct {
	Hint string `json:"hint,omitempty"`
}

// PermissionRequest is the session/request_permission payload sent by fx.
type PermissionRequest struct {
	SessionID string              `json:"sessionId"`
	ToolCall  *PermissionToolCall `json:"toolCall,omitempty"`
	Options   []PermissionOption  `json:"options"`

	Raw json.RawMessage `json:"-"`
}

// PermissionToolCall describes the tool call awaiting approval.
type PermissionToolCall struct {
	ToolCallID string `json:"toolCallId,omitempty"`
	Title      string `json:"title,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Status     string `json:"status,omitempty"`
}

// PermissionOption is one answer the client may select.
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// Permission option kinds defined by the ACP spec.
const (
	PermissionAllowOnce    = "allow_once"
	PermissionAllowAlways  = "allow_always"
	PermissionRejectOnce   = "reject_once"
	PermissionRejectAlways = "reject_always"
)

// PermissionOutcome is the client answer to a permission request.
type PermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

// Outcome values for a permission reply.
const (
	OutcomeSelected  = "selected"
	OutcomeCancelled = "cancelled"
)

// PermissionResponse wraps the outcome sent back to fx.
type PermissionResponse struct {
	Outcome PermissionOutcome `json:"outcome"`
}

func decodeRecoveryMeta(meta json.RawMessage) *Recovery {
	if len(bytes.TrimSpace(meta)) == 0 {
		return nil
	}
	var wrapper struct {
		FX struct {
			ModelResponseRecovery *Recovery `json:"modelResponseRecovery"`
		} `json:"fx"`
	}
	if err := json.Unmarshal(meta, &wrapper); err != nil {
		return nil
	}
	return wrapper.FX.ModelResponseRecovery
}
