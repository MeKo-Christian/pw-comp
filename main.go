//go:generate sh -c "gcc -shared -o libpw_wrapper.so -fPIC csrc/pw_wrapper.c -I/usr/include/pipewire-0.3 -I/usr/include/spa-0.2 -lpipewire-0.3"

package main

/*
#cgo CFLAGS: -I./csrc -I/usr/include/pipewire-0.3 -I/usr/include/spa-0.2
#cgo LDFLAGS: -L. -lpw_wrapper -lpipewire-0.3

#include <pipewire/pipewire.h>
#include <spa/param/audio/format-utils.h>
#include <spa/param/audio/format.h>
#include <spa/param/format-utils.h>
#include <spa/utils/type.h>
#include <spa/pod/builder.h>
#include <spa/pod/pod.h>
#include <spa/pod/parser.h>
#include <spa/pod/vararg.h>
#include "pw_wrapper.h"
*/
import "C"

import (
	"flag"
	"fmt"
	"os"
	"unsafe"
	"time"
	"sync"
)

// Audio configuration
var (
	channels   = 2     // Stereo (modify for 5.1, etc.)
	sampleRate = 48000 // Default sample rate, will be updated by PipeWire
)

// Compressor instance
var compressor *SoftKneeCompressor

// processAudioBuffer processes an INTERLEAVED audio buffer through the compressor (Go wrapper for tests)
func processAudioBuffer(audio []float32) {
	if compressor == nil {
		return
	}

	if len(audio)%channels != 0 {
		return
	}

	samplesPerChannel := len(audio) / channels

	for i := 0; i < samplesPerChannel; i++ {
		for ch := 0; ch < channels; ch++ {
			index := i*channels + ch
			audio[index] = compressor.ProcessSample(audio[index], ch)
		}
	}
}

//export process_channel_go
func process_channel_go(in *C.float, out *C.float, samples C.int, rate C.int, channelIndex C.int) {
	if compressor == nil {
		return
	}

	// Update sample rate if changed
	if rate > 0 {
		compressor.SetSampleRate(float64(rate))
	}

	// Convert C arrays to Go slices
	inBuf := unsafe.Slice((*float32)(unsafe.Pointer(in)), int(samples))
	outBuf := unsafe.Slice((*float32)(unsafe.Pointer(out)), int(samples))

	// Process the block for this specific channel
	compressor.ProcessBlock(inBuf, outBuf, int(channelIndex))
}

func main() {
	// Command-line flags for compressor parameters
	threshold := flag.Float64("threshold", -20.0, "Compression threshold in dB")
	ratio := flag.Float64("ratio", 4.0, "Compression ratio (e.g., 4.0 for 4:1)")
	knee := flag.Float64("knee", 6.0, "Soft knee width in dB")
	attack := flag.Float64("attack", 10.0, "Attack time in milliseconds")
	release := flag.Float64("release", 100.0, "Release time in milliseconds")
	makeupGain := flag.Float64("makeup", 0.0, "Manual makeup gain in dB (0 = auto)")
	autoMakeup := flag.Bool("auto-makeup", true, "Enable automatic makeup gain")
	noTUI := flag.Bool("no-tui", false, "Disable interactive TUI")
	showHelp := flag.Bool("help", false, "Show this help message")

	flag.Parse()

	if *showHelp {
		fmt.Println("PipeWire Audio Compressor (pw-comp)")
		fmt.Println("===================================")
		fmt.Println("\nA real-time audio dynamic range compressor for PipeWire.")
		fmt.Println("\nUsage: pw-comp [options]")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		os.Exit(0)
	}

	// Initialize compressor with default settings
	compressor = NewSoftKneeCompressor(float64(sampleRate), channels)

	// Configure compressor parameters from command-line flags
	compressor.SetThreshold(*threshold)
	compressor.SetRatio(*ratio)
	compressor.SetKnee(*knee)
	compressor.SetAttack(*attack)
	compressor.SetRelease(*release)

	if *makeupGain != 0.0 {
		compressor.SetMakeupGain(*makeupGain)
	} else {
		compressor.SetAutoMakeup(*autoMakeup)
	}

	// Initialize PipeWire
	C.pw_init(nil, nil)

	// Create main loop
	loop := C.pw_main_loop_new(nil)
	if loop == nil {
		fmt.Println("ERROR: Failed to create PipeWire main loop")
		return
	}

	// Create a new PipeWire filter with separate ports for each channel
	filterData := C.create_pipewire_filter(loop, C.int(channels))
	if filterData == nil {
		fmt.Println("ERROR: Failed to create PipeWire filter")
		C.pw_main_loop_destroy(loop)
		return
	}

	if *noTUI {
		fmt.Println("Starting PipeWire Audio Compressor (pw-comp)...")
		fmt.Println("TUI disabled. Running in headless mode.")
		fmt.Println("Press Ctrl+C to exit.")
		
		// Run in main thread
		C.pw_main_loop_run(loop)
	} else {
		var wg sync.WaitGroup
		wg.Add(1)

		// Run PipeWire loop in background
		go func() {
			defer wg.Done()
			C.pw_main_loop_run(loop)
		}()
		
		// Give PipeWire a moment to start (optional)
		time.Sleep(100 * time.Millisecond)

		// Run TUI in main thread
		runTUI(compressor)
		
		// When TUI returns, quit PipeWire loop
		C.pw_main_loop_quit(loop)

		// Wait for PipeWire loop to finish cleaning up its internal state
		wg.Wait()
	}

	// Cleanup
	C.destroy_pipewire_filter(filterData)
	C.pw_main_loop_destroy(loop)
}