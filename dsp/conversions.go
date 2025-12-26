package dsp

import "math"

const (
	// Silence threshold in dB - samples below this are considered silence.
	silenceThresholdDB = -144.0
)

// DBToLinear converts decibels to linear amplitude scale.
// Uses the formula: linear = 10^(dB/20).
//
// For performance, this uses FastLog2 internally:
// 10^(dB/20) = 2^(dB/20 * log2(10)).
func DBToLinear(db float64) float64 {
	// 10^(dB/20) = 2^(dB/20 * log2(10))
	// log2(10) ≈ 3.32192809489
	return math.Pow(2.0, db*log2Of10Div20)
}

// LinearToDB converts linear amplitude scale to decibels.
// Uses the formula: dB = 20 * log10(linear).
//
// For performance, this uses FastLog2:
// log10(x) = log2(x) / log2(10).
// dB = 20 * log2(linear) / log2(10).
func LinearToDB(linear float64) float64 {
	if linear <= 0 {
		return silenceThresholdDB
	}

	// Use FastLog2 for performance
	// log10(x) = log2(x) / log2(10)
	// 20 * log10(x) = 20 * log2(x) / log2(10)
	// = log2(x) * (20 / log2(10))
	// where 20 / log2(10) ≈ 6.020599913
	return FastLog2(linear) * 6.020599913
}

// LinearToDBSafe is a safe version that handles edge cases explicitly.
// Use this when you're uncertain about input values.
func LinearToDBSafe(linear float64) float64 {
	if math.IsNaN(linear) || math.IsInf(linear, 0) {
		return silenceThresholdDB
	}

	return LinearToDB(linear)
}

// FastPow2 computes 2^x efficiently for small integer exponents.
// For non-integer values, falls back to math.Pow.
func FastPow2(x float64) float64 {
	// For integer exponents, use bit shifting (extremely fast)
	if x == float64(int(x)) && x >= 0 && x < 64 {
		return float64(uint64(1) << uint(x))
	}

	// Otherwise use standard power function
	return math.Pow(2.0, x)
}

// FastPow computes base^exponent using FastLog2 for improved performance.
// This is faster than math.Pow for general cases.
// Formula: base^exp = 2^(exp * log2(base)).
func FastPow(base, exponent float64) float64 {
	if base <= 0 {
		return math.Pow(base, exponent) // Fall back for edge cases
	}

	// base^exp = 2^(exp * log2(base))
	return math.Pow(2.0, exponent*FastLog2(base))
}
