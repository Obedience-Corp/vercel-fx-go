package main

import (
	"bufio"
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`

	raw []byte
}

func (m rpcMsg) isRequest() bool      { return m.Method != "" && len(m.ID) > 0 }
func (m rpcMsg) isNotification() bool { return m.Method != "" && len(m.ID) == 0 }
func (m rpcMsg) isResponse() bool     { return m.Method == "" && len(m.ID) > 0 }

func idKey(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return strconv.FormatInt(n, 10)
	}
	return ""
}

var clientMethods = map[string]bool{
	"initialize":                true,
	"authenticate":              true,
	"session/new":               true,
	"session/load":              true,
	"session/resume":            true,
	"session/list":              true,
	"session/close":             true,
	"session/prompt":            true,
	"session/set_config_option": true,
	"session/set_mode":          true,
}

func isAgentRequestMethod(method string) bool {
	return method == "session/request_permission" ||
		strings.HasPrefix(method, "fs/") ||
		strings.HasPrefix(method, "terminal/")
}

type scriptEntry struct {
	raw          []byte
	agentRequest bool
}

type scriptGroup struct {
	method   string
	pre      []scriptEntry
	response []byte
	post     []scriptEntry
}

type script struct {
	groups map[string][]*scriptGroup
}

func (s *script) next(method string) *scriptGroup {
	queue := s.groups[method]
	if len(queue) == 0 {
		return nil
	}
	s.groups[method] = queue[1:]
	return queue[0]
}

func loadScript(path string) (*script, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := &script{groups: map[string][]*scriptGroup{}}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	parser := &scriptParser{out: out}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parser.consume([]byte(line))
	}
	return out, scanner.Err()
}

type scriptParser struct {
	out     *script
	current *scriptGroup
	last    *scriptGroup
	waiting string
}

func (p *scriptParser) consume(line []byte) {
	var msg rpcMsg
	if err := json.Unmarshal(line, &msg); err != nil {
		return
	}
	switch {
	case msg.isRequest() && clientMethods[msg.Method]:
		p.startGroup(msg)
	case msg.isRequest() && isAgentRequestMethod(msg.Method):
		p.appendEntry(scriptEntry{raw: copyBytes(line), agentRequest: true})
	case msg.isResponse():
		p.finishGroup(msg, line)
	case msg.isNotification():
		p.appendEntry(scriptEntry{raw: copyBytes(line)})
	}
}

func (p *scriptParser) startGroup(msg rpcMsg) {
	p.current = &scriptGroup{method: msg.Method}
	p.waiting = idKey(msg.ID)
	p.last = nil
}

func (p *scriptParser) finishGroup(msg rpcMsg, line []byte) {
	if p.current == nil || idKey(msg.ID) != p.waiting {
		return
	}
	p.current.response = copyBytes(line)
	p.out.groups[p.current.method] = append(p.out.groups[p.current.method], p.current)
	p.last = p.current
	p.current = nil
	p.waiting = ""
}

func (p *scriptParser) appendEntry(entry scriptEntry) {
	switch {
	case p.current != nil:
		p.current.pre = append(p.current.pre, entry)
	case p.last != nil:
		p.last.post = append(p.last.post, entry)
	}
}

func copyBytes(b []byte) []byte {
	return append([]byte(nil), b...)
}

func withID(raw []byte, id json.RawMessage) []byte {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return raw
	}
	object["id"] = id
	out, err := json.Marshal(object)
	if err != nil {
		return raw
	}
	return out
}
