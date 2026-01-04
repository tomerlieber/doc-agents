package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Key prefix for cached results
	cacheKeyPrefix = "query:"

	// Key prefix for document tracking
	docKeyPrefix = "doc:"
)

type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache client
func NewRedisCache(addr, password string) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisCache{
		client: client,
	}, nil
}

// GetQueryResult retrieves a cached query result by key
func (c *RedisCache) GetQueryResult(ctx context.Context, key string) (*QueryResult, error) {
	data, err := c.client.Get(ctx, cacheKeyPrefix+key).Bytes()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, err
	}

	var result QueryResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetQueryResult stores a query result with TTL
func (c *RedisCache) SetQueryResult(ctx context.Context, key string, result *QueryResult, ttl time.Duration) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	// Store the result
	if err := c.client.Set(ctx, cacheKeyPrefix+key, data, ttl).Err(); err != nil {
		return err
	}

	return nil
}

// InvalidateDocument removes all cached queries for a document
func (c *RedisCache) InvalidateDocument(ctx context.Context, docID string) error {
	// Use SCAN to find all keys containing this docID
	// This is a simple implementation - for production you might want to maintain
	// a separate index of document->query mappings
	iter := c.client.Scan(ctx, 0, cacheKeyPrefix+"*", 0).Iterator()

	pipe := c.client.Pipeline()
	count := 0

	for iter.Next(ctx) {
		key := iter.Val()
		// Check if the cached query involves this document
		// For now, we'll use a simple approach: delete all caches
		// In production, you'd want to track which documents each query uses
		pipe.Del(ctx, key)
		count++
	}

	if err := iter.Err(); err != nil {
		return err
	}

	if count > 0 {
		_, err := pipe.Exec(ctx)
		return err
	}

	return nil
}

// Close closes the cache connection
func (c *RedisCache) Close() error {
	return c.client.Close()
}
