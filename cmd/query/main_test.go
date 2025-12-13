package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"doc-agents/internal/app"
	"doc-agents/internal/config"
	"doc-agents/internal/embeddings"
	"doc-agents/internal/llm"
	"doc-agents/internal/store"
)

func newTestDeps(st store.Store, l llm.Client, e embeddings.Embedder) app.Deps {
	return app.Deps{
		Store:    st,
		LLM:      l,
		Embedder: e,
		Config: config.Config{
			EmbeddingModel: "test-model",
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestQueryHandler(t *testing.T) {
	validDocID := uuid.New()
	chunk1ID := uuid.New()

	tests := []struct {
		name           string
		requestBody    string
		setup          func(*store.MockStore, *llm.MockClient, *embeddings.MockEmbedder)
		wantStatusCode int
		checkResponse  func(*testing.T, *http.Response)
	}{
		{
			name: "successful query with results",
			requestBody: `{
				"question": "What is Go?",
				"document_ids": ["` + validDocID.String() + `"],
				"top_k": 3
			}`,
			setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
			// Expect Embed to be called for the question
			e.On("Embed", "What is Go?").Return(embeddings.Vector{0.1, 0.2}, nil).Once()

				// Expect TopK search
				s.On("TopK", mock.Anything, mock.MatchedBy(func(ids []uuid.UUID) bool {
					return len(ids) == 1 && ids[0] == validDocID
				}), mock.Anything, 3).Return([]store.SearchResult{
					{
						Chunk: store.Chunk{ID: chunk1ID, Text: "Go is a programming language", TokenCount: 5},
						Score: 0.95,
					},
				}, nil).Once()

				// Expect LLM.Answer to be called
				l.On("Answer", mock.Anything, "What is Go?", mock.Anything).
					Return("Go is a programming language developed by Google", float64(0.95), nil).Once()
			},
			wantStatusCode: http.StatusOK,
			checkResponse: func(t *testing.T, resp *http.Response) {
				var result map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if _, ok := result["answer"]; !ok {
					t.Error("Expected answer in response")
				}
				if _, ok := result["confidence"]; !ok {
					t.Error("Expected confidence in response")
				}
				if _, ok := result["sources"]; !ok {
					t.Error("Expected sources in response")
				}
			},
		},
		{
			name: "TopK defaults to 5 when omitted",
			requestBody: `{
				"question": "What is Go?",
				"document_ids": ["` + validDocID.String() + `"]
			}`,
		setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
			e.On("Embed", "What is Go?").Return(embeddings.Vector{0.1}, nil).Once()

				// Expect TopK=5 (default)
				s.On("TopK", mock.Anything, mock.Anything, mock.Anything, 5).
					Return([]store.SearchResult{}, nil).Once()

				l.On("Answer", mock.Anything, mock.Anything, mock.Anything).
					Return("Answer", float64(0.8), nil).Once()
			},
			wantStatusCode: http.StatusOK,
			checkResponse:  func(t *testing.T, resp *http.Response) {},
		},
		{
			name:           "invalid JSON payload returns 400",
			requestBody:    `{invalid json}`,
			setup:          func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {},
			wantStatusCode: http.StatusBadRequest,
			checkResponse:  func(t *testing.T, resp *http.Response) {},
		},
		{
			name: "empty question fails validation",
			requestBody: `{
				"question": "",
				"document_ids": ["` + validDocID.String() + `"]
			}`,
			setup:          func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {},
			wantStatusCode: http.StatusBadRequest,
			checkResponse:  func(t *testing.T, resp *http.Response) {},
		},
		{
			name: "question too short fails validation",
			requestBody: `{
				"question": "Hi",
				"document_ids": ["` + validDocID.String() + `"]
			}`,
			setup:          func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {},
			wantStatusCode: http.StatusBadRequest,
			checkResponse:  func(t *testing.T, resp *http.Response) {},
		},
		{
			name: "invalid UUID in document_ids fails validation",
			requestBody: `{
				"question": "Valid question here",
				"document_ids": ["not-a-uuid"]
			}`,
			setup:          func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {},
			wantStatusCode: http.StatusBadRequest,
			checkResponse:  func(t *testing.T, resp *http.Response) {},
		},
		{
			name: "empty document_ids fails validation",
			requestBody: `{
				"question": "Valid question",
				"document_ids": []
			}`,
			setup:          func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {},
			wantStatusCode: http.StatusBadRequest,
			checkResponse:  func(t *testing.T, resp *http.Response) {},
		},
		{
			name: "top_k above max fails validation",
			requestBody: `{
				"question": "Valid question",
				"document_ids": ["` + validDocID.String() + `"],
				"top_k": 25
			}`,
			setup:          func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {},
			wantStatusCode: http.StatusBadRequest,
			checkResponse:  func(t *testing.T, resp *http.Response) {},
		},
		{
			name: "store TopK failure returns 500",
			requestBody: `{
				"question": "What is Go?",
				"document_ids": ["` + validDocID.String() + `"]
			}`,
		setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
			e.On("Embed", "What is Go?").Return(embeddings.Vector{0.1}, nil).Once()
				s.On("TopK", mock.Anything, mock.Anything, mock.Anything, 5).
					Return(nil, errors.New("database error")).Once()
			},
			wantStatusCode: http.StatusInternalServerError,
			checkResponse:  func(t *testing.T, resp *http.Response) {},
		},
		{
			name: "LLM Answer failure returns 500",
			requestBody: `{
				"question": "What is Go?",
				"document_ids": ["` + validDocID.String() + `"]
			}`,
		setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
			e.On("Embed", "What is Go?").Return(embeddings.Vector{0.1}, nil).Once()
				s.On("TopK", mock.Anything, mock.Anything, mock.Anything, 5).
					Return([]store.SearchResult{}, nil).Once()
				l.On("Answer", mock.Anything, mock.Anything, mock.Anything).
					Return("", float64(0), errors.New("LLM error")).Once()
			},
			wantStatusCode: http.StatusInternalServerError,
			checkResponse:  func(t *testing.T, resp *http.Response) {},
		},
		{
			name: "no search results still returns answer",
			requestBody: `{
				"question": "What is Go?",
				"document_ids": ["` + uuid.New().String() + `"]
			}`,
		setup: func(s *store.MockStore, l *llm.MockClient, e *embeddings.MockEmbedder) {
			e.On("Embed", "What is Go?").Return(embeddings.Vector{0.1}, nil).Once()
				s.On("TopK", mock.Anything, mock.Anything, mock.Anything, 5).
					Return([]store.SearchResult{}, nil).Once()
				l.On("Answer", mock.Anything, "What is Go?", "").
					Return("I don't have enough context", float64(0.3), nil).Once()
			},
			wantStatusCode: http.StatusOK,
			checkResponse: func(t *testing.T, resp *http.Response) {
				var result map[string]any
				json.NewDecoder(resp.Body).Decode(&result)

				sources, ok := result["sources"].([]any)
				if !ok {
					t.Error("Expected sources array")
				}
				if len(sources) != 0 {
					t.Error("Expected empty sources for no results")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mocks
			mockStore := new(store.MockStore)
			mockLLM := new(llm.MockClient)
			mockEmbedder := new(embeddings.MockEmbedder)

			// Setup expectations
			if tt.setup != nil {
				tt.setup(mockStore, mockLLM, mockEmbedder)
			}

			// Create test dependencies
			deps := newTestDeps(mockStore, mockLLM, mockEmbedder)

			// Create handler
			handler := queryHandler(deps)

			// Create request
			req := httptest.NewRequest(http.MethodPost, "/api/query", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			w := httptest.NewRecorder()

			// Execute
			handler(w, req)

			// Check status code
			resp := w.Result()
			if resp.StatusCode != tt.wantStatusCode {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected status %d, got %d. Body: %s", tt.wantStatusCode, resp.StatusCode, string(body))
			}

			// Additional response checks
			if tt.checkResponse != nil {
				resp.Body = io.NopCloser(bytes.NewReader(w.Body.Bytes()))
				tt.checkResponse(t, resp)
			}

			// Assert all expectations were met
			mockStore.AssertExpectations(t)
			mockLLM.AssertExpectations(t)
			mockEmbedder.AssertExpectations(t)
		})
	}
}
