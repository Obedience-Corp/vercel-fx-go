# Mock Binary Commands
# Run `just mock` to see all mock binary commands
#
# The mock is a compiled binary, not a long-running server. It impersonates the
# `fx` CLI for unit tests: argv and env recording, fixture playback from
# test/testdata, configurable exit codes, and a scripted ACP conversation.

set dotenv-load

PROJECT_DIR := justfile_directory()
MOCK_DIR := PROJECT_DIR / "test/mockfx"
MOCK_BIN := MOCK_DIR / "bin/fx-mock"

[private]
@default:
    @echo "Mock Binary Commands"
    @echo "===================="
    @just --list --justfile {{ source_file() }}

# Build the mock fx binary
build:
    @echo "Building mock fx binary..."
    @mkdir -p {{ MOCK_DIR }}/bin
    cd {{ PROJECT_DIR }} && go build -o {{ MOCK_BIN }} ./test/mockfx
    @echo "Built: {{ MOCK_BIN }}"

# Rebuild (clean + build)
rebuild: clean build

# Print the path to the mock binary
path:
    @echo "{{ MOCK_BIN }}"

# Smoke test the mock against the captured ask-success fixture
smoke: build
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -x "{{ MOCK_BIN }}" ]; then
        echo "Mock binary missing at {{ MOCK_BIN }}"
        exit 1
    fi
    echo "Invoking mock with: ask --json -- ping"
    FX_MOCK_SCENARIO="ask-success" "{{ MOCK_BIN }}" ask --json -- ping

# Clean the mock binary
clean:
    @echo "Cleaning mock binary..."
    rm -rf {{ MOCK_DIR }}/bin
    @echo "Mock binary cleaned"

# Show captured testdata fixtures
fixtures:
    @echo "Captured fixtures under test/testdata/:"
    @find {{ PROJECT_DIR }}/test/testdata -type f 2>/dev/null || echo "  (none yet)"
