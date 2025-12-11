package retry

import (
	"testing"
	"time"
)

func TestExponentialBackoff(t *testing.T) {
	base := 100 * time.Millisecond

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},  // base * 2^0 = 100ms
		{1, 200 * time.Millisecond},  // base * 2^1 = 200ms
		{2, 400 * time.Millisecond},  // base * 2^2 = 400ms
		{3, 800 * time.Millisecond},  // base * 2^3 = 800ms
		{4, 1600 * time.Millisecond}, // base * 2^4 = 1600ms
	}

	for _, tt := range tests {
		result := ExponentialBackoff(tt.attempt, base)
		if result != tt.expected {
			t.Errorf("attempt %d: got %v, want %v", tt.attempt, result, tt.expected)
		}
	}
}

func TestExponentialBackoffWithDifferentBase(t *testing.T) {
	base := 1 * time.Second

	result := ExponentialBackoff(2, base)
	expected := 4 * time.Second

	if result != expected {
		t.Errorf("got %v, want %v", result, expected)
	}
}
