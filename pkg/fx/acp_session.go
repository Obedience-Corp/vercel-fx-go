package fx

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
)

// ACPConfig configures the "fx acp" server process.
type ACPConfig struct {
	Model            string
	LogFile          string
	AddDirs          []string
	NoAdditionalDirs bool
	ContextLimits    map[string]string

	// PermissionMode sets FX_PERMISSION_MODE for the acp process. In "ask" mode,
	// fx sends requests to PermissionHandler. "yolo" requires AllowDangerousMode.
	PermissionMode     PermissionMode
	AllowDangerousMode bool
	MaxAgentSteps      *int
	WorkingDirectory   string
	Env                []string
	ClientName         string
	ClientVersion      string
	ClientCapabilities ClientCapabilities
	PermissionHandler  PermissionHandler
}

func (c *ACPConfig) validate() *Error {
	if c == nil {
		return nil
	}
	if !c.AllowDangerousMode && c.PermissionMode == PermissionYolo {
		return validationError("PermissionMode \"yolo\" requires AllowDangerousMode; use the dangerous subpackage")
	}
	if err := validatePermissionMode(c.PermissionMode); err != nil {
		return err
	}
	if c.LogFile != "" && !filepath.IsAbs(c.LogFile) {
		return validationError("LogFile must be an absolute path")
	}
	return validateContextLimits(c.ContextLimits)
}

// ACPSession is a live "fx acp" connection speaking JSON-RPC over stdio.
type ACPSession struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	handler PermissionHandler

	writeMu sync.Mutex
	nextID  atomic.Int64
	pending sync.Map

	updatesCh chan SessionUpdate
	notifCh   chan RPCMessage
	errsCh    chan error

	emitMu     sync.RWMutex
	emitClosed bool

	closed     chan struct{}
	readDone   chan struct{}
	stderrDone chan struct{}
	waitDone   chan error

	stderrMu  sync.Mutex
	stderrBuf []byte

	closeOnce   sync.Once
	closedOnce  sync.Once
	streamsOnce sync.Once
	closeErr    error
}

