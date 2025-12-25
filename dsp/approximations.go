package dsp

import "math"

// Polynomial coefficients for continuous error function approximation.
// These coefficients provide a fast log2 approximation using a 5th-order polynomial.
//
//nolint:gochecknoglobals // Mathematical constants used across all FastLog2 calls
var cl2Continuous5 = []float64{
	-0.0821343513178931783,
	0.649732456739820052,
	-2.13417801862571777,
	4.08642207062728868,
	-1.51984215742349793,
}

// FastLog2 provides a fast approximation of log2(x) using polynomial evaluation.
// This is significantly faster than math.Log2() with acceptable accuracy for audio DSP.
//
// The approximation works by:
// 1. Extracting the exponent and mantissa using math.Frexp
// 2. Approximating log2(mantissa) using a polynomial
// 3. Combining: log2(x) = exponent + log2(mantissa).
func FastLog2(x float64) float64 {
	// Handle edge cases to avoid log(0)
	if x <= 0 {
		return -math.Inf(1)
	}

	// Extract exponent and mantissa
	// Frexp breaks x into mantissa (0.5 <= frac < 1.0) and exponent
	frac, exp := math.Frexp(x)

	// Approximate log2(frac) using Horner's method for polynomial evaluation
	logMantissa := cl2Continuous5[0]*frac + cl2Continuous5[1]
	logMantissa = logMantissa*frac + cl2Continuous5[2]
	logMantissa = logMantissa*frac + cl2Continuous5[3]
	logMantissa = logMantissa*frac + cl2Continuous5[4]

	// Compute log2(x) = exponent + log2(mantissa)
	// Note: exp-1 because Frexp returns mantissa in [0.5, 1.0) range
	return float64(exp-1) + logMantissa
}
