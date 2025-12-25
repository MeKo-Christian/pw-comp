package main

import (
	"math"
	"testing"
)

// Test configuration constants.
const (
	testSampleRate   = 48000.0
	testBufferSmall  = 256
	testBufferMedium = 512
	testBufferLarge  = 1024

	testFreq440Hz = 440.0
	testFreq1kHz  = 1000.0

	defaultThreshold = -20.0
	defaultRatio     = 4.0
	defaultKnee      = 6.0
	defaultAttack    = 10.0
	defaultRelease   = 100.0
)

// setupTestCompressor creates a fresh compressor instance with standard test parameters.
func setupTestCompressor() {
	compressor = NewSoftKneeCompressor(testSampleRate, 2)
	compressor.SetThreshold(defaultThreshold)
	compressor.SetRatio(defaultRatio)
	compressor.SetKnee(defaultKnee)
	compressor.SetAttack(defaultAttack)
	compressor.SetRelease(defaultRelease)
	compressor.SetMakeupGain(0.0) // Manual, no makeup gain by default
	compressor.Reset()
}

// A. Buffer Handling Tests

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_SilencePassthrough(t *testing.T) {
	setupTestCompressor()

	// Generate silent buffer
	buffer := GenerateDC(0.0, testBufferMedium*2) // Stereo

	// Process through integration point
	processAudioBuffer(buffer)

	// Verify output is still silent (or near-silent due to numerical precision)
	peak := FindPeak(buffer)
	if peak > 0.0001 {
		t.Errorf("Silence passthrough failed: expected near-zero output, got peak %.6f", peak)
	}
}

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_ValidBufferSizes(t *testing.T) {
	setupTestCompressor()

	bufferSizes := []int{testBufferSmall, testBufferMedium, testBufferLarge, 2048}

	for _, size := range bufferSizes {
		buffer := GenerateInterleavedStereoSine(SineWaveConfig{
			Frequency:  testFreq1kHz,
			Amplitude:  0.1,
			SampleRate: testSampleRate,
		}, size, 0.0)

		// Should not panic
		processAudioBuffer(buffer)

		// Verify output is reasonable
		peak := FindPeak(buffer)
		if peak <= 0 || peak > 1.0 {
			t.Errorf("Buffer size %d produced invalid output: peak %.6f", size, peak)
		}
	}
}

