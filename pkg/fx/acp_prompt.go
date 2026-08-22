package fx

import (
	"context"
	"strings"
)

// Initialize performs the ACP handshake and returns the agent capabilities.
func (s *ACPSession) Initialize(ctx context.Context, caps ClientCapabilities, info *ClientInfo) (*InitializeResult, error) {
	var result InitializeResult
	params := InitializeParams{ProtocolVersion: ACPProtocolVersion, ClientCapabilities: caps, ClientInfo: info}
	if err := s.call(ctx, "initialize", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// NewSession creates a session rooted at cwd.
func (s *ACPSession) NewSession(ctx context.Context, cwd string, mcp []MCPServerSpec) (*NewSessionResult, error) {
	if cwd == "" {
		return nil, validationError("session cwd must not be empty")
	}
	var result NewSessionResult
	if err := s.call(ctx, "session/new", NewSessionParams{CWD: cwd, MCPServers: orEmptyMCP(mcp)}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// LoadSession reopens a saved session and replays its history as updates.
func (s *ACPSession) LoadSession(ctx context.Context, sessionID, cwd string, mcp []MCPServerSpec) error {
	return s.loadOrResume(ctx, "session/load", sessionID, cwd, mcp)
}

// ResumeSession reopens a saved session without replaying its history.
func (s *ACPSession) ResumeSession(ctx context.Context, sessionID, cwd string, mcp []MCPServerSpec) error {
	return s.loadOrResume(ctx, "session/resume", sessionID, cwd, mcp)
}

func (s *ACPSession) loadOrResume(ctx context.Context, method, sessionID, cwd string, mcp []MCPServerSpec) error {
	if sessionID == "" {
		return validationError("session id must not be empty")
	}
	if cwd == "" {
		return validationError("session cwd must not be empty")
	}
	params := NewSessionParams{SessionID: sessionID, CWD: cwd, MCPServers: orEmptyMCP(mcp)}
	if err := s.call(ctx, method, params, nil); err != nil {
		return err
	}
	return nil
}

// ListSessions returns the saved sessions for the current workspace.
func (s *ACPSession) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	var result ListSessionsResult
	if err := s.call(ctx, "session/list", struct{}{}, &result); err != nil {
		return nil, err
	}
	return result.Sessions, nil
}

// CloseSession releases a session on the agent side.
func (s *ACPSession) CloseSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return validationError("session id must not be empty")
	}
	if err := s.call(ctx, "session/close", SessionIDParams{SessionID: sessionID}, nil); err != nil {
		return err
	}
	return nil
}

// SetModel switches the model of a session and returns the updated options.
func (s *ACPSession) SetModel(ctx context.Context, sessionID, model string) (*ConfigOptionsResult, error) {
	if sessionID == "" || model == "" {
		return nil, validationError("session id and model must not be empty")
	}
	var result ConfigOptionsResult
	params := SetConfigOptionParams{SessionID: sessionID, ConfigID: "model", Value: model}
	if err := s.call(ctx, "session/set_config_option", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetMode switches a session between "ask" and "code". The "code" mode bills a
// reviewer model for automatic permission review.
func (s *ACPSession) SetMode(ctx context.Context, sessionID, mode string) error {
	if mode != "ask" && mode != "code" {
		return validationError("session mode must be \"ask\" or \"code\"")
	}
	if err := s.call(ctx, "session/set_mode", SetModeParams{SessionID: sessionID, ModeID: mode}, nil); err != nil {
		return err
	}
	return nil
}

// Cancel asks the agent to stop the active turn. It is a notification, so the
// pending Prompt returns with stopReason "cancelled".
func (s *ACPSession) Cancel(sessionID string) error {
	return s.Notify("session/cancel", SessionIDParams{SessionID: sessionID})
}

// Prompt runs one turn and blocks until it ends. Drain Updates concurrently.
func (s *ACPSession) Prompt(ctx context.Context, sessionID string, blocks []PromptBlock) (*PromptResult, error) {
	if sessionID == "" {
		return nil, validationError("session id must not be empty")
	}
	if len(blocks) == 0 {
		return nil, validationError("prompt must have at least one block")
	}
	var result PromptResult
	if err := s.call(ctx, "session/prompt", PromptParams{SessionID: sessionID, Prompt: blocks}, &result); err != nil {
		if ctx.Err() != nil {
			_ = s.Cancel(sessionID)
		}
		return nil, err
	}
	return &result, nil
}

// PromptText runs one turn from a single text block.
func (s *ACPSession) PromptText(ctx context.Context, sessionID, text string) (*PromptResult, error) {
	if strings.TrimSpace(text) == "" {
		return nil, validationError("prompt text must not be empty")
	}
	return s.Prompt(ctx, sessionID, []PromptBlock{TextBlock(text)})
}

// ToolCallEvent is the aggregated lifecycle of one tool call in a turn.
type ToolCallEvent struct {
	ToolCallID string
	Title      string
	Kind       string
	Status     string
	Text       string
}

// PromptCollection is the aggregated result of a turn for callers that do not
// want to stream updates themselves.
type PromptCollection struct {
	Text       string
	Thoughts   string
	ToolCalls  []ToolCallEvent
	Recovery   *Recovery
	StopReason string
}

// CollectPrompt runs a turn and drains the update stream into one result. It
// consumes Updates, so do not use it alongside another reader.
func (s *ACPSession) CollectPrompt(ctx context.Context, sessionID string, blocks []PromptBlock) (*PromptCollection, error) {
	type outcome struct {
		result *PromptResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := s.Prompt(ctx, sessionID, blocks)
		done <- outcome{result, err}
	}()

	collected := &PromptCollection{}
	updates := s.updatesCh
	for {
		select {
		case update, ok := <-updates:
			if !ok {
				updates = nil
				continue
			}
			collected.absorb(update.Update)
		case finished := <-done:
			collected.drain(s.updatesCh)
			if finished.err != nil {
				return collected, finished.err
			}
			collected.StopReason = finished.result.StopReason
			return collected, nil
		}
	}
}

func (p *PromptCollection) drain(updates <-chan SessionUpdate) {
	for {
		select {
		case update, ok := <-updates:
			if !ok {
				return
			}
			p.absorb(update.Update)
		default:
			return
		}
	}
}

func (p *PromptCollection) absorb(update SessionUpdateInner) {
	switch update.Kind {
	case UpdateAgentMessageChunk:
		p.Text += update.Text()
	case UpdateAgentThoughtChunk:
		p.Thoughts += update.Text()
	case UpdateToolCall, UpdateToolCallUpdate:
		p.absorbToolCall(update)
	case UpdateSessionInfo:
		if update.Recovery != nil {
			p.Recovery = update.Recovery
		}
	}
}

func (p *PromptCollection) absorbToolCall(update SessionUpdateInner) {
	event := p.toolCallEvent(update.ToolCallID)
	if update.Title != "" {
		event.Title = update.Title
	}
	if update.ToolKind != "" {
		event.Kind = update.ToolKind
	}
	if update.Status != "" {
		event.Status = update.Status
	}
	event.Text += update.ToolText()
}

func (p *PromptCollection) toolCallEvent(id string) *ToolCallEvent {
	for i := range p.ToolCalls {
		if p.ToolCalls[i].ToolCallID == id {
			return &p.ToolCalls[i]
		}
	}
	p.ToolCalls = append(p.ToolCalls, ToolCallEvent{ToolCallID: id})
	return &p.ToolCalls[len(p.ToolCalls)-1]
}

func orEmptyMCP(mcp []MCPServerSpec) []MCPServerSpec {
	if mcp == nil {
		return []MCPServerSpec{}
	}
	return mcp
}