// StartACP spawns "fx acp" and starts the read pump. Cancelling ctx kills the
// process; call Close to stop it gracefully.
func (c *Client) StartACP(ctx context.Context, cfg *ACPConfig) (*ACPSession, error) {
	session, err := c.startACP(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (c *Client) startACP(ctx context.Context, cfg *ACPConfig) (*ACPSession, *Error) {
	if cfg == nil {
		cfg = &ACPConfig{}
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, &Error{Kind: KindInterrupted, Message: "context done before fx acp", Original: ctxErr}
	}
	dir, dirErr := c.workDir(cfg.WorkingDirectory)
	if dirErr != nil {
		return nil, dirErr
	}
	cmd := c.command(ctx, BuildACPArgs(cfg)...)
	cmd.Dir = dir
	cmd.Env = c.envWith(managedEnv(cfg.Model, cfg.PermissionMode, cfg.MaxAgentSteps, cfg.Env))
	return startACPSession(cmd, cfg)
}

func startACPSession(cmd *exec.Cmd, cfg *ACPConfig) (*ACPSession, *Error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, transportError("open fx acp stdin", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, transportError("open fx acp stdout", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, transportError("open fx acp stderr", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, transportError("start fx acp", err)
	}
	s := newACPSession(cmd, stdin, stdout, cfg)
	go s.readLoop()
	go s.drainStderr(stderr)
	go func() { s.waitDone <- cmd.Wait() }()
	return s, nil
}

func newACPSession(cmd *exec.Cmd, stdin io.WriteCloser, stdout io.ReadCloser, cfg *ACPConfig) *ACPSession {
	handler := cfg.PermissionHandler
	if handler == nil {
		handler = DefaultPermissionHandler
	}
	return &ACPSession{
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		handler:    handler,
		updatesCh:  make(chan SessionUpdate, 64),
		notifCh:    make(chan RPCMessage, 32),
		errsCh:     make(chan error, 8),
		closed:     make(chan struct{}),
		readDone:   make(chan struct{}),
		stderrDone: make(chan struct{}),
		waitDone:   make(chan error, 1),
	}
}

// Updates streams session/update notifications. The channel is buffered to 64
// and drops rather than blocks the read pump, so drain it while a prompt runs.
func (s *ACPSession) Updates() <-chan SessionUpdate { return s.updatesCh }

// Notifications streams notifications other than session/update.
func (s *ACPSession) Notifications() <-chan RPCMessage { return s.notifCh }

// Errors streams decode and transport problems seen by the read pump.
func (s *ACPSession) Errors() <-chan error { return s.errsCh }

// Done is closed when the connection ends.
func (s *ACPSession) Done() <-chan struct{} { return s.closed }

// PID is the process id of the fx acp server, or zero.
func (s *ACPSession) PID() int {
	if s.cmd == nil || s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

// Stderr returns the diagnostics fx wrote to stderr so far.
func (s *ACPSession) Stderr() string {
	s.stderrMu.Lock()
	defer s.stderrMu.Unlock()
	return string(s.stderrBuf)
}

// Call sends a JSON-RPC request and waits for its response.
func (s *ACPSession) Call(ctx context.Context, method string, params, result any) error {
	if err := s.call(ctx, method, params, result); err != nil {
		return err
	}
	return nil
}

func (s *ACPSession) call(ctx context.Context, method string, params, result any) *Error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return &Error{Kind: KindInterrupted, Message: "context done before " + method, Original: ctxErr}
	}
	id := s.nextID.Add(1)
	idStr := strconv.FormatInt(id, 10)
	ch := make(chan *RPCMessage, 1)
	s.pending.Store(idStr, ch)
	defer s.pending.Delete(idStr)

	paramsRaw, marshalErr := marshalParams(params)
	if marshalErr != nil {
		return marshalErr
	}
	if err := s.writeMessage(&RPCMessage{JSONRPC: "2.0", ID: json.RawMessage(idStr), Method: method, Params: paramsRaw}); err != nil {
		return err
	}
	return s.awaitResponse(ctx, method, ch, result)
}

func (s *ACPSession) awaitResponse(ctx context.Context, method string, ch chan *RPCMessage, result any) *Error {
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return &Error{Kind: KindProcess, Message: method + ": " + resp.Error.Message, Original: resp.Error}
		}
		if result == nil || len(resp.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return validationErrorWith("decode result for "+method, err)
		}
		return nil
	case <-ctx.Done():
		return &Error{Kind: KindInterrupted, Message: method + " canceled", Original: ctx.Err()}
	case <-s.closed:
		return transportError("fx acp closed before "+method+" responded", nil)
	}
}

// Notify sends a JSON-RPC notification and does not wait for a reply.
func (s *ACPSession) Notify(method string, params any) error {
	paramsRaw, marshalErr := marshalParams(params)
	if marshalErr != nil {
		return marshalErr
	}
	if err := s.writeMessage(&RPCMessage{JSONRPC: "2.0", Method: method, Params: paramsRaw}); err != nil {
		return err
	}
	return nil
}

func marshalParams(params any) (json.RawMessage, *Error) {
	if params == nil {
		return nil, nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, validationErrorWith("marshal params", err)
	}
	return b, nil
}

func (s *ACPSession) writeMessage(m *RPCMessage) *Error {
	b, err := json.Marshal(m)
	if err != nil {
		return validationErrorWith("marshal rpc message", err)
	}
	b = append(b, '\n')
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.stdin.Write(b); err != nil {
		return transportError("write to fx acp stdin", err)
	}
	return nil
}

func (s *ACPSession) readLoop() {
	defer func() {
		s.signalClosed()
		s.closeStreams()
		close(s.readDone)
	}()
	sc := bufio.NewScanner(s.stdout)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		if len(line) == 0 {
			continue
		}
		s.dispatch(line)
	}
	if err := sc.Err(); err != nil && !s.isClosed() {
		s.sendErr(transportError("read fx acp stdout", err))
	}
}

