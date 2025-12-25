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
    @echo "  test                      - Run all tests (unit + integration)"
    @echo "  test-unit                 - Run unit tests only"
    @echo "  test-integration          - Run integration tests only"
    @echo "  test-coverage             - Run all tests with coverage report"
    @echo "  test-integration-coverage - Run integration tests with coverage"
    @echo "  bench                     - Run benchmarks"
    @echo "  info           - Show this help message"

# Default target
default: build
