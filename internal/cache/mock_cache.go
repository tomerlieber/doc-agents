package cache

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
)

// MockCache is a mock implementation of the Cache interface for testing
type MockCache struct {
	mock.Mock
}

func (m *MockCache) GetQueryResult(ctx context.Context, key string) (*QueryResult, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*QueryResult), args.Error(1)
}

func (m *MockCache) SetQueryResult(ctx context.Context, key string, result *QueryResult, ttl time.Duration) error {
	args := m.Called(ctx, key, result, ttl)
	return args.Error(0)
}

func (m *MockCache) InvalidateDocument(ctx context.Context, docID string) error {
	args := m.Called(ctx, docID)
	return args.Error(0)
}

func (m *MockCache) Close() error {
	args := m.Called()
	return args.Error(0)
}
