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
)

type queryRequest struct {
	Question    string   `json:"question" validate:"required,min=3,max=500"`
	DocumentIDs []string `json:"document_ids" validate:"required,min=1,dive,uuid4"`
	TopK        int      `json:"top_k" validate:"omitempty,min=1,max=20"`
}

type source struct {
	ChunkID string  `json:"chunk_id"`
	Score   float32 `json:"score"`
	Text    string  `json:"text"`
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

		// Parse document IDs
		var ids []uuid.UUID
		for _, s := range req.DocumentIDs {
			if id, err := uuid.Parse(s); err == nil {
				ids = append(ids, id)
			}
		}

		ctx := r.Context()

		// Embed question and search for relevant chunks
		vec := deps.Embedder.Embed(req.Question)
		results, err := deps.Store.TopK(ctx, ids, vec, req.TopK)
		if err != nil {
			httputil.Fail(deps.Log, w, "search failed", err, http.StatusInternalServerError)
			return
		}

		// Build context from chunks
		var contextBuilder strings.Builder
		for _, res := range results {
			contextBuilder.WriteString(res.Chunk.Text)
			contextBuilder.WriteString("\n")
		}

		// Get LLM answer
		answer, confidence, err := deps.LLM.Answer(ctx, req.Question, contextBuilder.String())
		if err != nil {
			httputil.Fail(deps.Log, w, "llm failed", err, http.StatusInternalServerError)
			return
		}

		// Build response with sources
		sources := make([]source, len(results))
		for i, res := range results {
			sources[i] = source{
				ChunkID: res.Chunk.ID.String(),
				Score:   res.Score,
				Text:    res.Chunk.Text,
			}
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"answer":     answer,
			"sources":    sources,
			"confidence": confidence,
		})
	}
}
