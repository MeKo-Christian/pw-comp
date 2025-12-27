# Install development dependencies (formatters and linters)
setup-deps:
    #!/bin/bash
    echo "Installing development dependencies..."

    # Install treefmt (required for formatting)
    command -v treefmt >/dev/null 2>&1 || { echo "Installing treefmt..."; curl -fsSL https://github.com/numtide/treefmt/releases/download/v2.1.1/treefmt_2.1.1_linux_amd64.tar.gz | sudo tar -C /usr/local/bin -xz treefmt; }

    # Install prettier (Node.js formatter)
    command -v prettier >/dev/null 2>&1 || { echo "Installing prettier..."; npm install -g prettier || echo "Prettier installation failed - npm not found."; }

    # Install gofumpt (Go formatter)
    command -v gofumpt >/dev/null 2>&1 || { echo "Installing gofumpt..."; go install mvdan.cc/gofumpt@latest; }

    # Install gci (Go import formatter)
    command -v gci >/dev/null 2>&1 || { echo "Installing gci..."; go install github.com/daixiang0/gci@latest; }

    # Install clang-format (C formatter)
    command -v clang-format >/dev/null 2>&1 || echo "WARNING: clang-format not found. Please install: apt-get install clang-format"

    # Install golangci-lint (Go linter)
    command -v golangci-lint >/dev/null 2>&1 || { echo "Installing golangci-lint..."; curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.2.1; }

    echo "Development dependencies installation complete!"
    echo "Note: Ensure $(go env GOPATH)/bin is in your PATH for Go-based tools"

# Format code using treefmt
fmt:
    treefmt --allow-missing-formatter

# Check if code is formatted
fmt-check:
    treefmt --allow-missing-formatter --fail-on-change

# Run linter
lint:
    golangci-lint run --config ./.golangci.toml --timeout 2m

# Run linter with auto-fix
lint-fix:
    golangci-lint run --config ./.golangci.toml --timeout 2m --fix

#################################
# Checks
#################################

# Run all checks
check: check-formatted lint test check-tidy

# Check if go.mod is tidy
check-tidy:
    ./scripts/error-on-diff.sh go mod tidy

# Check if code is formatted
check-formatted:
    ./scripts/error-on-diff.sh just fmt

#################################
# Build targets
#################################

# Build the C shared library
build-lib:
    gcc -shared -o libpw_wrapper.so -fPIC csrc/pw_wrapper.c \
        -I/usr/include/pipewire-0.3 \
        -I/usr/include/spa-0.2 \
        -lpipewire-0.3

# Build the Go binary
build: build-lib
    go build -o pw-comp

# Clean build artifacts
clean:
    rm -f pw-comp libpw_wrapper.so csrc/*.o csrc/*.so

# Run the compressor
run: build
    ./pw-comp

# Full rebuild (clean + build)
rebuild: clean build

# Run all tests (unit + integration)
test:
    go test -v

# Run unit tests only
test-unit:
    go test -v -run Test[^I]

# Run integration tests only
test-integration:
    go test -v -run TestIntegration

# Run tests with coverage
test-coverage:
    go test -cover -coverprofile=coverage.out
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report: coverage.html"

# Run integration tests with coverage
test-integration-coverage:
    go test -v -run TestIntegration -cover -coverprofile=integration_coverage.out
    go tool cover -html=integration_coverage.out -o integration_coverage.html
    @echo "Integration coverage report: integration_coverage.html"

# Run benchmarks
bench:
    go test -bench=. -benchmem

# Show build info
info:
    @echo "PipeWire Audio Compressor Build System"
    @echo "======================================="
    @echo "Targets:"
    @echo "  build          - Build the complete project"
    @echo "  build-lib      - Build only the C library"
    @echo "  clean          - Remove build artifacts"
    @echo "  run            - Build and run the compressor"
    @echo "  rebuild        - Clean and build from scratch"
    @echo ""
    @echo "Testing:"
    @echo "  test                      - Run all tests (unit + integration)"
    @echo "  test-unit                 - Run unit tests only"
    @echo "  test-integration          - Run integration tests only"
    @echo "  test-coverage             - Run all tests with coverage report"
    @echo "  test-integration-coverage - Run integration tests with coverage"
    @echo "  bench                     - Run benchmarks"
    @echo ""
    @echo "Code Quality:"
    @echo "  fmt            - Format code using treefmt"
    @echo "  lint           - Run golangci-lint"
    @echo "  lint-fix       - Run linter with auto-fix"
    @echo "  check          - Run all checks (format, lint, test, tidy)"
    @echo "  check-formatted - Check if code is formatted"
    @echo "  check-tidy     - Check if go.mod is tidy"
    @echo "  setup-deps     - Install development dependencies"
    @echo ""
    @echo "  info           - Show this help message"

# Default target
default: build
