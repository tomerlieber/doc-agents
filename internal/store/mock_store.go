package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"doc-agents/internal/embeddings"
)

// MockStore is a mock implementation of Store using testify/mock.
type MockStore struct {
	mock.Mock
}

func (m *MockStore) CreateDocument(ctx context.Context, filename string) (Document, error) {
	args := m.Called(ctx, filename)
	return args.Get(0).(Document), args.Error(1)
}

func (m *MockStore) UpdateDocumentStatus(ctx context.Context, id uuid.UUID, status DocumentStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockStore) SaveChunks(ctx context.Context, docID uuid.UUID, chunks []Chunk) ([]Chunk, error) {
	args := m.Called(ctx, docID, chunks)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Chunk), args.Error(1)
}

func (m *MockStore) ListChunks(ctx context.Context, docID uuid.UUID) ([]Chunk, error) {
	args := m.Called(ctx, docID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Chunk), args.Error(1)
}

func (m *MockStore) SaveSummary(ctx context.Context, docID uuid.UUID, summary Summary) error {
	args := m.Called(ctx, docID, summary)
	return args.Error(0)
}

func (m *MockStore) SaveEmbedding(ctx context.Context, emb Embedding) error {
	args := m.Called(ctx, emb)
	return args.Error(0)
}

func (m *MockStore) GetSummary(ctx context.Context, docID uuid.UUID) (Summary, error) {
	args := m.Called(ctx, docID)
	return args.Get(0).(Summary), args.Error(1)
}

func (m *MockStore) TopK(ctx context.Context, docIDs []uuid.UUID, vector embeddings.Vector, k int) ([]SearchResult, error) {
	args := m.Called(ctx, docIDs, vector, k)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]SearchResult), args.Error(1)
}
