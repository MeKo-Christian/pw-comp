# PipeWire Audio Compressor (pw-comp)

A real-time audio dynamic range compressor implemented in Go, using PipeWire for audio I/O.

## Overview

This project implements a PipeWire filter node that performs dynamic range compression on audio streams. The DSP processing is written in Go for maintainability, while PipeWire integration is handled through C bindings (cgo).

**Status**: Core functionality is implemented and working. The compressor features professional-grade soft-knee compression with attack/release envelopes and automatic makeup gain. Ready for testing with real audio sources.

## Architecture

- **[csrc/pw_wrapper.c](csrc/pw_wrapper.c)** - C wrapper for PipeWire API, handles stream creation and audio callbacks
- **[csrc/pw_wrapper.h](csrc/pw_wrapper.h)** - Header file with function declarations and type definitions
- **[main.go](main.go)** - Go implementation of the compressor DSP algorithm and main event loop
- **libpw_wrapper.so** - Compiled shared library (generated)

### How It Works

1. PipeWire creates an audio stream configured as a filter node with separate ports for each channel (e.g., FL, FR).
2. Audio buffers arrive via the `on_process` callback in C, which processes each channel individually.
3. The callback invokes `process_channel_go` which processes samples through the compressor DSP.
4. The compressor dynamically adapts its internal time constants to the sample rate negotiated by PipeWire.
5. Compressed audio is queued back to PipeWire's output.

## Current Status

### Implemented

- [x] PipeWire stream initialization and event registration
- [x] Audio buffer processing loop with real-time callbacks
- [x] Professional soft-knee compressor with:
  - [x] Attack/release envelope follower
  - [x] Soft-knee compression curve
  - [x] Automatic makeup gain calculation
  - [x] Threshold, ratio, knee, attack, release controls
- [x] Stereo audio support via separate planar ports (FL, FR)
- [x] Adaptable sample rate support (automatically negotiated)
- [x] Bidirectional I/O (filter node)
- [x] Command-line parameter configuration
- [x] Build system (justfile)
- [x] Comprehensive test suite

### TODO

- [ ] Add visual feedback (level meters, gain reduction display)
- [ ] Implement sidechain filtering
- [ ] Add preset management
- [ ] Test with various audio sources in production environments

## Compressor Parameters

All parameters can be configured via command-line flags (see Usage section):

- **Threshold**: Compression threshold in dB (default: -20 dB)
- **Ratio**: Compression ratio (default: 4:1)
- **Knee**: Soft knee width in dB (default: 6 dB)
- **Attack**: Attack time in milliseconds (default: 10 ms)
- **Release**: Release time in milliseconds (default: 100 ms)
- **Makeup Gain**: Manual makeup gain in dB, or auto (default: auto)
- **Channels**: 2 (Exposed as separate `FL` and `FR` green ports)
- **Sample Rate**: Adaptable (Negotiated by PipeWire, compressor updates automatically)

## Building

Using the justfile (recommended):

```bash
# Build everything (C library + Go binary)
just build

# Run tests
just test

# Clean build artifacts
just clean
```

Manual compilation:

```bash
# Generate the C shared library
go generate

# Build the Go binary
go build -o pw-comp
```

## Dependencies

- PipeWire development libraries (`libpipewire-0.3-dev`)
- Go 1.24 or later
- GCC
- [just](https://github.com/casey/just) (optional, for build automation)

### Ubuntu/Debian

```bash
sudo apt-get install libpipewire-0.3-dev
```

## Usage

Run with default settings:

```bash
./pw-comp
```

Run with custom parameters:

```bash
./pw-comp -threshold -30 -ratio 8 -attack 5 -release 200
```

Show all available options:

```bash
./pw-comp -help
```

### Available Command-Line Options

- `-threshold` - Compression threshold in dB (default: -20.0)
- `-ratio` - Compression ratio, e.g., 4.0 for 4:1 (default: 4.0)
- `-knee` - Soft knee width in dB (default: 6.0)
- `-attack` - Attack time in milliseconds (default: 10.0)
- `-release` - Release time in milliseconds (default: 100.0)
- `-makeup` - Manual makeup gain in dB, 0 = auto (default: 0.0)
- `-auto-makeup` - Enable automatic makeup gain (default: true)
- `-help` - Show help message

The filter will appear as "Compressor" in PipeWire's audio graph and can be connected using tools like `pw-link` or `qpwgraph`.

## Testing

The project includes a comprehensive test suite covering both unit tests and integration tests.

### Test Organization

- **Unit Tests** ([compressor_test.go](compressor_test.go)) - Test the core DSP compression algorithm in isolation:
  - Compressor initialization and parameter configuration
  - Decibel conversions and coefficient calculations
  - Soft-knee curve behavior
  - Peak detection envelope follower
  - Attack/release time constants
  - Automatic makeup gain computation
  - Channel independence

- **Integration Tests** ([integration_test.go](integration_test.go)) - Test the full signal path from C boundary through compression:
  - Buffer handling and interleaved stereo processing
  - Compression behavior with realistic signals
  - Stereo channel independence and phase coherence
  - Dynamic response (attack/release envelopes)
  - Edge cases (clipping prevention, parameter changes, various buffer sizes)

- **Test Infrastructure** ([test_signals.go](test_signals.go), [test_analysis.go](test_analysis.go)) - Signal generation and analysis utilities

### Running Tests

Run all tests (unit + integration):
```bash
just test
```

Run only unit tests:
```bash
just test-unit
```

Run only integration tests:
```bash
just test-integration
```

Run tests with coverage report:
```bash
just test-coverage
```

Run integration tests with coverage:
```bash
just test-integration-coverage
```

Run benchmarks:
```bash
just bench
```

### CI/CD Compatibility

The test suite is designed to run in any CI/CD environment:
- ✅ No PipeWire daemon required for tests
- ✅ No audio hardware needed
- ✅ Fast execution (< 1 second total)
- ✅ Deterministic results with synthetic test signals
- ✅ Zero external dependencies beyond Go toolchain

Integration tests use mock signal generation to validate the complete processing pipeline without requiring actual PipeWire infrastructure, making them ideal for automated testing environments.

## License

(License to be added)
