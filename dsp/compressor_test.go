package dsp

import (
	"math"
	"testing"
)

// TestNewSoftKneeCompressor verifies the compressor initializes with correct defaults.
func TestNewSoftKneeCompressor(t *testing.T) {
	t.Parallel()

	sampleRate := 48000.0
	channels := 2

	comp := NewSoftKneeCompressor(sampleRate, channels)

	if comp == nil {
		t.Fatal("NewSoftKneeCompressor returned nil")
	}

	if comp.sampleRate != sampleRate {
		t.Errorf("Expected sample rate %f, got %f", sampleRate, comp.sampleRate)
	}

	if comp.channels != channels {
		t.Errorf("Expected %d channels, got %d", channels, comp.channels)
	}

	if len(comp.peak) != channels {
		t.Errorf("Expected peak array length %d, got %d", channels, len(comp.peak))
	}

	// Verify defaults
	if comp.thresholdDB != -20.0 {
		t.Errorf("Expected default threshold -20.0 dB, got %f", comp.thresholdDB)
	}

	if comp.ratio != 4.0 {
		t.Errorf("Expected default ratio 4.0, got %f", comp.ratio)
	}

	if comp.kneeDB != 6.0 {
		t.Errorf("Expected default knee 6.0 dB, got %f", comp.kneeDB)
	}

	if !comp.autoMakeup {
		t.Error("Expected auto makeup to be enabled by default")
	}
}

// TestSetParameters verifies parameter setters update internal state correctly.
func TestSetParameters(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)

	// Test threshold
	comp.SetThreshold(-10.0)

	if comp.thresholdDB != -10.0 {
		t.Errorf("SetThreshold failed: expected -10.0, got %f", comp.thresholdDB)
	}

	// Test ratio
	comp.SetRatio(8.0)

	if comp.ratio != 8.0 {
		t.Errorf("SetRatio failed: expected 8.0, got %f", comp.ratio)
	}

	// Test ratio minimum clamping
	comp.SetRatio(0.5)

	if comp.ratio < 1.0 {
		t.Errorf("SetRatio should clamp to minimum 1.0, got %f", comp.ratio)
	}

	// Test knee
	comp.SetKnee(3.0)

	if comp.kneeDB != 3.0 {
		t.Errorf("SetKnee failed: expected 3.0, got %f", comp.kneeDB)
	}

	// Test knee minimum clamping
	comp.SetKnee(-5.0)

	if comp.kneeDB < 0.0 {
		t.Errorf("SetKnee should clamp to minimum 0.0, got %f", comp.kneeDB)
	}

	// Test attack time
	comp.SetAttack(5.0)

	if comp.attackMs != 5.0 {
		t.Errorf("SetAttack failed: expected 5.0, got %f", comp.attackMs)
	}

	// Test release time
	comp.SetRelease(200.0)

	if comp.releaseMs != 200.0 {
		t.Errorf("SetRelease failed: expected 200.0, got %f", comp.releaseMs)
	}

	// Test manual makeup gain
	comp.SetMakeupGain(3.0)

	if comp.makeupGainDB != 3.0 {
		t.Errorf("SetMakeupGain failed: expected 3.0, got %f", comp.makeupGainDB)
	}

	if comp.autoMakeup {
		t.Error("SetMakeupGain should disable auto makeup")
	}

	// Test auto makeup
	comp.SetAutoMakeup(true)

	if !comp.autoMakeup {
		t.Error("SetAutoMakeup failed to enable auto makeup")
	}
}

// TestThresholdConversion verifies dB to linear conversion.
func TestThresholdConversion(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetThreshold(-20.0)

	expected := math.Pow(10.0, -20.0/20.0)
	if math.Abs(comp.threshold-expected) > 1e-9 {
		t.Errorf("Threshold conversion: expected %f, got %f", expected, comp.threshold)
	}
}

// TestAttackReleaseCoefficients verifies time constant calculations.
func TestAttackReleaseCoefficients(t *testing.T) {
	t.Parallel()

	sampleRate := 48000.0
	comp := NewSoftKneeCompressor(sampleRate, 2)

	attackMs := 10.0
	releaseMs := 100.0

	comp.SetAttack(attackMs)
	comp.SetRelease(releaseMs)

	// Attack factor should be between 0 and 1
	if comp.attackFactor <= 0.0 || comp.attackFactor >= 1.0 {
		t.Errorf("Attack factor out of range: %f", comp.attackFactor)
	}

	// Release factor should be between 0 and 1
	if comp.releaseFactor <= 0.0 || comp.releaseFactor >= 1.0 {
		t.Errorf("Release factor out of range: %f", comp.releaseFactor)
	}

	// Attack should respond faster than release
	// (higher attack factor means faster response)
	expectedAttack := 1.0 - math.Exp(-math.Ln2/(attackMs*0.001*sampleRate))
	expectedRelease := math.Exp(-math.Ln2 / (releaseMs * 0.001 * sampleRate))

	if math.Abs(comp.attackFactor-expectedAttack) > 1e-9 {
		t.Errorf("Attack factor: expected %f, got %f", expectedAttack, comp.attackFactor)
	}

	if math.Abs(comp.releaseFactor-expectedRelease) > 1e-9 {
		t.Errorf("Release factor: expected %f, got %f", expectedRelease, comp.releaseFactor)
	}
}

