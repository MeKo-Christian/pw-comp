package main

import (
	"math"
)

// SoftKneeCompressor implements a professional-quality dynamics processor
// with soft-knee compression, attack/release envelopes, and automatic makeup gain.
// Based on the Delphi ASIO & VST Project implementation.
type SoftKneeCompressor struct {
	// User parameters
	thresholdDB  float64 // Compression threshold in dB
	ratio        float64 // Compression ratio (e.g., 4.0 for 4:1)
	kneeDB       float64 // Soft knee width in dB
	attackMs     float64 // Attack time in milliseconds
	releaseMs    float64 // Release time in milliseconds
	makeupGainDB float64 // Makeup gain in dB
	autoMakeup   bool    // Automatic makeup gain calculation

	// Internal state (per channel)
	peak          []float64 // Current peak level for each channel
	attackFactor  float64   // Attack coefficient
	releaseFactor float64   // Release coefficient

	// Cached calculations
	threshold        float64 // Linear threshold
	thresholdRecip   float64 // 1 / threshold
	kneeWidth        float64 // Knee width in linear
	kneeUpper        float64 // Upper knee boundary
	kneeLower        float64 // Lower knee boundary
	makeupGainLin    float64 // Linear makeup gain
	slopeRecip       float64 // 1 / ratio - 1 (for gain calculation)
	sampleRate       float64 // Current sample rate
	channels         int     // Number of audio channels
}

// NewSoftKneeCompressor creates a new compressor with default settings
func NewSoftKneeCompressor(sampleRate float64, channels int) *SoftKneeCompressor {
	c := &SoftKneeCompressor{
		thresholdDB:  -20.0,
		ratio:        4.0,
		kneeDB:       6.0,
		attackMs:     10.0,
		releaseMs:    100.0,
		makeupGainDB: 0.0,
		autoMakeup:   true,
		sampleRate:   sampleRate,
		channels:     channels,
		peak:         make([]float64, channels),
	}
	c.updateParameters()
	return c
}

// SetThreshold sets the compression threshold in dB
func (c *SoftKneeCompressor) SetThreshold(dB float64) {
	c.thresholdDB = dB
	c.updateParameters()
}

// SetRatio sets the compression ratio
func (c *SoftKneeCompressor) SetRatio(ratio float64) {
	if ratio < 1.0 {
		ratio = 1.0
	}
	c.ratio = ratio
	c.updateParameters()
}

// SetKnee sets the soft knee width in dB
func (c *SoftKneeCompressor) SetKnee(dB float64) {
	if dB < 0.0 {
		dB = 0.0
	}
	c.kneeDB = dB
	c.updateParameters()
}

// SetAttack sets the attack time in milliseconds
func (c *SoftKneeCompressor) SetAttack(ms float64) {
	if ms < 0.1 {
		ms = 0.1
	}
	c.attackMs = ms
	c.updateTimeConstants()
}

// SetRelease sets the release time in milliseconds
func (c *SoftKneeCompressor) SetRelease(ms float64) {
	if ms < 1.0 {
		ms = 1.0
	}
	c.releaseMs = ms
	c.updateTimeConstants()
}

// SetMakeupGain sets the makeup gain in dB
func (c *SoftKneeCompressor) SetMakeupGain(dB float64) {
	c.makeupGainDB = dB
	c.autoMakeup = false
	c.updateParameters()
}

// SetAutoMakeup enables automatic makeup gain calculation
func (c *SoftKneeCompressor) SetAutoMakeup(enable bool) {
	c.autoMakeup = enable
	c.updateParameters()
}

// SetSampleRate updates the sample rate and recalculates time constants
func (c *SoftKneeCompressor) SetSampleRate(rate float64) {
	if rate <= 0.0 {
		return
	}
	// Only update if changed to avoid unnecessary recalculation
	if c.sampleRate != rate {
		c.sampleRate = rate
		c.updateTimeConstants()
	}
}