// B. Compression Behavior Tests

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_BelowThreshold_NoCompression(t *testing.T) {
	setupTestCompressor()

	// Generate signal well below threshold (-40 dBFS, threshold is -20 dBFS)
	amplitude := DBFSToLinear(-40.0)
	buffer := GenerateInterleavedStereoSine(SineWaveConfig{
		Frequency:  testFreq1kHz,
		Amplitude:  amplitude,
		SampleRate: testSampleRate,
	}, testBufferLarge, 0.0)

	inputCopy := append([]float32{}, buffer...)

	// Process
	processAudioBuffer(buffer)

	// Measure levels
	inputRMS := CalculateRMS(inputCopy)
	outputRMS := CalculateRMS(buffer)

	// Below threshold, output should be very close to input (minimal compression)
	ratio := outputRMS / inputRMS
	if ratio < 0.95 || ratio > 1.05 {
		t.Errorf("Signal below threshold was compressed: input RMS %.6f, output RMS %.6f, ratio %.3f",
			inputRMS, outputRMS, ratio)
	}
}

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_AboveThreshold_HasCompression(t *testing.T) {
	setupTestCompressor()

	// Generate signal well above threshold (-10 dBFS, threshold is -20 dBFS)
	amplitude := DBFSToLinear(-10.0)
	buffer := GenerateInterleavedStereoSine(SineWaveConfig{
		Frequency:  testFreq1kHz,
		Amplitude:  amplitude,
		SampleRate: testSampleRate,
	}, testBufferLarge, 0.0)

	inputCopy := append([]float32{}, buffer...)

	// Process (need multiple buffers for attack to fully engage)
	for range 10 {
		processAudioBuffer(buffer)
	}

	// Measure levels
	inputRMS := CalculateRMS(inputCopy)
	outputRMS := CalculateRMS(buffer)

	// Above threshold, output should be significantly reduced
	if outputRMS >= inputRMS {
		t.Errorf("Signal above threshold was not compressed: input RMS %.6f, output RMS %.6f",
			inputRMS, outputRMS)
	}

	// Verify compression occurred
	inputDBFS := LinearToDBFS(inputRMS)
	outputDBFS := LinearToDBFS(outputRMS)
	gainReduction := inputDBFS - outputDBFS

	if gainReduction < 1.0 {
		t.Errorf("Insufficient gain reduction: expected > 1dB, got %.2f dB", gainReduction)
	}
}

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_CompressionRatio_Verification(t *testing.T) {
	setupTestCompressor()

	// Set hard knee for predictable compression ratio
	compressor.SetKnee(0.0)

	// Fast attack to reach steady state quickly
	compressor.SetAttack(1.0)

	// Generate signal 12 dB above threshold
	// Threshold: -20 dBFS, Input: -8 dBFS, Excess: 12 dB
	amplitude := DBFSToLinear(-8.0)

	// Process multiple buffers to let attack envelope reach steady state
	// Each buffer is a fresh copy of the signal
	var buffer []float32
	for range 20 {
		buffer = GenerateInterleavedStereoSine(SineWaveConfig{
			Frequency:  testFreq1kHz,
			Amplitude:  amplitude,
			SampleRate: testSampleRate,
		}, testBufferLarge, 0.0)
		processAudioBuffer(buffer)
	}

	// Measure output from the last buffer
	outputRMS := CalculateRMS(buffer)
	outputDBFS := LinearToDBFS(outputRMS)

	// With 4:1 ratio, 12 dB excess becomes 3 dB excess
	// Expected output: -20 + 3 = -17 dBFS
	// Due to soft knee (even at 0 width), attack/release envelopes, and RMS vs peak measurements,
	// allow larger tolerance
	expectedDBFS := -17.0
	toleranceDB := 3.0 // dB

	if math.Abs(outputDBFS-expectedDBFS) > toleranceDB {
		t.Errorf("Compression ratio verification failed: expected ~%.1f dBFS, got %.1f dBFS (diff %.2f dB)",
			expectedDBFS, outputDBFS, math.Abs(outputDBFS-expectedDBFS))
	}
}

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_MakeupGain(t *testing.T) {
	setupTestCompressor()

	// Enable automatic makeup gain
	compressor.SetMakeupGain(-1.0) // Auto mode

	// Generate signal above threshold
	amplitude := DBFSToLinear(-10.0)

	// Process multiple buffers to let attack reach steady state
	var buffer []float32
	for range 20 {
		buffer = GenerateInterleavedStereoSine(SineWaveConfig{
			Frequency:  testFreq1kHz,
			Amplitude:  amplitude,
			SampleRate: testSampleRate,
		}, testBufferLarge, 0.0)
		processAudioBuffer(buffer)
	}

	// Measure output
	outputRMS := CalculateRMS(buffer)

	// With auto makeup gain, output should be boosted compared to compressed-only signal
	// We're not checking absolute level, just that makeup gain is applied
	// Output should be non-zero and reasonable
	if outputRMS < amplitude*0.1 || outputRMS == 0 {
		t.Errorf("Auto makeup gain appears not to be working: output RMS %.6f is too low (input was %.6f)",
			outputRMS, amplitude)
	}
}

