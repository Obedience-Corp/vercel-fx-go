package fx

import (
	"context"
	"encoding/json"
)

// PermissionHandler answers an fx session/request_permission request. It runs
// off the read pump, so it may block without stalling notifications.
type PermissionHandler func(ctx context.Context, req *PermissionRequest) (PermissionOutcome, error)

// DefaultPermissionHandler fails closed: it rejects the call once.
func DefaultPermissionHandler(_ context.Context, req *PermissionRequest) (PermissionOutcome, error) {
	return rejectOutcome(req), nil
}

// AllowOncePermissionHandler approves every request for a single use.
func AllowOncePermissionHandler(_ context.Context, req *PermissionRequest) (PermissionOutcome, error) {
	if opt, ok := req.OptionByKind(PermissionAllowOnce); ok {
		return PermissionOutcome{Outcome: OutcomeSelected, OptionID: opt.OptionID}, nil
	}
	return PermissionOutcome{Outcome: OutcomeCancelled}, nil
}

// OptionByKind finds the offered option with the given ACP option kind.
func (r *PermissionRequest) OptionByKind(kind string) (PermissionOption, bool) {
	if r == nil {
		return PermissionOption{}, false
	}
	for _, option := range r.Options {
		if option.Kind == kind {
			return option, true
		}
	}
	return PermissionOption{}, false
}

func rejectOutcome(req *PermissionRequest) PermissionOutcome {
	if opt, ok := req.OptionByKind(PermissionRejectOnce); ok {
		return PermissionOutcome{Outcome: OutcomeSelected, OptionID: opt.OptionID}
	}
	if opt, ok := req.OptionByKind(PermissionRejectAlways); ok {
		return PermissionOutcome{Outcome: OutcomeSelected, OptionID: opt.OptionID}
	}
	return PermissionOutcome{Outcome: OutcomeCancelled}
}

func (s *ACPSession) handleServerRequest(m *RPCMessage) {
	if m.Method != "session/request_permission" {
		s.respondError(m.ID, -32601, "Method not found: "+m.Method)
		return
	}
	req := decodePermissionRequest(m.Params)
	s.handlers.Add(1)
	go func() {
		defer s.handlers.Done()
		s.runPermissionHandler(m.ID, req)
	}()
}

func decodePermissionRequest(params json.RawMessage) *PermissionRequest {
	req := &PermissionRequest{Raw: append(json.RawMessage(nil), params...)}
	var decoded PermissionRequest
	if err := json.Unmarshal(params, &decoded); err != nil {
		return req
	}
	decoded.Raw = req.Raw
	return &decoded
}

func (s *ACPSession) runPermissionHandler(id json.RawMessage, req *PermissionRequest) {
	ctx, cancel := s.handlerContext()
	defer cancel()
	outcome, err := s.handler(ctx, req)
	if err != nil {
		s.sendErr(err)
		outcome = rejectOutcome(req)
	}
	result, marshalErr := json.Marshal(PermissionResponse{Outcome: outcome})
	if marshalErr != nil {
		s.sendErr(validationErrorWith("marshal permission outcome", marshalErr))
		s.respondError(id, -32603, "client could not encode the permission outcome")
		return
	}
	if writeErr := s.writeMessage(&RPCMessage{JSONRPC: "2.0", ID: id, Result: result}); writeErr != nil {
		s.sendErr(writeErr)
	}
}

func (s *ACPSession) handlerContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-s.closed:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func (s *ACPSession) respondError(id json.RawMessage, code int, message string) {
	resp := RPCMessage{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}}
	if err := s.writeMessage(&resp); err != nil {
		s.sendErr(err)
	}
}
