package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Cache provides query result and embedding caching
type Cache interface {
	// GetQueryResult retrieves a cached query result by key
	// Returns nil if not found
	GetQueryResult(ctx context.Context, key string) (*QueryResult, error)

	// SetQueryResult stores a query result with TTL
	SetQueryResult(ctx context.Context, key string, result *QueryResult, ttl time.Duration) error

	// GetEmbedding retrieves a cached embedding vector for the given text
	// Returns nil if not found
	GetEmbedding(ctx context.Context, text string) ([]float32, error)

	// SetEmbedding stores an embedding vector for the given text with TTL
	SetEmbedding(ctx context.Context, text string, vector []float32, ttl time.Duration) error

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

// GenerateCacheKey creates a deterministic cache key from query parameters.
// The key is implementation-agnostic and can be used with any cache backend.
func GenerateCacheKey(question string, docIDs []string, topK int) string {
	// Sort docIDs to ensure consistent ordering
	sortedIDs := make([]string, len(docIDs))
	copy(sortedIDs, docIDs)
	// Simple sort for determinism
	for i := 0; i < len(sortedIDs); i++ {
		for j := i + 1; j < len(sortedIDs); j++ {
			if sortedIDs[i] > sortedIDs[j] {
				sortedIDs[i], sortedIDs[j] = sortedIDs[j], sortedIDs[i]
			}
		}
	}

	data := fmt.Sprintf("q:%s|docs:%s|k:%d", question, strings.Join(sortedIDs, ","), topK)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// GenerateEmbeddingKey creates a deterministic cache key for embedding text.
// Uses SHA-256 hash to ensure same text always produces same key.
func GenerateEmbeddingKey(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}
