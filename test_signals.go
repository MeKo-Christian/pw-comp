package main

import "math"

// SineWaveConfig holds configuration for sine wave generation.
type SineWaveConfig struct {
	Frequency  float64 // Hz
	Amplitude  float64 // 0.0 to 1.0 (linear, not dB)
	Phase      float64 // radians
	SampleRate float64 // Hz
}

// GenerateSine creates a mono sine wave buffer.
func GenerateSine(config SineWaveConfig, frames int) []float32 {
	buffer := make([]float32, frames)
	omega := 2.0 * math.Pi * config.Frequency / config.SampleRate

	for i := range frames {
		phase := omega*float64(i) + config.Phase
		buffer[i] = float32(config.Amplitude * math.Sin(phase))
	}

	return buffer
}

// GenerateInterleavedStereoSine creates a stereo sine wave with interleaved L/R samples
// rightPhase allows phase offset between left and right channels (in radians).
func GenerateInterleavedStereoSine(config SineWaveConfig, frames int, rightPhase float64) []float32 {
	buffer := make([]float32, frames*2) // Interleaved L/R
	omega := 2.0 * math.Pi * config.Frequency / config.SampleRate

	for i := range frames {
		leftPhase := omega*float64(i) + config.Phase
		rightPhaseVal := omega*float64(i) + config.Phase + rightPhase

		buffer[i*2] = float32(config.Amplitude * math.Sin(leftPhase))       // Left
		buffer[i*2+1] = float32(config.Amplitude * math.Sin(rightPhaseVal)) // Right
	}

	return buffer
}

// GenerateDC creates a buffer filled with a constant DC level.
func GenerateDC(level float64, length int) []float32 {
	buffer := make([]float32, length)
	for i := range buffer {
		buffer[i] = float32(level)
	}

	return buffer
}

// GenerateStep creates a step function signal
// Signal is 0 before startPosition, then jumps to amplitude.
func GenerateStep(amplitude float64, startPosition, length int) []float32 {
	buffer := make([]float32, length)
	for i := startPosition; i < length; i++ {
		buffer[i] = float32(amplitude)
	}

	return buffer
}

// GenerateImpulse creates an impulse (single non-zero sample).
func GenerateImpulse(amplitude float64, position, length int) []float32 {
	buffer := make([]float32, length)
	if position < length {
		buffer[position] = float32(amplitude)
	}

	return buffer
}

// InterleaveChannels combines two mono buffers into a stereo interleaved buffer.
func InterleaveChannels(left, right []float32) []float32 {
	if len(left) != len(right) {
		panic("left and right channels must have same length")
	}

	interleaved := make([]float32, len(left)*2)
	for i := range left {
		interleaved[i*2] = left[i]
		interleaved[i*2+1] = right[i]
	}

	return interleaved
}

// DeinterleaveChannels splits a stereo interleaved buffer into two mono buffers.
func DeinterleaveChannels(interleaved []float32) (left, right []float32) {
	if len(interleaved)%2 != 0 {
		panic("interleaved buffer must have even length")
	}

	frames := len(interleaved) / 2
	left = make([]float32, frames)
	right = make([]float32, frames)

	for i := range frames {
		left[i] = interleaved[i*2]
		right[i] = interleaved[i*2+1]
	}

	return left, right
}
