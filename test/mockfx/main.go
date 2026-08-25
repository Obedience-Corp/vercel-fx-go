// Command fx-mock impersonates the fx CLI for the SDK tests.
//
// It records argv and the FX_* environment into $FX_MOCK_RECORD, selects a
// fixture with $FX_MOCK_SCENARIO, and serves a scripted ACP conversation for
// the acp subcommand.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	args := os.Args[1:]
	if err := writeRecord(args); err != nil {
		fmt.Fprintln(os.Stderr, "fx-mock: record:", err)
	}
	sleepIfRequested()
	os.Exit(route(args))
}

func sleepIfRequested() {
	v := os.Getenv("FX_MOCK_SLEEP_MS")
	if v == "" {
		return
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return
	}
	time.Sleep(time.Duration(n) * time.Millisecond)
}

// Record is the argv and environment capture the tests assert against.
type Record struct {
	Argv  []string          `json:"argv"`
	Env   map[string]string `json:"env"`
	Cwd   string            `json:"cwd"`
	Stdin string            `json:"stdin,omitempty"`
}

func writeRecord(args []string) error {
	path := os.Getenv("FX_MOCK_RECORD")
	if path == "" {
		return nil
	}
	cwd, _ := os.Getwd()
	record := Record{Argv: args, Env: fxEnv(), Cwd: cwd}
	if isAskFromStdin(args) {
		data, err := io.ReadAll(os.Stdin)
		if err == nil {
			record.Stdin = string(data)
		}
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func isAskFromStdin(args []string) bool {
	if len(args) == 0 || os.Getenv("FX_MOCK_READ_STDIN") != "1" {
		return false
	}
	for _, a := range args {
		if a == "acp" {
			return false
		}
	}
	return true
}

func fxEnv() map[string]string {
	out := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(key, "FX_") && !strings.HasPrefix(key, "FX_MOCK_") {
			out[key] = value
		}
	}
	return out
}

func route(args []string) int {
	if len(args) == 0 {
		return 0
	}
	command := firstSubcommand(args)
	if command == "acp" {
		return serveACP()
	}
	if command == "--version" || command == "-v" {
		fmt.Println("0.0.6")
		return 0
	}
	if command == "login" {
		return serveLogin()
	}
	return emitFixture(command)
}

func firstSubcommand(args []string) string {
	skipValue := map[string]bool{"--add-dir": true, "--context-limit": true}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if skipValue[arg] {
			i++
			continue
		}
		if arg == "--no-additional-dirs" {
			continue
		}
		return arg
	}
	return ""
}

func serveLogin() int {
	fmt.Fprintln(os.Stderr, "Open this URL to authorize fx:")
	fmt.Fprintln(os.Stderr, "https://vercel.com/oauth/device?user_code=MOCK-CODE")
	if tailBytes, err := strconv.Atoi(os.Getenv("FX_MOCK_LOGIN_TAIL_BYTES")); err == nil && tailBytes > 0 {
		fmt.Fprintln(os.Stderr, strings.Repeat("x", tailBytes))
	}
	if os.Getenv("FX_MOCK_LOGIN_HANG") == "1" {
		select {}
	}
	return 0
}

type scenario struct {
	fixture  string
	exitCode int
}

var scenarios = map[string]scenario{
	"ask-success":         {"ask-success.json", 0},
	"ask-tool-write":      {"ask-tool-write.json", 0},
	"ask-503-paused":      {"ask-503-paused.json", 1},
	"ask-model-not-found": {"ask-model-not-found.json", 1},
	"status":              {"status.json", 0},
	"doctor":              {"doctor.json", 0},
	"models":              {"models.json", 0},
	"permissions":         {"permissions.json", 0},
	"credits":             {"credits.json", 0},
	"usage":               {"usage.json", 0},
	"sessions":            {"sessions.json", 0},
	"session-detail":      {"session-detail.json", 0},
	"session-missing":     {"session-missing.json", 1},
	"background":          {"background.json", 0},
	"workspace":           {"workspace.json", 0},
}

var commandDefaults = map[string]string{
	"ask":         "ask-success",
	"status":      "status",
	"doctor":      "doctor",
	"models":      "models",
	"permissions": "permissions",
	"credits":     "credits",
	"usage":       "usage",
	"sessions":    "sessions",
	"session":     "session-detail",
	"background":  "background",
	"workspace":   "workspace",
}

func emitFixture(command string) int {
	name := os.Getenv("FX_MOCK_SCENARIO")
	if name == "" {
		name = commandDefaults[command]
	}
	switch name {
	case "not-json":
		fmt.Println("fx: --json is not supported by this build")
		return exitOverride(1)
	case "empty":
		return exitOverride(1)
	}
	selected, ok := scenarios[name]
	if !ok {
		fmt.Fprintln(os.Stderr, "fx-mock: unknown scenario", name)
		return 2
	}
	data, err := os.ReadFile(filepath.Join(testdataDir(), selected.fixture))
	if err != nil {
		fmt.Fprintln(os.Stderr, "fx-mock: read fixture:", err)
		return 2
	}
	if stderr := os.Getenv("FX_MOCK_STDERR"); stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}
	os.Stdout.Write(data)
	return exitOverride(selected.exitCode)
}

func exitOverride(code int) int {
	if v := os.Getenv("FX_MOCK_EXIT_CODE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return code
}

func testdataDir() string {
	if dir := os.Getenv("FX_MOCK_TESTDATA"); dir != "" {
		return dir
	}
	exe, err := os.Executable()
	if err != nil {
		return "test/testdata"
	}
	return filepath.Join(filepath.Dir(exe), "..", "..", "testdata")
}
