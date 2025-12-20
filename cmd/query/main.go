package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"

	"doc-agents/internal/app"
	"doc-agents/internal/httputil"
	"doc-agents/internal/store"
)

type queryRequest struct {
	Question    string   `json:"question" validate:"required,min=3,max=500"`
	DocumentIDs []string `json:"document_ids" validate:"required,min=1,dive,uuid4"`
	TopK        int      `json:"top_k" validate:"omitempty,min=1,max=20"`
}

type source struct {
	ChunkID string  `json:"chunk_id"`
	Score   float32 `json:"score"`
	Preview string  `json:"preview"` // Truncated text preview
}

func main() {
	deps, err := app.Build()
	if err != nil {
		slog.Default().Error("failed to build dependencies", "err", err)
		os.Exit(1)
	}
	r := httputil.NewRouter(deps.Log)

	r.Post("/api/query", queryHandler(deps))
	r.Get("/healthz", httputil.HealthHandler(deps))

	addr := fmt.Sprintf(":%d", deps.Config.Port)
	deps.Log.Info("query service listening", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		deps.Log.Error("server error", "err", err)
	}
}

func queryHandler(deps app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req queryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.Fail(deps.Log, w, "invalid payload", err, http.StatusBadRequest)
			return
		}

		// Validate request
		if err := httputil.Validator.Struct(&req); err != nil {
			httputil.ValidationError(deps.Log, w, err)
			return
		}

		if req.TopK == 0 {
			req.TopK = 5
		}

		ctx := r.Context()

		// Embed question and search for relevant chunks
		ids := parseDocumentIDs(req.DocumentIDs)
		vec, err := deps.Embedder.Embed(req.Question)
		if err != nil {
			httputil.Fail(deps.Log, w, "failed to embed question", err, http.StatusInternalServerError)
			return
		}
		results, err := deps.Store.TopK(ctx, ids, vec, req.TopK)
		if err != nil {
			httputil.Fail(deps.Log, w, "search failed", err, http.StatusInternalServerError)
			return
		}

		// Get LLM answer with context from search results (filtered by database)
		context := buildContext(results)
		answer, confidence, err := deps.LLM.Answer(ctx, req.Question, context)
		if err != nil {
			httputil.Fail(deps.Log, w, "llm failed", err, http.StatusInternalServerError)
			return
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"answer":     answer,
			"sources":    buildSources(results),
			"confidence": confidence,
		})
	}
}

// parseDocumentIDs converts string UUIDs to uuid.UUID slice, skipping invalid ones.
func parseDocumentIDs(ids []string) []uuid.UUID {
	var result []uuid.UUID
	for _, s := range ids {
		if id, err := uuid.Parse(s); err == nil {
			result = append(result, id)
		}
	}
	return result
}

// buildContext concatenates chunk texts from search results for LLM context.
func buildContext(results []store.SearchResult) string {
	var builder strings.Builder
	for _, res := range results {
		builder.WriteString(res.Chunk.Text)
		builder.WriteString("\n")
	}
	return builder.String()
}

// buildSources converts search results into source structs with truncated previews.
func buildSources(results []store.SearchResult) []source {
	sources := make([]source, len(results))
	for i, res := range results {
		sources[i] = source{
			ChunkID: res.Chunk.ID.String(),
			Score:   res.Score,
			Preview: truncate(res.Chunk.Text, 150),
		}
	}
	return sources
}

// truncate limits text to maxLen characters, cutting at word boundary.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Find last space before maxLen to avoid cutting words
	if idx := strings.LastIndex(s[:maxLen], " "); idx > 0 {
		return s[:idx] + "..."
	}
	return s[:maxLen] + "..."
}
