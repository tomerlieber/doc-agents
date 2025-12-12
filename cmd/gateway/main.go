package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"

	"doc-agents/internal/app"
	"doc-agents/internal/httputil"
	"doc-agents/internal/queue"
	"doc-agents/internal/store"
)

type parseTaskPayload struct {
	DocumentID uuid.UUID `json:"document_id"`
	Filename   string    `json:"filename"`
	Content    string    `json:"content"`
}

func main() {
	deps, err := app.Build()
	if err != nil {
		slog.Default().Error("failed to build dependencies", "err", err)
		os.Exit(1)
	}
	r := httputil.NewRouter(deps.Log)

	r.Post("/api/documents/upload", uploadHandler(deps))
	r.Get("/api/documents/{id}/summary", summaryHandler(deps))
	r.Post("/api/query", queryHandler(deps))
	r.Get("/healthz", httputil.HealthHandler(deps))

	addr := fmt.Sprintf(":%d", deps.Config.Port)
	deps.Log.Info("gateway listening", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		deps.Log.Error("server failed", "err", err)
	}
}

func uploadHandler(deps app.Deps) http.HandlerFunc {
	maxFileSize := deps.Config.MaxUploadSize

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Validate file size before parsing
		if r.ContentLength > maxFileSize {
			httputil.Fail(deps.Log, w, fmt.Sprintf("file too large (max %d bytes)", maxFileSize), nil, http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			httputil.Fail(deps.Log, w, "file is required", err, http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Validate file size
		if header.Size > maxFileSize {
			httputil.Fail(deps.Log, w, fmt.Sprintf("file too large (max %d bytes)", maxFileSize), nil, http.StatusBadRequest)
			return
		}

		// Validate file type
		contentType := header.Header.Get("Content-Type")
		
		// If Content-Type is missing, detect from filename
		if contentType == "" {
			ext := strings.ToLower(filepath.Ext(header.Filename))
			switch ext {
			case ".txt":
				contentType = "text/plain"
			case ".pdf":
				contentType = "application/pdf"
			default:
				httputil.Fail(deps.Log, w, "unsupported file type (only PDF and TXT allowed)", nil, http.StatusBadRequest)
				return
			}
		}
		
		// Validate Content-Type
		allowedTypes := map[string]bool{
			"text/plain":      true,
			"application/pdf": true,
		}
		if !allowedTypes[contentType] {
			httputil.Fail(deps.Log, w, "unsupported file type (only PDF and TXT allowed)", nil, http.StatusBadRequest)
			return
		}

		content, err := io.ReadAll(file)
		if err != nil {
			httputil.Fail(deps.Log, w, "failed to read file", err, http.StatusInternalServerError)
			return
		}
		text := extractText(header.Filename, content, deps)

		doc, err := deps.Store.CreateDocument(ctx, header.Filename)
		if err != nil {
			httputil.Fail(deps.Log, w, "failed to persist document", err, http.StatusInternalServerError)
			return
		}

		payload := parseTaskPayload{
			DocumentID: doc.ID,
			Filename:   header.Filename,
			Content:    text,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			fail(deps, ctx, w, "marshal payload failed", err, doc.ID, http.StatusInternalServerError, true)
			return
		}
		task := queue.Task{Type: queue.TaskTypeParse, Payload: body}
		if err := queue.EnqueueWithRetry(ctx, deps.Queue, task, 3, 200*time.Millisecond); err != nil {
			fail(deps, ctx, w, "failed to enqueue document; please retry", err, doc.ID, http.StatusInternalServerError, true)
			return
		}

		httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
			"document_id": doc.ID.String(),
			"status":      doc.Status,
		})
	}
}

// fail is gateway-specific error handler that can mark documents as failed
func fail(deps app.Deps, ctx context.Context, w http.ResponseWriter, message string, err error, docID uuid.UUID, status int, markFailed bool) {
	log := deps.Log.With("document_id", docID)
	if markFailed && docID != uuid.Nil {
		if upErr := deps.Store.UpdateDocumentStatus(ctx, docID, store.StatusFailed); upErr != nil {
			log.Error("failed to mark document failed", "err", upErr)
		}
	}

	httputil.Fail(log, w, message, err, status)
}

func summaryHandler(deps app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		docID, err := uuid.Parse(idStr)
		if err != nil {
			httputil.Fail(deps.Log, w, "invalid document id", err, http.StatusBadRequest)
			return
		}
		sum, err := deps.Store.GetSummary(r.Context(), docID)
		if err != nil {
			fail(deps, r.Context(), w, "summary not ready", err, docID, http.StatusNotFound, false)
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"summary":    sum.Summary,
			"key_points": sum.KeyPoints,
			"documentId": docID,
		})
	}
}

func queryHandler(deps app.Deps) http.HandlerFunc {
	queryURL := "http://query:8081/api/query"
	client := &http.Client{Timeout: 60 * time.Second}

	return func(w http.ResponseWriter, r *http.Request) {
		// Forward request to query agent service
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, queryURL, r.Body)
		if err != nil {
			httputil.Fail(deps.Log, w, "failed to create request", err, http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			httputil.Fail(deps.Log, w, "query service unavailable", err, http.StatusServiceUnavailable)
			return
		}
		defer resp.Body.Close()

		// Copy response status, headers, and body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			deps.Log.Error("failed to copy response", "err", err)
		}
	}
}

// extractText extracts text from uploaded files, with PDF support.
func extractText(filename string, content []byte, deps app.Deps) string {
	if strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		text, err := extractPDF(content)
		if err != nil {
			deps.Log.Warn("pdf extraction failed, using raw bytes", "err", err, "filename", filename)
			return string(content)
		}
		return text
	}
	// Treat other files as plain text
	return string(content)
}

func extractPDF(content []byte) (string, error) {
	reader := bytes.NewReader(content)
	pdfReader, err := pdf.NewReader(reader, int64(len(content)))
	if err != nil {
		return "", err
	}

	var textBuilder strings.Builder
	numPages := pdfReader.NumPage()

	for pageNum := 1; pageNum <= numPages; pageNum++ {
		page := pdfReader.Page(pageNum)
		if page.V.IsNull() || page.V.Key("Contents").Kind() == pdf.Null {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			// Skip pages that fail to extract
			continue
		}
		textBuilder.WriteString(text)
		textBuilder.WriteString("\n")
	}

	return textBuilder.String(), nil
}