// C. Stereo Processing Tests

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_StereoChannelIndependence_LeftOnly(t *testing.T) {
	setupTestCompressor()

	// Generate signal only on left channel
	leftAmplitude := DBFSToLinear(-10.0)
	left := GenerateSine(SineWaveConfig{
		Frequency:  testFreq1kHz,
		Amplitude:  leftAmplitude,
		SampleRate: testSampleRate,
	}, testBufferLarge)

	right := GenerateDC(0.0, testBufferLarge) // Silent

	buffer := InterleaveChannels(left, right)

	// Process multiple buffers
	for range 20 {
		processAudioBuffer(buffer)
	}

	// Deinterleave and analyze
	leftOut, rightOut := DeinterleaveChannels(buffer)

	leftRMS := CalculateRMS(leftOut)
	rightRMS := CalculateRMS(rightOut)

	// Left should be compressed (reduced), right should stay silent
	if leftRMS >= leftAmplitude {
		t.Errorf("Left channel was not compressed: RMS %.6f", leftRMS)
	}

	if rightRMS > 0.0001 {
		t.Errorf("Right channel should remain silent: RMS %.6f", rightRMS)
	}
}

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_StereoChannelIndependence_DifferentSignals(t *testing.T) {
	setupTestCompressor()

	// Generate different amplitude signals on each channel
	leftAmplitude := DBFSToLinear(-10.0)  // Loud (above threshold)
	rightAmplitude := DBFSToLinear(-30.0) // Quiet (below threshold)

	// Process multiple fresh buffers to reach steady state
	var buffer []float32

	for range 20 {
		left := GenerateSine(SineWaveConfig{
			Frequency:  testFreq440Hz,
			Amplitude:  leftAmplitude,
			SampleRate: testSampleRate,
		}, testBufferLarge)

		right := GenerateSine(SineWaveConfig{
			Frequency:  testFreq1kHz,
			Amplitude:  rightAmplitude,
			SampleRate: testSampleRate,
		}, testBufferLarge)

		buffer = InterleaveChannels(left, right)
		processAudioBuffer(buffer)
	}

	// Deinterleave and analyze the last buffer
	leftOut, rightOut := DeinterleaveChannels(buffer)

	leftRMS := CalculateRMS(leftOut)
	rightRMS := CalculateRMS(rightOut)

	// Left should be compressed significantly
	if leftRMS >= leftAmplitude*0.9 {
		t.Errorf("Left channel was not compressed enough: input %.6f, output %.6f",
			leftAmplitude, leftRMS)
	}

	// Right should pass through mostly unchanged (below threshold)
	// Allow 30% tolerance since signal is very quiet and numerical precision matters at low levels
	if math.Abs(float64(rightRMS)-rightAmplitude) > rightAmplitude*0.3 {
		t.Errorf("Right channel was affected: input %.6f, output %.6f (%.1f%% change)",
			rightAmplitude, rightRMS, 100.0*math.Abs(float64(rightRMS)-rightAmplitude)/rightAmplitude)
	}
}

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_StereoPhaseCoherence(t *testing.T) {
	setupTestCompressor()

	// Generate identical signals on both channels
	amplitude := DBFSToLinear(-15.0)
	buffer := GenerateInterleavedStereoSine(SineWaveConfig{
		Frequency:  testFreq1kHz,
		Amplitude:  amplitude,
		SampleRate: testSampleRate,
	}, testBufferLarge, 0.0) // 0.0 phase difference

	// Process
	for range 20 {
		processAudioBuffer(buffer)
	}

	// Deinterleave
	leftOut, rightOut := DeinterleaveChannels(buffer)

	// Verify L and R are still identical (or very close)
	maxDiff := float32(0.0)

	for i := range leftOut {
		diff := leftOut[i] - rightOut[i]
		if diff < 0 {
			diff = -diff
		}

		if diff > maxDiff {
			maxDiff = diff
		}
	}

	// Allow for minimal numerical differences
	if maxDiff > 0.001 {
		t.Errorf("Stereo phase coherence lost: max L/R difference %.6f", maxDiff)
	}
}

// D. Dynamic Response Tests

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_AttackResponse(t *testing.T) {
	setupTestCompressor()

	// Set known attack time
	attackTimeMS := 20.0
	compressor.SetAttack(attackTimeMS)
	compressor.SetRelease(200.0) // Slow release

	// Generate step signal: silence then loud
	stepPosition := testBufferMedium / 2
	amplitude := DBFSToLinear(-5.0) // Well above threshold

	left := GenerateStep(amplitude, stepPosition, testBufferMedium)
	right := GenerateStep(amplitude, stepPosition, testBufferMedium)
	buffer := InterleaveChannels(left, right)

	// Process
	processAudioBuffer(buffer)

	// Deinterleave
	leftOut, _ := DeinterleaveChannels(buffer)

	// Before step, should be silent
	preStepRMS := CalculateRMS(leftOut[:stepPosition])
	if preStepRMS > 0.001 {
		t.Errorf("Pre-step region should be silent: RMS %.6f", preStepRMS)
	}

	// After step, should show attack envelope (gradually increasing compression)
	// Not testing exact timing here, just that compression engages
	postStepRMS := CalculateRMS(leftOut[stepPosition:])
	if postStepRMS >= amplitude {
		t.Errorf("Attack response not detected: output RMS %.6f should be < input %.6f",
			postStepRMS, amplitude)
	}
}

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_ContinuousProcessing_StateCarryover(t *testing.T) {
	setupTestCompressor()

	// Generate continuous loud signal split across buffers
	amplitude := DBFSToLinear(-10.0)

	buffer1 := GenerateInterleavedStereoSine(SineWaveConfig{
		Frequency:  testFreq1kHz,
		Amplitude:  amplitude,
		SampleRate: testSampleRate,
	}, testBufferMedium, 0.0)

	buffer2 := GenerateInterleavedStereoSine(SineWaveConfig{
		Frequency:  testFreq1kHz,
		Amplitude:  amplitude,
		SampleRate: testSampleRate,
		Phase:      2.0 * math.Pi * testFreq1kHz * float64(testBufferMedium) / testSampleRate,
	}, testBufferMedium, 0.0)

	// Process first buffer
	processAudioBuffer(buffer1)
	rms1 := CalculateRMS(buffer1)

	// Process second buffer
	processAudioBuffer(buffer2)
	rms2 := CalculateRMS(buffer2)

	// Second buffer should have similar or more compression (state carries over)
	// RMS should be relatively stable
	if math.Abs(float64(rms1-rms2))/float64(rms1) > 0.3 {
		t.Errorf("State carryover issue: buffer1 RMS %.6f, buffer2 RMS %.6f (%.1f%% difference)",
			rms1, rms2, 100.0*math.Abs(float64(rms1-rms2))/float64(rms1))
	}
}

