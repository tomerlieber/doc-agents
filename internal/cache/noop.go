package cache

import (
	"context"
	"time"
)

// NoOpCache is a cache implementation that does nothing.
// Used as a fallback when Redis is unavailable - all operations succeed
// but no actual caching occurs (always cache miss).
type NoOpCache struct{}

// NewNoOpCache creates a new no-op cache instance
func NewNoOpCache() *NoOpCache {
	return &NoOpCache{}
}

// GetQueryResult always returns nil (cache miss)
func (c *NoOpCache) GetQueryResult(ctx context.Context, key string) (*QueryResult, error) {
	return nil, nil
}

// SetQueryResult does nothing and always succeeds
func (c *NoOpCache) SetQueryResult(ctx context.Context, key string, result *QueryResult, ttl time.Duration) error {
	return nil
}

// InvalidateDocument does nothing and always succeeds
func (c *NoOpCache) InvalidateDocument(ctx context.Context, docID string) error {
	return nil
}

// Close does nothing and always succeeds
func (c *NoOpCache) Close() error {
	return nil
}
