package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type acpServer struct {
	script    *script
	out       *bufio.Writer
	writeMu   sync.Mutex
	responses sync.Map
	nextID    int64
	cancelled bool
	cancelMu  sync.Mutex
	delay     time.Duration
}

func serveACP() int {
	name := os.Getenv("FX_MOCK_SCENARIO")
	if name == "" {
		name = "full-turn"
	}
	path := filepath.Join(testdataDir(), "acp", name+".jsonl")
	loaded, err := loadScript(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fx-mock: load acp script:", err)
		return 2
	}
	server := &acpServer{script: loaded, out: bufio.NewWriter(os.Stdout), nextID: 1000, delay: scriptDelay()}
	server.run()
	return exitOverride(0)
}

func scriptDelay() time.Duration {
	if v := os.Getenv("FX_MOCK_ACP_DELAY_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Millisecond
		}
	}
	return 0
}

func (s *acpServer) run() {
	incoming := make(chan rpcMsg, 32)
	go s.readStdin(incoming)
	for msg := range incoming {
		s.handle(msg)
	}
}

func (s *acpServer) readStdin(incoming chan<- rpcMsg) {
	defer close(incoming)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg rpcMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		msg.raw = copyBytes(line)
		if msg.isResponse() {
			s.deliver(msg)
			continue
		}
		incoming <- msg
	}
}

func (s *acpServer) deliver(msg rpcMsg) {
	value, ok := s.responses.LoadAndDelete(idKey(msg.ID))
	if !ok {
		return
	}
	ch, ok := value.(chan rpcMsg)
	if ok {
		ch <- msg
	}
}

func (s *acpServer) handle(msg rpcMsg) {
	if msg.Method == "session/cancel" {
		s.setCancelled(true)
		return
	}
	if !msg.isRequest() {
		return
	}
	group := s.script.next(msg.Method)
	if group == nil {
		s.writeLine(methodNotFound(msg))
		return
	}
	s.play(group, msg)
}

func (s *acpServer) play(group *scriptGroup, msg rpcMsg) {
	for _, entry := range group.pre {
		if s.isCancelled() && group.method == "session/prompt" {
			s.writeLine(withID(cancelledResult(), msg.ID))
			return
		}
		s.emit(entry)
		s.pause()
	}
	s.writeLine(withID(group.response, msg.ID))
	for _, entry := range group.post {
		s.pause()
		s.emit(entry)
	}
}

func (s *acpServer) emit(entry scriptEntry) {
	if !entry.agentRequest {
		s.writeLine(entry.raw)
		return
	}
	s.nextID++
	id := json.RawMessage(strconv.FormatInt(s.nextID, 10))
	ch := make(chan rpcMsg, 1)
	s.responses.Store(idKey(id), ch)
	s.writeLine(withID(entry.raw, id))
	select {
	case reply := <-ch:
		recordPermissionReply(reply)
	case <-time.After(permissionTimeout()):
		s.responses.Delete(idKey(id))
	}
}

func recordPermissionReply(reply rpcMsg) {
	path := os.Getenv("FX_MOCK_PERM_REPLY")
	if path == "" {
		return
	}
	body := reply.raw
	if len(body) == 0 {
		body = []byte("null")
	}
	_ = os.WriteFile(path, body, 0o644)
}

func permissionTimeout() time.Duration {
	if v := os.Getenv("FX_MOCK_PERM_TIMEOUT_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Millisecond
		}
	}
	return 30 * time.Second
}

func (s *acpServer) pause() {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
}

func (s *acpServer) writeLine(raw []byte) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.out.Write(raw)
	s.out.WriteByte('\n')
	s.out.Flush()
}

func (s *acpServer) setCancelled(v bool) {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	s.cancelled = v
}

func (s *acpServer) isCancelled() bool {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	return s.cancelled
}

func cancelledResult() []byte {
	return []byte(`{"jsonrpc":"2.0","id":0,"result":{"stopReason":"cancelled"}}`)
}

func methodNotFound(msg rpcMsg) []byte {
	out := map[string]any{
		"jsonrpc": "2.0",
		"id":      msg.ID,
		"error":   map[string]any{"code": -32601, "message": "Method not found: " + msg.Method},
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal"}}`)
	}
	return raw
}
