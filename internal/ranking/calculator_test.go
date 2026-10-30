package ranking

import (
	"math"
	"testing"
)

func TestCalculateBayesianRating_FiveStars(t *testing.T) {
	m := 10.0
	c := 3.50

	tests := []struct {
		name     string
		votes    int
		mean     float64
		expected float64
	}{
		{
			name:     "Zero votes defaults to prior mean",
			votes:    0,
			mean:     0.0,
			expected: 3.50,
		},
		{
			name:     "Single 5-star vote is dampened by prior",
			votes:    1,
			mean:     5.0,
			expected: 3.64, // (1*5 + 10*3.5) / 11 = 40 / 11 = 3.636 -> 3.64
		},
		{
			name:     "High votes with high score approaches true mean",
			votes:    1000,
			mean:     4.90,
			expected: 4.89, // (1000*4.9 + 10*3.5) / 1010 = 4935 / 1010 = 4.886 -> 4.89
		},
		{
			name:     "Low score with moderate votes",
			votes:    100,
			mean:     1.50,
			expected: 1.68, // (100*1.5 + 10*3.5) / 110 = 185 / 110 = 1.681 -> 1.68
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateBayesianRating(tt.votes, tt.mean, m, c)
			if math.Abs(got-tt.expected) > 0.01 {
				t.Errorf("CalculateBayesianRating(%d, %.2f) = %.2f, want %.2f", tt.votes, tt.mean, got, tt.expected)
			}
		})
	}
}
