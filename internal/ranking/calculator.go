package ranking

import (
	"math"
)

const (
	// DefaultMinimumVotes represents m: the credibility vote count threshold
	DefaultMinimumVotes = 10.0
	// DefaultPriorMean represents C: expected baseline prior score in 1-5 scale (ADR-0013)
	DefaultPriorMean = 3.50
)

// CalculateBayesianRating computes the weighted score WR:
// WR = (v / (v + m)) * R + (m / (v + m)) * C
// Clamped to [1.00, 5.00] and rounded to 2 decimal places.
func CalculateBayesianRating(v int, r float64, m float64, c float64) float64 {
	if m <= 0 {
		m = DefaultMinimumVotes
	}
	if c <= 0 {
		c = DefaultPriorMean
	}

	if v <= 0 {
		return c
	}

	vFloat := float64(v)
	wr := (vFloat/(vFloat+m))*r + (m/(vFloat+m))*c

	// Clamp within 1.00 - 5.00
	if wr < 1.00 {
		wr = 1.00
	} else if wr > 5.00 {
		wr = 5.00
	}

	return math.Round(wr*100) / 100
}