// updateTimeConstants recalculates attack and release coefficients
func (c *SoftKneeCompressor) updateTimeConstants() {
	// Convert time constants to exponential coefficients
	// Attack: how fast to respond to increases in level
	c.attackFactor = 1.0 - math.Exp(-math.Ln2/(c.attackMs*0.001*c.sampleRate))

	// Release: how fast to decay when level decreases
	c.releaseFactor = math.Exp(-math.Ln2 / (c.releaseMs * 0.001 * c.sampleRate))
}

// updateParameters recalculates all internal cached values
func (c *SoftKneeCompressor) updateParameters() {
	// Convert threshold from dB to linear
	c.threshold = math.Pow(10.0, c.thresholdDB/20.0)
	c.thresholdRecip = 1.0 / c.threshold

	// Calculate knee boundaries
	kneeHalfDB := c.kneeDB / 2.0
	c.kneeLower = math.Pow(10.0, (c.thresholdDB-kneeHalfDB)/20.0)
	c.kneeUpper = math.Pow(10.0, (c.thresholdDB+kneeHalfDB)/20.0)
	c.kneeWidth = c.kneeUpper - c.kneeLower

	// Slope for gain calculation
	c.slopeRecip = 1.0/c.ratio - 1.0

	// Calculate makeup gain
	if c.autoMakeup {
		// Automatic: compensate for gain reduction at threshold
		gainReductionDB := c.thresholdDB * (1.0 - 1.0/c.ratio)
		c.makeupGainDB = -gainReductionDB
	}
	c.makeupGainLin = math.Pow(10.0, c.makeupGainDB/20.0)

	// Update time constants
	c.updateTimeConstants()
}

// ProcessSample processes a single sample for a specific channel
// using peak detection with attack/release envelope and soft-knee compression
func (c *SoftKneeCompressor) ProcessSample(sample float32, channel int) float32 {
	if channel < 0 || channel >= c.channels {
		return sample
	}

	// Get absolute value of input
	inputLevel := math.Abs(float64(sample))

	// Peak detector with attack/release envelope follower
	if inputLevel > c.peak[channel] {
		// Attack: fast response to increasing levels
		c.peak[channel] += (inputLevel - c.peak[channel]) * c.attackFactor
	} else {
		// Release: slower decay when level decreases
		c.peak[channel] = inputLevel + (c.peak[channel]-inputLevel)*c.releaseFactor
	}

	// Calculate gain reduction based on peak level
	gain := c.calculateGain(c.peak[channel])

	// Apply gain and makeup gain
	return float32(float64(sample) * gain * c.makeupGainLin)
}

// ProcessBlock processes a slice of samples for a specific channel
func (c *SoftKneeCompressor) ProcessBlock(in []float32, out []float32, channel int) {
	if channel < 0 || channel >= c.channels || len(in) != len(out) {
		return
	}

	for i := 0; i < len(in); i++ {
		out[i] = c.ProcessSample(in[i], channel)
	}
}

// calculateGain computes the gain multiplier for a given peak level
// using soft-knee compression curve
func (c *SoftKneeCompressor) calculateGain(peakLevel float64) float64 {
	if peakLevel <= c.kneeLower {
		// Below knee: no compression
		return 1.0
	} else if peakLevel >= c.kneeUpper {
		// Above knee: full compression ratio
		// Formula: gain = (threshold/level)^(1 - 1/ratio)
		// This reduces loud signals above threshold
		return math.Pow(c.threshold/peakLevel, 1.0-1.0/c.ratio)
	} else {
		// Inside knee: soft transition using polynomial approximation
		// Normalized position within knee (0 to 1)
		kneePos := (peakLevel - c.kneeLower) / c.kneeWidth

		// Smooth polynomial interpolation (cubic hermite)
		// This creates a smooth S-curve transition from 1.0 to compressed gain
		smoothFactor := kneePos * kneePos * (3.0 - 2.0*kneePos)

		// Gain at upper knee boundary (full compression)
		compressedGain := math.Pow(c.threshold/c.kneeUpper, 1.0-1.0/c.ratio)

		// Interpolate between no compression (1.0) and full compression
		return 1.0 + (compressedGain-1.0)*smoothFactor
	}
}

// Reset clears the internal state (peak levels)
func (c *SoftKneeCompressor) Reset() {
	for i := range c.peak {
		c.peak[i] = 0.0
	}
}
