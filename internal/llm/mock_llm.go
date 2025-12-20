package llm

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockClient is a mock implementation of Client using testify/mock.
type MockClient struct {
	mock.Mock
}

func (m *MockClient) Summarize(ctx context.Context, text string) (string, []string, error) {
	args := m.Called(ctx, text)
	return args.String(0), args.Get(1).([]string), args.Error(2)
}

func (m *MockClient) Answer(ctx context.Context, question, context string, contextQuality float32) (string, float32, error) {
	args := m.Called(ctx, question, context, contextQuality)
	return args.String(0), float32(args.Get(1).(float64)), args.Error(2)
}
