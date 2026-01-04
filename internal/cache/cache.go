package cache

import (
	"context"
	"time"
)

// Cache provides query result caching
type Cache interface {
	// GetQueryResult retrieves a cached query result by key
	// Returns nil if not found
	GetQueryResult(ctx context.Context, key string) (*QueryResult, error)

	// SetQueryResult stores a query result with TTL
	SetQueryResult(ctx context.Context, key string, result *QueryResult, ttl time.Duration) error

	// InvalidateDocument removes all cached queries for a document
	InvalidateDocument(ctx context.Context, docID string) error

	// Close closes the cache connection
	Close() error
}

// QueryResult represents a cached query response
type QueryResult struct {
	Answer     string
	Confidence float32
	Sources    []Source
}

// Source represents a document chunk source in query results
type Source struct {
	ChunkID string  `json:"chunk_id"`
	Score   float32 `json:"score"`
	Preview string  `json:"preview"` // Truncated text preview
}
