package main

import (
	"math"
	"sync"
	"sync/atomic"
)

// MeterStats holds current levels for UI.
type MeterStats struct {
	InputL         float64
	InputR         float64
	OutputL        float64
	OutputR        float64
	GainReductionL float64
	GainReductionR float64
	Blocks         uint64
	SampleRate     float64
}

// SoftKneeCompressor implements a professional-quality dynamics processor
// with soft-knee compression, attack/release envelopes, and automatic makeup gain.
type SoftKneeCompressor struct {
	mu sync.Mutex // Protects parameters and coefficient updates

	// User parameters
	thresholdDB  float64 // Compression threshold in dB
	ratio        float64 // Compression ratio (e.g., 4.0 for 4:1)
	kneeDB       float64 // Soft knee width in dB
	attackMs     float64 // Attack time in milliseconds
	releaseMs    float64 // Release time in milliseconds
	makeupGainDB float64 // Makeup gain in dB
	autoMakeup   bool    // Automatic makeup gain calculation
	bypass       bool    // Bypass processing

	// Internal state (per channel)
	peak          []float64 // Current peak level for each channel
	attackFactor  float64   // Attack coefficient
	releaseFactor float64   // Release coefficient

	// Cached calculations
	threshold      float64 // Linear threshold
	thresholdRecip float64 // 1 / threshold
	kneeWidth      float64 // Knee width in linear
	kneeUpper      float64 // Upper knee boundary
	kneeLower      float64 // Lower knee boundary
	makeupGainLin  float64 // Linear makeup gain
	slopeRecip     float64 // 1 / ratio - 1 (for gain calculation)
	sampleRate     float64 // Current sample rate
	channels       int     // Number of audio channels

	// Metering (Atomic bits of float64 for lock-free UI reading)
	inputPeakL      uint64
	inputPeakR      uint64
	outputPeakL     uint64
	outputPeakR     uint64
	gainReductionL  uint64
	gainReductionR  uint64
	processedBlocks uint64 // Atomic counter
}

// NewSoftKneeCompressor creates a new compressor with default settings.
func NewSoftKneeCompressor(sampleRate float64, channels int) *SoftKneeCompressor {
	compressor := &SoftKneeCompressor{
		thresholdDB:     -20.0,
		ratio:           4.0,
		kneeDB:          6.0,
		attackMs:        10.0,
		releaseMs:       100.0,
		makeupGainDB:    0.0,
		autoMakeup:      true,
		bypass:          false,
		sampleRate:      sampleRate,
		channels:        channels,
		peak:            make([]float64, channels),
		processedBlocks: 0,
	}
	compressor.updateParameters()

	return compressor
}

// SetThreshold sets the compression threshold in dB.
func (c *SoftKneeCompressor) SetThreshold(dB float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.thresholdDB = dB
	c.updateParameters()
}

// SetRatio sets the compression ratio.
func (c *SoftKneeCompressor) SetRatio(ratio float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ratio < 1.0 {
		ratio = 1.0
	}

	c.ratio = ratio
	c.updateParameters()
}

// SetKnee sets the soft knee width in dB.
func (c *SoftKneeCompressor) SetKnee(kneeDB float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if kneeDB < 0.0 {
		kneeDB = 0.0
	}

	c.kneeDB = kneeDB
	c.updateParameters()
}

// SetAttack sets the attack time in milliseconds.
func (c *SoftKneeCompressor) SetAttack(timeMs float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if timeMs < 0.1 {
		timeMs = 0.1
	}

	c.attackMs = timeMs
	c.updateTimeConstants()
}

// SetRelease sets the release time in milliseconds.
func (c *SoftKneeCompressor) SetRelease(timeMs float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if timeMs < 1.0 {
		timeMs = 1.0
	}

	c.releaseMs = timeMs
	c.updateTimeConstants()
}

// SetMakeupGain sets the makeup gain in dB.
func (c *SoftKneeCompressor) SetMakeupGain(dB float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.makeupGainDB = dB
	c.autoMakeup = false
	c.updateParameters()
}

// SetAutoMakeup enables automatic makeup gain calculation.
func (c *SoftKneeCompressor) SetAutoMakeup(enable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.autoMakeup = enable
	c.updateParameters()
}

// SetBypass toggles bypass.
func (c *SoftKneeCompressor) SetBypass(bypass bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.bypass = bypass
}

// SetSampleRate updates the sample rate and recalculates time constants.
func (c *SoftKneeCompressor) SetSampleRate(rate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if rate <= 0.0 {
		return
	}

	if c.sampleRate != rate {
		c.sampleRate = rate
		c.updateTimeConstants()
	}
}

// updateTimeConstants recalculates attack and release coefficients (internal, assumes lock held).
func (c *SoftKneeCompressor) updateTimeConstants() {
	c.attackFactor = 1.0 - math.Exp(-math.Ln2/(c.attackMs*0.001*c.sampleRate))
	c.releaseFactor = math.Exp(-math.Ln2 / (c.releaseMs * 0.001 * c.sampleRate))
}

// updateParameters recalculates all internal cached values (internal, assumes lock held).
func (c *SoftKneeCompressor) updateParameters() {
	c.threshold = math.Pow(10.0, c.thresholdDB/20.0)
	c.thresholdRecip = 1.0 / c.threshold

	kneeHalfDB := c.kneeDB / 2.0
	c.kneeLower = math.Pow(10.0, (c.thresholdDB-kneeHalfDB)/20.0)
	c.kneeUpper = math.Pow(10.0, (c.thresholdDB+kneeHalfDB)/20.0)
	c.kneeWidth = c.kneeUpper - c.kneeLower

	c.slopeRecip = 1.0/c.ratio - 1.0

	if c.autoMakeup {
		gainReductionDB := c.thresholdDB * (1.0 - 1.0/c.ratio)
		c.makeupGainDB = -gainReductionDB
	}

	c.makeupGainLin = math.Pow(10.0, c.makeupGainDB/20.0)
	c.updateTimeConstants()
}

