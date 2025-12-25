package main

import "math"

// CalculateRMS calculates the Root Mean Square (average power) of a signal
func CalculateRMS(samples []float32) float64 {
	if len(samples) == 0 {
		return 0.0
	}

	var sum float64
	for _, sample := range samples {
		sum += float64(sample) * float64(sample)
	}

	return math.Sqrt(sum / float64(len(samples)))
}

// FindPeak finds the maximum absolute value in a buffer
func FindPeak(samples []float32) float32 {
	var peak float32
	for _, sample := range samples {
		abs := sample
		if abs < 0 {
			abs = -abs
		}
		if abs > peak {
			peak = abs
		}
	}
	return peak
}

// LinearToDBFS converts a linear amplitude value to dBFS
// Returns -infinity for values <= 0
func LinearToDBFS(linear float64) float64 {
	if linear <= 0 {
		return math.Inf(-1)
	}
	return 20.0 * math.Log10(linear)
}

// DBFSToLinear converts a dBFS value to linear amplitude
func DBFSToLinear(dbfs float64) float64 {
	return math.Pow(10.0, dbfs/20.0)
}

// MeasureGainReduction compares input and output buffers to calculate gain reduction
func MeasureGainReduction(input, output []float32) (inputRMS, outputRMS, gainReductionDB float64) {
	if len(input) != len(output) {
		panic("input and output buffers must have same length")
	}

	inputRMS = CalculateRMS(input)
	outputRMS = CalculateRMS(output)

	if inputRMS > 0 && outputRMS > 0 {
		inputDB := LinearToDBFS(inputRMS)
		outputDB := LinearToDBFS(outputRMS)
		gainReductionDB = inputDB - outputDB
	}

	return inputRMS, outputRMS, gainReductionDB
}
