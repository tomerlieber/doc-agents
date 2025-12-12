package queue

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockQueue is a mock implementation of Queue using testify/mock.
type MockQueue struct {
	mock.Mock
}

func (m *MockQueue) Enqueue(ctx context.Context, task Task) error {
	args := m.Called(ctx, task)
	return args.Error(0)
}

func (m *MockQueue) Worker(ctx context.Context, taskType TaskType, handler Handler) error {
	args := m.Called(ctx, taskType, handler)
	return args.Error(0)
}
