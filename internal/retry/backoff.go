package retry

import "time"

// ExponentialBackoff returns delay based on attempt number.
// The delay doubles with each attempt: base * 2^attempt
func ExponentialBackoff(attempt int, base time.Duration) time.Duration {
	return base * (1 << attempt)
}
