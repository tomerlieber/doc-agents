package cache

import (
	"context"
	"testing"
	"time"
)

// TestNoOpCache verifies that NoOpCache implements the Cache interface correctly
func TestNoOpCache(t *testing.T) {
	cache := NewNoOpCache()
	ctx := context.Background()

	// Test GetQueryResult - should always return nil (cache miss)
	result, err := cache.GetQueryResult(ctx, "test-key")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result (cache miss), got %v", result)
	}

	// Test SetQueryResult - should succeed silently
	err = cache.SetQueryResult(ctx, "test-key", &QueryResult{
		Answer:     "test answer",
		Confidence: 0.95,
		Sources:    []byte(`[{"chunk_id":"123"}]`),
	}, 1*time.Hour)
	if err != nil {
		t.Errorf("Expected no error on SetQueryResult, got %v", err)
	}

	// Verify it still returns nil (nothing was actually cached)
	result, err = cache.GetQueryResult(ctx, "test-key")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result (no-op cache doesn't store), got %v", result)
	}

	// Test InvalidateDocument - should succeed silently
	err = cache.InvalidateDocument(ctx, "doc-123")
	if err != nil {
		t.Errorf("Expected no error on InvalidateDocument, got %v", err)
	}

	// Test Close - should succeed silently
	err = cache.Close()
	if err != nil {
		t.Errorf("Expected no error on Close, got %v", err)
	}
}
