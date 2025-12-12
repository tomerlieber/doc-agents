package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"doc-agents/internal/app"
	"doc-agents/internal/config"
	"doc-agents/internal/queue"
	"doc-agents/internal/store"
)

func newTestDeps(st store.Store, q queue.Queue) app.Deps {
	return app.Deps{
		Store: st,
		Queue: q,
		Config: config.Config{
			EmbeddingModel: "test-model",
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestHandleParse(t *testing.T) {
	validDocID := uuid.New()

	tests := []struct {
		name    string
		payload parseTaskPayload
		setup   func(*store.MockStore, *queue.MockQueue)
		wantErr bool
	}{
		{
			name: "successful parse with small text",
			payload: parseTaskPayload{
				DocumentID: validDocID.String(),
				Filename:   "test.txt",
				Content:    "This is a short test document.",
			},
			setup: func(s *store.MockStore, q *queue.MockQueue) {
				// Expect SaveChunks to be called with any chunks
				s.On("SaveChunks", mock.Anything, validDocID, mock.MatchedBy(func(chunks []store.Chunk) bool {
					return len(chunks) > 0
				})).Return([]store.Chunk{{ID: uuid.New(), DocumentID: validDocID}}, nil).Once()

				// Expect Enqueue to be called with analyze task
				q.On("Enqueue", mock.Anything, mock.MatchedBy(func(task queue.Task) bool {
					if task.Type != queue.TaskTypeAnalyze {
						return false
					}
					var payload map[string]any
					json.Unmarshal(task.Payload, &payload)
					return payload["document_id"] == validDocID.String()
				})).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name: "successful parse with long text creates multiple chunks",
			payload: parseTaskPayload{
				DocumentID: validDocID.String(),
				Filename:   "long.txt",
				Content:    generateLongText(1000),
			},
			setup: func(s *store.MockStore, q *queue.MockQueue) {
				// Expect multiple chunks
				s.On("SaveChunks", mock.Anything, validDocID, mock.MatchedBy(func(chunks []store.Chunk) bool {
					return len(chunks) > 1 // Verify multiple chunks
				})).Return([]store.Chunk{{ID: uuid.New()}}, nil).Once()

				q.On("Enqueue", mock.Anything, mock.Anything).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name: "invalid document ID returns error",
			payload: parseTaskPayload{
				DocumentID: "invalid-uuid",
				Filename:   "test.txt",
				Content:    "Test content",
			},
			setup:   func(s *store.MockStore, q *queue.MockQueue) {},
			wantErr: true,
		},
		{
			name: "store SaveChunks failure propagates error",
			payload: parseTaskPayload{
				DocumentID: validDocID.String(),
				Filename:   "test.txt",
				Content:    "Test content",
			},
			setup: func(s *store.MockStore, q *queue.MockQueue) {
				s.On("SaveChunks", mock.Anything, validDocID, mock.Anything).
					Return(nil, errors.New("database error")).Once()
				// Enqueue should NOT be called
			},
			wantErr: true,
		},
		{
			name: "queue enqueue failure returns error",
			payload: parseTaskPayload{
				DocumentID: validDocID.String(),
				Filename:   "test.txt",
				Content:    "Test content",
			},
			setup: func(s *store.MockStore, q *queue.MockQueue) {
				s.On("SaveChunks", mock.Anything, validDocID, mock.Anything).
					Return([]store.Chunk{{ID: uuid.New()}}, nil).Once()

				// Enqueue fails (may be retried)
				q.On("Enqueue", mock.Anything, mock.Anything).
					Return(errors.New("queue error"))
			},
			wantErr: true,
		},
		{
			name: "empty content still enqueues analysis task",
			payload: parseTaskPayload{
				DocumentID: validDocID.String(),
				Filename:   "empty.txt",
				Content:    "",
			},
			setup: func(s *store.MockStore, q *queue.MockQueue) {
				s.On("SaveChunks", mock.Anything, validDocID, mock.Anything).
					Return([]store.Chunk{}, nil).Once()

				q.On("Enqueue", mock.Anything, mock.Anything).Return(nil).Once()
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mocks for each test
			mockStore := new(store.MockStore)
			mockQueue := new(queue.MockQueue)

			// Setup expectations
			if tt.setup != nil {
				tt.setup(mockStore, mockQueue)
			}

			// Create test dependencies
			deps := newTestDeps(mockStore, mockQueue)

			// Execute
			err := handleParse(context.Background(), deps, tt.payload)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("handleParse() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Assert all expectations were met
			mockStore.AssertExpectations(t)
			mockQueue.AssertExpectations(t)
		})
	}
}

// generateLongText creates text of approximately the specified word count.
func generateLongText(words int) string {
	text := ""
	for i := 0; i < words; i++ {
		text += "word "
	}
	return text
}
