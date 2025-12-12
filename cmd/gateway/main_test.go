package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
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
			MaxUploadSize: 1024 * 1024, // 1MB for tests
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestUploadHandler(t *testing.T) {
	validDocID := uuid.New()

	tests := []struct {
		name          string
		filename      string
		contentType   string
		content       []byte
		setup         func(*store.MockStore, *queue.MockQueue)
		wantStatus    int
		checkResponse func(*testing.T, *http.Response)
	}{
		{
			name:        "successful upload",
			filename:    "test.txt",
			contentType: "text/plain",
			content:     []byte("Hello"),
			setup: func(s *store.MockStore, q *queue.MockQueue) {
				s.On("CreateDocument", mock.Anything, "test.txt").
					Return(store.Document{ID: validDocID, Status: store.StatusProcessing}, nil).Once()
				q.On("Enqueue", mock.Anything, mock.Anything).Return(nil).Once()
			},
			wantStatus: http.StatusAccepted,
			checkResponse: func(t *testing.T, resp *http.Response) {
				var result map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if result["document_id"] == "" {
					t.Error("Expected document_id in response")
				}
				if result["status"] != string(store.StatusProcessing) {
					t.Errorf("Expected status %s, got %v", store.StatusProcessing, result["status"])
				}
			},
		},
		{
			name:        "file too large",
			filename:    "large.txt",
			contentType: "text/plain",
			content:     make([]byte, 2*1024*1024), // 2MB
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "missing Content-Type detects from extension",
			filename:    "test.txt",
			contentType: "", // Empty, should detect from .txt
			content:     []byte("content"),
			setup: func(s *store.MockStore, q *queue.MockQueue) {
				s.On("CreateDocument", mock.Anything, "test.txt").
					Return(store.Document{ID: validDocID, Status: store.StatusProcessing}, nil).Once()
				q.On("Enqueue", mock.Anything, mock.Anything).Return(nil).Once()
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name:        "unsupported extension",
			filename:    "test.docx",
			contentType: "",
			content:     []byte("content"),
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "unsupported Content-Type",
			filename:    "test.doc",
			contentType: "application/msword",
			content:     []byte("content"),
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "CreateDocument failure",
			filename:    "test.txt",
			contentType: "text/plain",
			content:     []byte("content"),
			setup: func(s *store.MockStore, q *queue.MockQueue) {
				s.On("CreateDocument", mock.Anything, "test.txt").
					Return(store.Document{}, errors.New("db error")).Once()
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:        "Enqueue failure marks doc failed",
			filename:    "test.txt",
			contentType: "text/plain",
			content:     []byte("content"),
			setup: func(s *store.MockStore, q *queue.MockQueue) {
				s.On("CreateDocument", mock.Anything, "test.txt").
					Return(store.Document{ID: validDocID, Status: store.StatusProcessing}, nil).Once()
				q.On("Enqueue", mock.Anything, mock.Anything).Return(errors.New("queue error")).Times(3)
				s.On("UpdateDocumentStatus", mock.Anything, validDocID, store.StatusFailed).Return(nil).Once()
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(store.MockStore)
			mockQueue := new(queue.MockQueue)

			if tt.setup != nil {
				tt.setup(mockStore, mockQueue)
			}

			deps := newTestDeps(mockStore, mockQueue)
			handler := uploadHandler(deps)

			req, err := createMultipartRequest(tt.filename, tt.contentType, tt.content)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			w := httptest.NewRecorder()
			handler(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.wantStatus {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected status %d, got %d. Body: %s", tt.wantStatus, resp.StatusCode, string(body))
			}

			if tt.checkResponse != nil {
				resp.Body = io.NopCloser(bytes.NewReader(w.Body.Bytes()))
				tt.checkResponse(t, resp)
			}

			mockStore.AssertExpectations(t)
			mockQueue.AssertExpectations(t)
		})
	}

	// Test missing file separately since it requires different request setup
	t.Run("missing file", func(t *testing.T) {
		mockStore := new(store.MockStore)
		mockQueue := new(queue.MockQueue)
		deps := newTestDeps(mockStore, mockQueue)
		handler := uploadHandler(deps)

		req := httptest.NewRequest(http.MethodPost, "/api/documents/upload", nil)
		req.Header.Set("Content-Type", "multipart/form-data")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})
}

func TestSummaryHandler(t *testing.T) {
	validDocID := uuid.New()

	tests := []struct {
		name          string
		docID         string
		setup         func(*store.MockStore)
		wantStatus    int
		checkResponse func(*testing.T, *http.Response)
	}{
		{
			name:  "successful retrieval",
			docID: validDocID.String(),
			setup: func(s *store.MockStore) {
				s.On("GetSummary", mock.Anything, validDocID).
					Return(store.Summary{
						DocumentID: validDocID,
						Summary:    "Test summary",
						KeyPoints:  []string{"Point 1", "Point 2"},
					}, nil).Once()
			},
			wantStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *http.Response) {
				var result map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if result["summary"] == "" || result["documentId"] == "" {
					t.Error("Expected summary and documentId in response")
				}
				keyPoints, ok := result["key_points"].([]any)
				if !ok || len(keyPoints) != 2 {
					t.Errorf("Expected 2 key_points, got %v", result["key_points"])
				}
			},
		},
		{
			name:       "invalid UUID",
			docID:      "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "summary not found",
			docID: validDocID.String(),
			setup: func(s *store.MockStore) {
				s.On("GetSummary", mock.Anything, validDocID).
					Return(store.Summary{}, store.ErrSummaryNotFound).Once()
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:  "store error",
			docID: validDocID.String(),
			setup: func(s *store.MockStore) {
				s.On("GetSummary", mock.Anything, validDocID).
					Return(store.Summary{}, errors.New("db error")).Once()
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(store.MockStore)
			mockQueue := new(queue.MockQueue)

			if tt.setup != nil {
				tt.setup(mockStore)
			}

			deps := newTestDeps(mockStore, mockQueue)
			handler := summaryHandler(deps)

			req := httptest.NewRequest(http.MethodGet, "/api/documents/"+tt.docID+"/summary", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.docID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			handler(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.wantStatus {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected status %d, got %d. Body: %s", tt.wantStatus, resp.StatusCode, string(body))
			}

			if tt.checkResponse != nil {
				resp.Body = io.NopCloser(bytes.NewReader(w.Body.Bytes()))
				tt.checkResponse(t, resp)
			}

			mockStore.AssertExpectations(t)
			mockQueue.AssertExpectations(t)
		})
	}
}

func createMultipartRequest(filename, contentType string, content []byte) (*http.Request, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	h := make(map[string][]string)
	h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)}
	if contentType != "" {
		h["Content-Type"] = []string{contentType}
	}

	part, err := writer.CreatePart(h)
	if err != nil {
		return nil, err
	}

	if _, err := part.Write(content); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	req := httptest.NewRequest(http.MethodPost, "/api/documents/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req, nil
}
