package dsp

import (
	"math"
	"sync"
	"sync/atomic"
)

const (
	// log2Of10Div20 is the conversion factor for dB to log2: log2(10) / 20
	// Used for converting decibel values to log2 domain for fast approximation
	log2Of10Div20 = 0.166096404744
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
	kneeFactor     float64 // Knee factor in log2² space: (2*log2(10)/20 * kneeDB)²
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

// ProcessSample processes a single sample for tests (wraps internal with lock).
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

// GetThreshold returns the current threshold in dB.
func (c *SoftKneeCompressor) GetThreshold() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.thresholdDB
}

// GetRatio returns the current compression ratio.
func (c *SoftKneeCompressor) GetRatio() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ratio
}

// GetKnee returns the current knee width in dB.
func (c *SoftKneeCompressor) GetKnee() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.kneeDB
}

// GetAttack returns the current attack time in milliseconds.
func (c *SoftKneeCompressor) GetAttack() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.attackMs
}

// GetRelease returns the current release time in milliseconds.
func (c *SoftKneeCompressor) GetRelease() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.releaseMs
}

// GetMakeupGain returns the current makeup gain in dB.
func (c *SoftKneeCompressor) GetMakeupGain() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.makeupGainDB
}

// GetAutoMakeup returns whether automatic makeup gain is enabled.
func (c *SoftKneeCompressor) GetAutoMakeup() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.autoMakeup
}

// GetBypass returns whether bypass is enabled.
func (c *SoftKneeCompressor) GetBypass() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.bypass
}

// updateTimeConstants recalculates attack and release coefficients (internal, assumes lock held).
func (c *SoftKneeCompressor) updateTimeConstants() {
	c.attackFactor = 1.0 - math.Exp(-math.Ln2/(c.attackMs*0.001*c.sampleRate))
	c.releaseFactor = math.Exp(-math.Ln2 / (c.releaseMs * 0.001 * c.sampleRate))
}

// updateParameters recalculates all internal cached values (internal, assumes lock held).
func (c *SoftKneeCompressor) updateParameters() {
	c.threshold = DBToLinear(c.thresholdDB)
	c.thresholdRecip = 1.0 / c.threshold

	// Calculate kneeFactor = (2 * log2(10)/20 * kneeDB)²
	// This matches the Pascal implementation: Sqr(2 * CdBtoAmpExpGain32 * FKnee_dB)
	// Working in log2 space, not dB space
	kneeLog2 := 2.0 * log2Of10Div20 * c.kneeDB
	c.kneeFactor = kneeLog2 * kneeLog2

	c.slopeRecip = 1.0/c.ratio - 1.0 // Kept for future use or debugging

	if c.autoMakeup {
		gainReductionDB := c.thresholdDB * (1.0 - 1.0/c.ratio)
		c.makeupGainDB = -gainReductionDB
	}

	c.makeupGainLin = DBToLinear(c.makeupGainDB)
	c.updateTimeConstants()
}

// processSampleInternal processes a single sample (internal DSP logic, called by ProcessBlock).
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

// calculateGain computes the gain multiplier using log2-domain soft-knee formula.
// This matches the Pascal reference implementation which works entirely in log2 space.
// Formula: gain_log2 = 0.5 * (delta - sqrt(delta² + kneeFactor)).
// where delta = thresholdLog2 - peakLog2.
func (c *SoftKneeCompressor) calculateGain(peakLevel float64) float64 {
	if peakLevel <= 0 {
		return 1.0
	}

	// Convert peak level to log2 using FastLog2
	peakLog2 := FastLog2(peakLevel)

	// Calculate log2 difference from threshold
	// thresholdLog2 = thresholdDB * (log2(10)/20)
	thresholdLog2 := c.thresholdDB * log2Of10Div20

	// Temp = thresholdLog2 - peakLog2 (Pascal: Temp := FThrshlddB - FastLog2(FPeak))
	// When signal is above threshold, Temp will be negative
	// When signal is below threshold, Temp will be positive
	temp := thresholdLog2 - peakLog2

	// If signal is below threshold (temp > 0), no compression needed
	if temp > 0 {
		return 1.0
	}

	// Soft-knee formula in log2 domain (based on Pascal limiter implementation)
	// gain_log2 = 0.5 * (Temp - sqrt(Temp² + kneeFactor))
	// Note: Temp is negative when above threshold, so this produces more negative values
	// as the signal gets louder, resulting in more gain reduction
	tempSq := temp * temp
	sqrtTerm := FastSqrt(tempSq + c.kneeFactor)
	gainLog2 := 0.5 * (temp - sqrtTerm)

	// Apply compression ratio to the gain reduction
	// The Pascal reference is a limiter (infinite ratio). To support variable ratios:
	// - Ratio 1:1 → factor = 0.0 → no compression
	// - Ratio 4:1 → factor = 0.75 → 75% of reduction
	// - Ratio ∞:1 → factor = 1.0 → full reduction (limiter behavior)
	gainLog2 *= (1.0 - 1.0/c.ratio)

	// Convert back to linear using FastPower2
	return FastPower2(gainLog2)
}
