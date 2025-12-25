# PipeWire Audio Compressor (pw-comp)

## Project Overview

`pw-comp` is a real-time audio dynamic range compressor for Linux, utilizing the PipeWire multimedia server. It features a hybrid architecture:
- **DSP Core (Go):** The audio processing logic (soft-knee compression, envelope following, makeup gain) is implemented in pure Go for safety and maintainability.
- **PipeWire Interface (C):** A thin C layer (`csrc/`) manages the interaction with the PipeWire API via CGO.

**Key Features:**
- Soft-knee compression curve.
- Attack/Release envelope followers.
- Automatic and manual makeup gain.
- **Stereo support via separate planar ports (FL, FR).**
- **Adaptable sample rate (negotiated automatically).**
- Low-latency real-time processing.

## Building and Running

### Prerequisites
- **Go:** Version 1.24 or later.
- **C Compiler:** `gcc` (required for CGO).
- **Libraries:** `libpipewire-0.3-dev` (and `libspa-0.2-dev` usually included).
- **Build Tool:** `just` (optional, but recommended).

### Commands

The project uses `just` as a task runner.

| Task | Command (`just`) | Manual Equivalent |
| :--- | :--- | :--- |
| **Build** | `just build` | `go generate && go build -o pw-comp` |
| **Run** | `just run` | `./pw-comp` |
| **Test (All)** | `just test` | `go test -v` |
| **Test (Unit)** | `just test-unit` | `go test -v -run Test[^I]` |
| **Test (Integ)** | `just test-integration` | `go test -v -run TestIntegration` |
| **Clean** | `just clean` | `rm -f pw-comp libpw_wrapper.so csrc/*.o` |

## Code Structure

### Go Components
- **`main.go`**: The application entry point. It parses command-line flags, initializes the `SoftKneeCompressor`, and starts the PipeWire main loop. It exports `process_channel_go`, which processes individual channel buffers.
- **`compressor.go`**: Contains the core DSP logic.
    - `SoftKneeCompressor`: Main struct holding parameters and state (peak followers).
    - `ProcessBlock()`: Efficiently processes a buffer for a specific channel.
    - `SetSampleRate()`: Dynamically updates time constants when the sample rate changes.

### C Components (`csrc/`)
- **`pw_wrapper.c` / `pw_wrapper.h`**: Handles the complexity of the PipeWire C API.
    - Creates separate mono ports (FL, FR) to ensure compatibility with standard audio nodes.
    - Uses `SPA_PARAM_EnumFormat` and `PW_KEY_FORMAT_DSP` hints to ensure ports appear as "Green" (Audio) in graph tools.
    - Detects sample rate from the graph clock and passes it to Go.

### Testing
- **`compressor_test.go`**: Unit tests for the DSP logic.
- **`integration_test.go`**: Tests the full pipeline (simulating audio buffers).
- **`test_signals.go` / `test_analysis.go`**: Helpers for generating and analyzing signals.

## Development Conventions

1.  **DSP Logic:** Keep all audio processing logic in Go (`compressor.go`).
2.  **CGO Interface:** The boundary uses `process_channel_go`.
3.  **Memory Management:** Processes C-allocated buffers using `unsafe.Slice`. No Go memory escapes to C.
4.  **Port Compatibility:** When adding ports, always include `SPA_FORMAT_AUDIO_position` and the `PW_KEY_FORMAT_DSP` property hint to ensure visibility in graph tools.