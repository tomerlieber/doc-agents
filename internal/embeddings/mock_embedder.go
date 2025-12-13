package embeddings

import "github.com/stretchr/testify/mock"

// MockEmbedder is a mock implementation of Embedder using testify/mock.
type MockEmbedder struct {
	mock.Mock
}

func (m *MockEmbedder) Embed(text string) (Vector, error) {
	args := m.Called(text)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(Vector), args.Error(1)
}
