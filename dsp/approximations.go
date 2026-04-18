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

// Polynomial coefficients for 2^x approximation (3rd order minimum error).
// These coefficients provide a fast power-of-2 approximation for the fractional part.
//
//nolint:gochecknoglobals // Mathematical constants used across all FastPower2 calls
var cp2MinError3 = []float64{
	0.693292707161004662,
	0.242162975514835621,
	0.0548668824216034384,
}

// FastPower2 computes 2^x using 3rd-order polynomial approximation.
// This is significantly faster than math.Pow(2.0, x) with acceptable accuracy for audio DSP.
//
// The approximation works by:
// 1. Splitting x into integer and fractional parts
// 2. Computing 2^intPart using efficient bit shifting (math.Ldexp)
// 3. Approximating 2^fracPart using a polynomial
// 4. Combining: 2^x = 2^intPart * 2^fracPart.
func FastPower2(x float64) float64 {
	// Split into integer and fractional parts
	intPart := int(math.Round(x))
	fracPart := x - float64(intPart)

	// Use bit shifting for integer part (very fast)
	intResult := math.Ldexp(1.0, intPart) // Efficient 2^intPart

	// Evaluate polynomial for fractional part using Horner's method
	// poly(f) = 1 + f * (c0 + f * (c1 + f * c2))
	polyResult := 1.0 + fracPart*(cp2MinError3[0]+
		fracPart*(cp2MinError3[1]+fracPart*cp2MinError3[2]))

	return intResult * polyResult
}

// FastSqrt computes sqrt(x) using Babylonian/Newton-Raphson method.
// with bit manipulation for a fast initial guess.
//
// The approximation works by:
// 1. Using bit manipulation to get an initial guess
// 2. Performing two Newton-Raphson iterations for refinement
// Formula: x_new = 0.5 * (x_old + value/x_old).
func FastSqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}

	// Get initial approximation via bit manipulation
	// This exploits the IEEE 754 representation to estimate sqrt
	bits := math.Float64bits(x)
	bits = ((bits - (1 << 52)) >> 1) + (1 << 61)
	result := math.Float64frombits(bits)

	// Two Newton-Raphson iterations for refinement
	// First iteration: standard formula
	result = 0.5 * (result + x/result)

	// Second iteration: apply the same formula again
	result = 0.5 * (result + x/result)

	return result
}