// ProcessSample processes a single sample (internal DSP logic, called by ProcessBlock)
// Assumes caller holds lock or is single-threaded context (tests).
func (c *SoftKneeCompressor) processSampleInternal(sample float32, channel int) (float32, float64) {
	if c.bypass {
		return sample, 1.0
	}

	if channel < 0 || channel >= c.channels {
		return sample, 1.0
	}

	inputLevel := math.Abs(float64(sample))
	if math.IsNaN(inputLevel) {
		inputLevel = 0 // Sanitize
	}

	if inputLevel > c.peak[channel] {
		c.peak[channel] += (inputLevel - c.peak[channel]) * c.attackFactor
	} else {
		c.peak[channel] = inputLevel + (c.peak[channel]-inputLevel)*c.releaseFactor
	}

	if math.IsNaN(c.peak[channel]) {
		c.peak[channel] = 0 // Safety reset
	}

	gain := c.calculateGain(c.peak[channel])
	if math.IsNaN(gain) {
		gain = 1.0
	}

	output := float32(float64(sample) * gain * c.makeupGainLin)

	return output, gain
}

// Public ProcessSample for tests (wraps internal with lock).
func (c *SoftKneeCompressor) ProcessSample(sample float32, channel int) float32 {
	c.mu.Lock()
	defer c.mu.Unlock()

	out, _ := c.processSampleInternal(sample, channel)

	return out
}

// ProcessBlock processes a slice of samples for a specific channel.
func (c *SoftKneeCompressor) ProcessBlock(in []float32, out []float32, channel int) {
	if channel < 0 || channel >= c.channels || len(in) != len(out) {
		return
	}

	// Lock once per block
	c.mu.Lock()
	defer c.mu.Unlock()

	var maxInput, maxOutput float64
	minGain := 1.0

	for i := 0; i < len(in); i++ {
		// NaN Check
		if math.IsNaN(float64(in[i])) || math.IsInf(float64(in[i]), 0) {
			in[i] = 0
		}

		// Calculate meters
		absIn := math.Abs(float64(in[i]))
		if absIn > maxInput {
			maxInput = absIn
		}

		processed, gain := c.processSampleInternal(in[i], channel)

		// NaN Check Output
		if math.IsNaN(float64(processed)) || math.IsInf(float64(processed), 0) {
			processed = 0
		}

		out[i] = processed

		absOut := math.Abs(float64(processed))
		if absOut > maxOutput {
			maxOutput = absOut
		}

		if gain < minGain {
			minGain = gain
		}
	}

	// Update atomic meters
	switch channel {
	case 0: // Left
		atomic.StoreUint64(&c.inputPeakL, math.Float64bits(maxInput))
		atomic.StoreUint64(&c.outputPeakL, math.Float64bits(maxOutput))
		atomic.StoreUint64(&c.gainReductionL, math.Float64bits(minGain))
		// Increment block counter (only on left channel to avoid double counting per stereo frame)
		atomic.AddUint64(&c.processedBlocks, 1)
	case 1: // Right
		atomic.StoreUint64(&c.inputPeakR, math.Float64bits(maxInput))
		atomic.StoreUint64(&c.outputPeakR, math.Float64bits(maxOutput))
		atomic.StoreUint64(&c.gainReductionR, math.Float64bits(minGain))
	}
}

// calculateGain computes the gain multiplier.
func (c *SoftKneeCompressor) calculateGain(peakLevel float64) float64 {
	if peakLevel <= c.kneeLower {
		return 1.0
	}

	if peakLevel >= c.kneeUpper {
		return math.Pow(c.threshold/peakLevel, 1.0-1.0/c.ratio)
	}

	kneePos := (peakLevel - c.kneeLower) / c.kneeWidth
	smoothFactor := kneePos * kneePos * (3.0 - 2.0*kneePos)
	compressedGain := math.Pow(c.threshold/c.kneeUpper, 1.0-1.0/c.ratio)

	return 1.0 + (compressedGain-1.0)*smoothFactor
}

// Reset clears the internal state.
func (c *SoftKneeCompressor) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.peak {
		c.peak[i] = 0.0
	}
}

// GetMeters returns current meter values safely.
func (c *SoftKneeCompressor) GetMeters() MeterStats {
	// Sample rate requires lock
	c.mu.Lock()
	sampleRate := c.sampleRate
	c.mu.Unlock()

	return MeterStats{
		InputL:         math.Float64frombits(atomic.LoadUint64(&c.inputPeakL)),
		InputR:         math.Float64frombits(atomic.LoadUint64(&c.inputPeakR)),
		OutputL:        math.Float64frombits(atomic.LoadUint64(&c.outputPeakL)),
		OutputR:        math.Float64frombits(atomic.LoadUint64(&c.outputPeakR)),
		GainReductionL: math.Float64frombits(atomic.LoadUint64(&c.gainReductionL)),
		GainReductionR: math.Float64frombits(atomic.LoadUint64(&c.gainReductionR)),
		Blocks:         atomic.LoadUint64(&c.processedBlocks),
		SampleRate:     sampleRate,
	}
}