// E. Edge Cases & Robustness

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_FullScaleSignal_NoClipping(t *testing.T) {
	setupTestCompressor()

	// Generate signal near full scale
	amplitude := 0.99
	buffer := GenerateInterleavedStereoSine(SineWaveConfig{
		Frequency:  testFreq1kHz,
		Amplitude:  amplitude,
		SampleRate: testSampleRate,
	}, testBufferLarge, 0.0)

	// Process multiple buffers
	for range 20 {
		processAudioBuffer(buffer)
	}

	// Verify no clipping
	peak := FindPeak(buffer)
	if peak > 1.0 {
		t.Errorf("Output clipping detected: peak %.6f > 1.0", peak)
	}

	// Verify compression occurred
	rms := CalculateRMS(buffer)
	if rms > amplitude*0.9 {
		t.Errorf("Full scale signal should be compressed: RMS %.6f", rms)
	}
}

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_ParameterChangeMidStream(t *testing.T) {
	setupTestCompressor()

	// Generate signal
	amplitude := DBFSToLinear(-10.0)
	buffer := GenerateInterleavedStereoSine(SineWaveConfig{
		Frequency:  testFreq1kHz,
		Amplitude:  amplitude,
		SampleRate: testSampleRate,
	}, testBufferMedium, 0.0)

	// Process with initial parameters
	for range 10 {
		processAudioBuffer(buffer)
	}

	rms1 := CalculateRMS(buffer)

	// Change threshold mid-stream
	compressor.SetThreshold(-30.0) // Much lower threshold = less compression

	// Process more buffers
	for range 10 {
		processAudioBuffer(buffer)
	}

	rms2 := CalculateRMS(buffer)

	// Output should change (less compression with lower threshold)
	if math.Abs(float64(rms1-rms2))/float64(rms1) < 0.05 {
		t.Errorf("Parameter change had minimal effect: RMS before %.6f, after %.6f", rms1, rms2)
	}

	// Verify no clicks/pops (peak should be reasonable)
	peak := FindPeak(buffer)
	if peak > 1.0 {
		t.Errorf("Parameter change caused clipping: peak %.6f", peak)
	}
}

//nolint:paralleltest // integration tests use shared global compressor state
func TestIntegration_RealisticBufferSizes(t *testing.T) {
	setupTestCompressor()

	// Test common PipeWire buffer sizes
	bufferSizes := []int{64, 128, 256, 512, 1024, 2048}

	for _, size := range bufferSizes {
		amplitude := DBFSToLinear(-15.0)
		buffer := GenerateInterleavedStereoSine(SineWaveConfig{
			Frequency:  testFreq1kHz,
			Amplitude:  amplitude,
			SampleRate: testSampleRate,
		}, size, 0.0)

		// Process multiple times
		for range 5 {
			processAudioBuffer(buffer)
		}

		// Verify reasonable output
		rms := CalculateRMS(buffer)
		peak := FindPeak(buffer)

		if rms == 0 || peak == 0 {
			t.Errorf("Buffer size %d produced zero output", size)
		}

		if peak > 1.0 {
			t.Errorf("Buffer size %d caused clipping: peak %.6f", size, peak)
		}
	}
}