func (s *ACPSession) dispatch(line []byte) {
	var m RPCMessage
	if err := json.Unmarshal(line, &m); err != nil {
		s.sendErr(validationErrorWith("decode rpc message: "+truncate(line, 200), err))
		return
	}
	m.Raw = line
	switch {
	case m.IsResponse():
		s.deliverResponse(&m)
	case m.IsRequest():
		s.handleServerRequest(&m)
	case m.IsNotification():
		s.handleNotification(&m)
	}
}

func (s *ACPSession) deliverResponse(m *RPCMessage) {
	idStr := normalizeID(m.ID)
	if idStr == "" {
		return
	}
	v, ok := s.pending.LoadAndDelete(idStr)
	if !ok {
		return
	}
	ch, ok := v.(chan *RPCMessage)
	if !ok {
		return
	}
	select {
	case ch <- m:
	default:
	}
}

func normalizeID(raw json.RawMessage) string {
	var idStr string
	if err := json.Unmarshal(raw, &idStr); err == nil {
		return idStr
	}
	var idNum int64
	if err := json.Unmarshal(raw, &idNum); err == nil {
		return strconv.FormatInt(idNum, 10)
	}
	return ""
}

func (s *ACPSession) handleNotification(m *RPCMessage) {
	if m.Method == "session/update" && len(m.Params) > 0 {
		var su SessionUpdate
		if err := json.Unmarshal(m.Params, &su); err == nil {
			su.Raw = m.Params
			s.sendUpdate(su)
			return
		}
	}
	s.sendNotification(*m)
}

func (s *ACPSession) drainStderr(stderr io.ReadCloser) {
	defer close(s.stderrDone)
	buf := make([]byte, 4096)
	for {
		n, err := stderr.Read(buf)
		if n > 0 {
			s.appendStderr(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

const maxStderrBytes = 64 * 1024

func (s *ACPSession) appendStderr(chunk []byte) {
	s.stderrMu.Lock()
	defer s.stderrMu.Unlock()
	s.stderrBuf = append(s.stderrBuf, chunk...)
	if len(s.stderrBuf) > maxStderrBytes {
		s.stderrBuf = s.stderrBuf[len(s.stderrBuf)-maxStderrBytes:]
	}
}

func (s *ACPSession) sendUpdate(su SessionUpdate) {
	s.emitMu.RLock()
	defer s.emitMu.RUnlock()
	if s.emitClosed {
		return
	}
	select {
	case s.updatesCh <- su:
	case <-s.closed:
	default:
	}
}

func (s *ACPSession) sendNotification(m RPCMessage) {
	s.emitMu.RLock()
	defer s.emitMu.RUnlock()
	if s.emitClosed {
		return
	}
	select {
	case s.notifCh <- m:
	default:
	}
}

func (s *ACPSession) sendErr(err error) {
	s.emitMu.RLock()
	defer s.emitMu.RUnlock()
	if s.emitClosed {
		return
	}
	select {
	case s.errsCh <- err:
	default:
	}
}

func (s *ACPSession) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *ACPSession) signalClosed() {
	s.closedOnce.Do(func() { close(s.closed) })
}

func (s *ACPSession) closeStreams() {
	s.streamsOnce.Do(func() {
		s.emitMu.Lock()
		s.emitClosed = true
		s.emitMu.Unlock()
		close(s.updatesCh)
		close(s.notifCh)
		close(s.errsCh)
	})
}

// Close stops the fx acp process with SIGTERM, then SIGKILL after five seconds.
func (s *ACPSession) Close() error {
	s.closeOnce.Do(func() {
		s.signalClosed()
		_ = s.stdin.Close()
		s.closeErr = stopGracefully(s.cmd, s.waitDone)
		<-s.readDone
		<-s.stderrDone
	})
	return s.closeErr
}