// TestKneeBoundaries verifies kneeFactor calculation in log2 space.
func TestKneeBoundaries(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetThreshold(-20.0)
	comp.SetRatio(4.0)
	comp.SetKnee(6.0)

	// kneeFactor should equal (2 * log2(10)/20 * kneeDB)²
	// This matches the Pascal implementation: Sqr(2 * CdBtoAmpExpGain32 * FKnee_dB)
	kneeLog2 := 2.0 * 0.166096404744 * 6.0
	expectedKneeFactor := kneeLog2 * kneeLog2

	if math.Abs(comp.kneeFactor-expectedKneeFactor) > 1e-9 {
		t.Errorf("Knee factor: expected %f, got %f", expectedKneeFactor, comp.kneeFactor)
	}
}

// TestNoCompressionBelowKnee verifies no compression below the threshold.
func TestNoCompressionBelowKnee(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetThreshold(-20.0)
	comp.SetKnee(6.0)

	// Level well below threshold should have gain of 1.0 (no compression)
	// -30 dB is well below -20 dB threshold
	lowLevel := math.Pow(10.0, -30.0/20.0) // -30 dBFS in linear
	gain := comp.calculateGain(lowLevel)

	if math.Abs(gain-1.0) > 1e-6 {
		t.Errorf("Gain below threshold should be 1.0, got %f", gain)
	}
}

// TestFullCompressionAboveKnee verifies compression behavior well above threshold.
func TestFullCompressionAboveKnee(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetThreshold(-20.0)
	comp.SetRatio(4.0)
	comp.SetKnee(6.0)

	// Level well above threshold (e.g., -10 dBFS, which is 10 dB above -20 dB threshold)
	highLevel := math.Pow(10.0, -10.0/20.0)
	gain := comp.calculateGain(highLevel)

	// The new dB-domain formula produces slightly different results
	// Verify gain is less than 1.0 (compression is happening)
	if gain >= 1.0 {
		t.Errorf("Gain should be less than 1.0 for compression, got %f", gain)
	}

	// Verify compression is substantial (gain should be much less than 1.0)
	if gain > 0.8 {
		t.Errorf("Gain should show significant compression, got %f", gain)
	}
}

// TestSoftKneeTransition verifies smooth gain transition with hyperbolic knee.
func TestSoftKneeTransition(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetThreshold(-20.0)
	comp.SetRatio(4.0)
	comp.SetKnee(6.0)

	// Test that gain transitions smoothly around threshold
	// At exactly the threshold level
	thresholdLevel := math.Pow(10.0, -20.0/20.0)
	gainAtThreshold := comp.calculateGain(thresholdLevel)

	// At threshold, we should have some compression starting
	if gainAtThreshold >= 1.0 {
		t.Errorf("Gain at threshold should show some compression, got %f", gainAtThreshold)
	}

	if gainAtThreshold <= 0.0 {
		t.Errorf("Gain at threshold should be positive, got %f", gainAtThreshold)
	}

	// Test slightly above threshold
	aboveThreshold := math.Pow(10.0, -18.0/20.0) // 2 dB above threshold
	gainAbove := comp.calculateGain(aboveThreshold)

	// Gain should decrease as we go above threshold
	if gainAbove >= gainAtThreshold {
		t.Errorf("Gain should decrease above threshold: at threshold=%f, above=%f", gainAtThreshold, gainAbove)
	}
}

// TestProcessSampleNoCompression verifies silent signal passes through.
func TestProcessSampleNoCompression(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetThreshold(-20.0)
	comp.SetAutoMakeup(false)
	comp.SetMakeupGain(0.0)

	// Very quiet signal should pass through unaffected (accounting for makeup gain)
	input := float32(0.001)
	output := comp.ProcessSample(input, 0)

	// Should be approximately equal (may have minimal processing)
	if math.Abs(float64(output-input)) > 0.001 {
		t.Errorf("Quiet signal should pass through: input %f, output %f", input, output)
	}
}

// TestProcessSampleCompression verifies loud signal is compressed.
func TestProcessSampleCompression(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetThreshold(-20.0)
	comp.SetRatio(4.0)
	comp.SetAttack(1.0)     // Fast attack
	comp.SetMakeupGain(0.0) // Disable makeup gain
	comp.Reset()

	// Very loud signal should be compressed
	input := float32(1.0) // 0 dBFS

	// Process multiple samples to allow peak detector to respond
	var output float32
	for range 200 {
		output = comp.ProcessSample(input, 0)
	}

	// Output should be reduced (compressed)
	// Allow small tolerance for numerical precision
	if float64(output) >= float64(input)*0.99 {
		t.Errorf("Loud signal should be compressed: input %f, output %f", input, output)
	}
}

