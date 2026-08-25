# Test Commands
# Run `just test` to see all test commands

set dotenv-load

PROJECT_DIR := justfile_directory()

[private]
@default:
    @echo "Test Commands"
    @echo "============="
    @just --list --justfile {{ source_file() }}

# Run all unit tests (library + dangerous subpackage)
all: lib dangerous

# Run the complete local gate in an isolated Go container
container GO_VERSION="1.26":
    #!/usr/bin/env bash
    set -euo pipefail
    docker run --rm \
        -e GOFLAGS=-buildvcs=false \
        -v "{{ PROJECT_DIR }}:/src" \
        -w /src \
        "golang:{{ GO_VERSION }}-bookworm" \
        sh -c 'set -eu; test -z "$(gofmt -l .)"; go vet ./...; go test ./...; go test -race ./pkg/fx/...; go build -o /tmp/fx-mock ./test/mockfx; FX_MOCK_BIN=/tmp/fx-mock go test -tags=integration ./test/integration; go build ./examples/...'

# Run core library tests
lib:
    @echo "Testing core library..."
    cd {{ PROJECT_DIR }} && go test ./pkg/fx
    @echo "Core library tests complete"

# Run dangerous package tests
dangerous:
    @echo "Testing dangerous package..."
    cd {{ PROJECT_DIR }} && go test ./pkg/fx/dangerous
    @echo "Dangerous package tests complete"

# Run integration tests against the mock fx binary
integration:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Running integration tests against mock binary..."
    just mock build
    cd {{ PROJECT_DIR }}
    FX_MOCK_BIN="{{ PROJECT_DIR }}/test/mockfx/bin/fx-mock" \
    INTEGRATION_TESTS=1 USE_MOCK_BINARY=1 \
        go test -tags=integration ./test/integration -v
    echo "Mock-binary integration tests complete"

# Run integration tests against the real fx CLI
integration-real:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Running real integration tests..."
    echo "Requires fx installed and authenticated"
    cd {{ PROJECT_DIR }}
    INTEGRATION_REAL=1 INTEGRATION_TESTS=1 \
        go test -tags=integration ./test/integration -v -timeout 20m
    echo "Real integration tests complete"

# Run tests with race detection
race:
    @echo "Running tests with race detection..."
    cd {{ PROJECT_DIR }} && go test -race ./pkg/fx/...
    @echo "Race detection tests complete"

# Run tests verbosely
verbose:
    cd {{ PROJECT_DIR }} && go test -v ./pkg/fx/...

# Run tests for a specific package
[no-exit-message]
pkg package:
    @echo "Testing package: {{ package }}"
    cd {{ PROJECT_DIR }} && go test -v ./{{ package }}

# Verify examples compile
examples:
    @echo "Compiling examples (build-only)..."
    cd {{ PROJECT_DIR }} && go build ./examples/...
    @echo "All examples compile"
