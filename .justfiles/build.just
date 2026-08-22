# Build Commands
# Run `just build` to see all build commands

PROJECT_DIR := justfile_directory()
BIN_DIR := PROJECT_DIR / "bin"

[private]
@default:
    @echo "Build Commands"
    @echo "=============="
    @just --list --justfile {{source_file()}}

# Build everything (library + dangerous + examples + mock)
all: lib dangerous examples mock

# Build core library
lib:
    @echo "Building core library..."
    cd {{PROJECT_DIR}} && go build ./pkg/fx
    @echo "Core library built"

# Build dangerous subpackage
dangerous:
    @echo "Building dangerous subpackage..."
    cd {{PROJECT_DIR}} && go build ./pkg/fx/dangerous
    @echo "Dangerous subpackage built"

# Build the mock fx binary used by unit tests
mock:
    @echo "Building mock fx binary..."
    @mkdir -p {{PROJECT_DIR}}/test/mockfx/bin
    cd {{PROJECT_DIR}} && go build -o test/mockfx/bin/fx-mock ./test/mockfx
    @echo "Built: test/mockfx/bin/fx-mock"

# Build all examples
examples: ask ask-json acp-stream sessions models usage
    @echo "All examples built"

# Build ask example
ask:
    @mkdir -p {{BIN_DIR}}
    cd {{PROJECT_DIR}} && go build -o {{BIN_DIR}}/ask ./examples/ask
    @echo "Built: {{BIN_DIR}}/ask"

# Build ask_json example
ask-json:
    @mkdir -p {{BIN_DIR}}
    cd {{PROJECT_DIR}} && go build -o {{BIN_DIR}}/ask_json ./examples/ask_json
    @echo "Built: {{BIN_DIR}}/ask_json"

# Build acp_stream example
acp-stream:
    @mkdir -p {{BIN_DIR}}
    cd {{PROJECT_DIR}} && go build -o {{BIN_DIR}}/acp_stream ./examples/acp_stream
    @echo "Built: {{BIN_DIR}}/acp_stream"

# Build sessions example
sessions:
    @mkdir -p {{BIN_DIR}}
    cd {{PROJECT_DIR}} && go build -o {{BIN_DIR}}/sessions ./examples/sessions
    @echo "Built: {{BIN_DIR}}/sessions"

# Build models example
models:
    @mkdir -p {{BIN_DIR}}
    cd {{PROJECT_DIR}} && go build -o {{BIN_DIR}}/models ./examples/models
    @echo "Built: {{BIN_DIR}}/models"

# Build usage example
usage:
    @mkdir -p {{BIN_DIR}}
    cd {{PROJECT_DIR}} && go build -o {{BIN_DIR}}/usage ./examples/usage
    @echo "Built: {{BIN_DIR}}/usage"