// TestPeakDetectorAttack verifies fast attack response.
func TestPeakDetectorAttack(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetAttack(1.0) // Very fast attack
	comp.SetThreshold(-20.0)
	comp.Reset()

	// Send a series of increasing samples
	loudSample := float32(0.5)

	// Process multiple samples to allow peak detector to respond
	for range 500 {
		comp.ProcessSample(loudSample, 0)
	}

	// Peak should have tracked up close to the signal level
	// With attack time, it should be at least 90% of the target
	if comp.peak[0] < 0.45 {
		t.Errorf("Peak detector should track loud signal: peak %f, expected >= 0.45", comp.peak[0])
	}
}

// TestPeakDetectorRelease verifies release decay.
func TestPeakDetectorRelease(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetAttack(1.0)
	comp.SetRelease(10.0) // Fast release for testing
	comp.SetThreshold(-20.0)
	comp.Reset()

	// Build up peak
	loudSample := float32(0.5)
	for range 100 {
		comp.ProcessSample(loudSample, 0)
	}

	peakAfterAttack := comp.peak[0]

	// Process silence to trigger release
	for range 100 {
		comp.ProcessSample(0.0, 0)
	}

	// Peak should have decayed
	if comp.peak[0] >= peakAfterAttack {
		t.Errorf("Peak should decay during release: before %f, after %f",
			peakAfterAttack, comp.peak[0])
	}
}

// TestAutoMakeupGain verifies automatic makeup gain calculation.
func TestAutoMakeupGain(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetThreshold(-20.0)
	comp.SetRatio(4.0)
	comp.SetAutoMakeup(true)

	// Auto makeup gain should compensate for gain reduction at threshold
	// Formula: -threshold_dB * (1 - 1/ratio)
	expectedMakeupDB := -(-20.0) * (1.0 - 1.0/4.0)

	if math.Abs(comp.makeupGainDB-expectedMakeupDB) > 1e-6 {
		t.Errorf("Auto makeup gain: expected %f dB, got %f dB",
			expectedMakeupDB, comp.makeupGainDB)
	}

	// Verify linear makeup gain
	expectedLinear := math.Pow(10.0, expectedMakeupDB/20.0)
	if math.Abs(comp.makeupGainLin-expectedLinear) > 1e-6 {
		t.Errorf("Makeup gain linear: expected %f, got %f",
			expectedLinear, comp.makeupGainLin)
	}
}

// TestReset verifies reset clears peak state.
func TestReset(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetThreshold(-20.0)

	// Build up peak
	for range 100 {
		comp.ProcessSample(0.5, 0)
		comp.ProcessSample(0.5, 1)
	}

	// Verify peaks are non-zero
	if comp.peak[0] == 0.0 || comp.peak[1] == 0.0 {
		t.Error("Peaks should be non-zero after processing")
	}

	// Reset
	comp.Reset()

	// Verify peaks are cleared
	if comp.peak[0] != 0.0 || comp.peak[1] != 0.0 {
		t.Errorf("Reset should clear peaks: ch0=%f, ch1=%f", comp.peak[0], comp.peak[1])
	}
}

// TestChannelIndependence verifies each channel maintains independent state.
func TestChannelIndependence(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetAttack(1.0) // Fast attack for testing
	comp.SetThreshold(-20.0)
	comp.Reset()

	// Process only channel 0
	for range 500 {
		comp.ProcessSample(0.5, 0)
	}

	// Channel 0 should have peak, channel 1 should not
	if comp.peak[0] < 0.45 {
		t.Errorf("Channel 0 should have peak level: got %f", comp.peak[0])
	}

	if comp.peak[1] != 0.0 {
		t.Error("Channel 1 should remain at zero")
	}
}

// TestInvalidChannel verifies out-of-bounds channel handling.
func TestInvalidChannel(t *testing.T) {
	t.Parallel()

	comp := NewSoftKneeCompressor(48000.0, 2)

	input := float32(0.5)

	// Test negative channel
	output := comp.ProcessSample(input, -1)
	if output != input {
		t.Error("Invalid channel should return input unchanged")
	}

	// Test channel beyond range
	output = comp.ProcessSample(input, 10)
	if output != input {
		t.Error("Invalid channel should return input unchanged")
	}
}

// BenchmarkProcessSample benchmarks single sample processing.
func BenchmarkProcessSample(b *testing.B) {
	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetThreshold(-20.0)
	comp.SetRatio(4.0)

	sample := float32(0.5)

	b.ResetTimer()

	for range b.N {
		comp.ProcessSample(sample, 0)
	}
}

// BenchmarkProcessStereo benchmarks stereo processing.
func BenchmarkProcessStereo(b *testing.B) {
	comp := NewSoftKneeCompressor(48000.0, 2)
	comp.SetThreshold(-20.0)
	comp.SetRatio(4.0)

	sampleL := float32(0.5)
	sampleR := float32(0.6)

	b.ResetTimer()

	for range b.N {
		comp.ProcessSample(sampleL, 0)
		comp.ProcessSample(sampleR, 1)
	}
}
