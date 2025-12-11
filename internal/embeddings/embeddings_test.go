package embeddings

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     Vector
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        Vector{1, 0, 0},
			b:        Vector{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        Vector{1, 0},
			b:        Vector{0, 1},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        Vector{1, 0},
			b:        Vector{-1, 0},
			expected: -1.0,
		},
		{
			name:     "empty vectors",
			a:        Vector{},
			b:        Vector{},
			expected: 0.0,
		},
		{
			name:     "different length vectors",
			a:        Vector{1, 2},
			b:        Vector{1, 2, 3},
			expected: 0.0,
		},
		{
			name:     "normalized vectors 45 degrees",
			a:        Vector{1, 0},
			b:        Vector{0.707, 0.707},
			expected: 0.707,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CosineSimilarity(tt.a, tt.b)
			if math.Abs(float64(result-tt.expected)) > 0.01 {
				t.Errorf("got %f, want %f", result, tt.expected)
			}
		})
	}
}
