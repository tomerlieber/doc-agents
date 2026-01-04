package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"doc-agents/internal/app"
	"doc-agents/internal/config"
	"doc-agents/internal/embeddings"
	"doc-agents/internal/llm"
	"doc-agents/internal/store"
)

func newTestDeps(st store.Store, l llm.Client, e embeddings.Embedder) app.AnalysisDeps {
	return app.AnalysisDeps{
		BaseDeps: app.BaseDeps{
			Store: st,
			Config: config.Config{
				EmbeddingModel: "test-model",
			},
			Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		LLM:      l,
		Embedder: e,
	}
}

func TestHandleAnalyze(t *testing.T) {
	validDocID := uuid.New()
	chunk1ID := uuid.New()
	chunk2ID := uuid.New()

	tests := []struct {
		name    string
		payload analyzeTaskPayload
		setup   func(*store.MockStore, *llm.MockClient, *embeddings.MockEmbedder)
		wantErr bool
	}{
		{
			name: "successful analysis with single chunk",
			payload: analyzeTaskPayload{
				DocumentID: validDocID.String(),
				ChunkIDs:   []uuid.UUID{chunk1ID},
			},
			setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
				// Expect GetDocument to be called
				s.On("GetDocument", mock.Anything, validDocID).
					Return(store.Document{ID: validDocID, Filename: "test.pdf"}, nil).Once()

				// Expect ListChunks to be called
				s.On("ListChunks", mock.Anything, validDocID).
					Return([]store.Chunk{
						{ID: chunk1ID, Index: 0, Text: "Test chunk", TokenCount: 2},
					}, nil).Once()

				// Expect LLM.Summarize to be called
				l.On("Summarize", mock.Anything, "Test chunk\n").
					Return("Test summary", []string{"Key point 1"}, nil).Once()

				// Expect SaveSummary to be called
				s.On("SaveSummary", mock.Anything, validDocID, mock.MatchedBy(func(sum store.Summary) bool {
					return sum.Summary == "Test summary"
				})).Return(nil).Once()

				// Expect batch embedder to be called with enriched chunk texts
				e.On("EmbedBatch", []string{"Document: test.pdf\n\nTest chunk"}).
					Return([]embeddings.Vector{{0.1, 0.2, 0.3}}, nil).Once()

				// Expect SaveEmbeddings (batch) to be called with 1 embedding
				s.On("SaveEmbeddings", mock.Anything, mock.MatchedBy(func(embs []store.Embedding) bool {
					return len(embs) == 1 && embs[0].ChunkID == chunk1ID
				})).Return(nil).Once()

				// Expect status update to ready
				s.On("UpdateDocumentStatus", mock.Anything, validDocID, store.StatusReady).
					Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name: "successful analysis with multiple chunks",
			payload: analyzeTaskPayload{
				DocumentID: validDocID.String(),
				ChunkIDs:   []uuid.UUID{chunk1ID, chunk2ID},
			},
			setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
				s.On("GetDocument", mock.Anything, validDocID).
					Return(store.Document{ID: validDocID, Filename: "test.pdf"}, nil).Once()

				s.On("ListChunks", mock.Anything, validDocID).
					Return([]store.Chunk{
						{ID: chunk1ID, Index: 0, Text: "First chunk", TokenCount: 2},
						{ID: chunk2ID, Index: 1, Text: "Second chunk", TokenCount: 2},
					}, nil).Once()

				// Expect combined text
				l.On("Summarize", mock.Anything, "First chunk\nSecond chunk\n").
					Return("Combined summary", []string{"Point 1", "Point 2"}, nil).Once()

				s.On("SaveSummary", mock.Anything, validDocID, mock.Anything).Return(nil).Once()

				// Expect batch embedder called with enriched chunk texts
				e.On("EmbedBatch", []string{"Document: test.pdf\n\nFirst chunk", "Document: test.pdf\n\nSecond chunk"}).
					Return([]embeddings.Vector{{0.1}, {0.2}}, nil).Once()

				// Expect SaveEmbeddings (batch) called with 2 embeddings
				s.On("SaveEmbeddings", mock.Anything, mock.MatchedBy(func(embs []store.Embedding) bool {
					return len(embs) == 2
				})).Return(nil).Once()

				s.On("UpdateDocumentStatus", mock.Anything, validDocID, store.StatusReady).
					Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name: "invalid document ID returns error",
			payload: analyzeTaskPayload{
				DocumentID: "invalid-uuid",
				ChunkIDs:   []uuid.UUID{chunk1ID},
			},
			setup:   func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {},
			wantErr: true,
		},
		{
			name: "store ListChunks failure propagates error",
			payload: analyzeTaskPayload{
				DocumentID: validDocID.String(),
				ChunkIDs:   []uuid.UUID{chunk1ID},
			},
			setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
				s.On("ListChunks", mock.Anything, validDocID).
					Return(nil, errors.New("database error")).Once()
			},
			wantErr: true,
		},
		{
			name: "LLM Summarize failure propagates error",
			payload: analyzeTaskPayload{
				DocumentID: validDocID.String(),
				ChunkIDs:   []uuid.UUID{chunk1ID},
			},
			setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
				s.On("ListChunks", mock.Anything, validDocID).
					Return([]store.Chunk{{ID: chunk1ID, Text: "Test", TokenCount: 1}}, nil).Once()

				l.On("Summarize", mock.Anything, mock.Anything).
					Return("", []string{}, errors.New("LLM error")).Once()
			},
			wantErr: true,
		},
		{
			name: "EmbedBatch failure propagates error",
			payload: analyzeTaskPayload{
				DocumentID: validDocID.String(),
				ChunkIDs:   []uuid.UUID{chunk1ID},
			},
			setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
				s.On("GetDocument", mock.Anything, validDocID).
					Return(store.Document{ID: validDocID, Filename: "test.pdf"}, nil).Once()

				s.On("ListChunks", mock.Anything, validDocID).
					Return([]store.Chunk{{ID: chunk1ID, Text: "Test", TokenCount: 1}}, nil).Once()

				l.On("Summarize", mock.Anything, mock.Anything).
					Return("Summary", []string{"Point"}, nil).Once()

				s.On("SaveSummary", mock.Anything, validDocID, mock.Anything).Return(nil).Once()

				// EmbedBatch fails
				e.On("EmbedBatch", []string{"Document: test.pdf\n\nTest"}).
					Return(nil, errors.New("embedding API error")).Once()
			},
			wantErr: true,
		},
		{
			name: "store SaveEmbeddings failure propagates error",
			payload: analyzeTaskPayload{
				DocumentID: validDocID.String(),
				ChunkIDs:   []uuid.UUID{chunk1ID},
			},
			setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
				s.On("GetDocument", mock.Anything, validDocID).
					Return(store.Document{ID: validDocID, Filename: "test.pdf"}, nil).Once()

				s.On("ListChunks", mock.Anything, validDocID).
					Return([]store.Chunk{{ID: chunk1ID, Text: "Test", TokenCount: 1}}, nil).Once()

				l.On("Summarize", mock.Anything, mock.Anything).
					Return("Summary", []string{"Point"}, nil).Once()

				s.On("SaveSummary", mock.Anything, validDocID, mock.Anything).Return(nil).Once()

				e.On("EmbedBatch", []string{"Document: test.pdf\n\nTest"}).
					Return([]embeddings.Vector{{0.1}}, nil).Once()

				// SaveEmbeddings fails
				s.On("SaveEmbeddings", mock.Anything, mock.Anything).
					Return(errors.New("embedding save error")).Once()
			},
			wantErr: true,
		},
		{
			name: "missing chunks returns empty text for summarization",
			payload: analyzeTaskPayload{
				DocumentID: validDocID.String(),
				ChunkIDs:   []uuid.UUID{chunk1ID},
			},
			setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
				s.On("GetDocument", mock.Anything, validDocID).
					Return(store.Document{ID: validDocID, Filename: "test.pdf"}, nil).Once()

				// Return empty chunks
				s.On("ListChunks", mock.Anything, validDocID).Return([]store.Chunk{}, nil).Once()

				// LLM should still be called with empty text
				l.On("Summarize", mock.Anything, "").Return("No content", []string{}, nil).Once()

				s.On("SaveSummary", mock.Anything, validDocID, mock.Anything).Return(nil).Once()

				// EmbedBatch called with empty texts array
				e.On("EmbedBatch", []string{}).Return([]embeddings.Vector{}, nil).Once()

				// SaveEmbeddings called with empty slice
				s.On("SaveEmbeddings", mock.Anything, []store.Embedding{}).Return(nil).Once()

				s.On("UpdateDocumentStatus", mock.Anything, validDocID, store.StatusReady).
					Return(nil).Once()
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mocks for each test
			mockStore := new(store.MockStore)
			mockLLM := new(llm.MockClient)
			mockEmbedder := new(embeddings.MockEmbedder)

			// Setup expectations
			if tt.setup != nil {
				tt.setup(mockStore, mockLLM, mockEmbedder)
			}

			// Create test dependencies
			deps := newTestDeps(mockStore, mockLLM, mockEmbedder)

			// Execute
			err := handleAnalyze(context.Background(), deps, tt.payload)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("handleAnalyze() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Assert all expectations were met
			mockStore.AssertExpectations(t)
			mockLLM.AssertExpectations(t)
			mockEmbedder.AssertExpectations(t)
		})
	}
}
