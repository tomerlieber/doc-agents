package embeddings

import "github.com/stretchr/testify/mock"

// MockEmbedder is a mock implementation of Embedder using testify/mock.
type MockEmbedder struct {
	mock.Mock
}

func (m *MockEmbedder) Embed(text string) Vector {
	args := m.Called(text)
	return args.Get(0).(Vector)
}
