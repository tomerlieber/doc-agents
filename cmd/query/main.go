package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"doc-agents/internal/app"
	"doc-agents/internal/cache"
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
	deps, err := app.BuildQuery()
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

func queryHandler(deps app.QueryDeps) http.HandlerFunc {
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

		// Check cache first
		cacheKey := cache.GenerateCacheKey(req.Question, req.DocumentIDs, req.TopK)
		if cached, err := deps.Cache.GetQueryResult(ctx, cacheKey); err == nil && cached != nil {
			deps.Log.Info("cache hit", "question", req.Question)

			var sources []source
			if err := json.Unmarshal(cached.Sources, &sources); err == nil {
				httputil.WriteJSON(w, http.StatusOK, map[string]any{
					"answer":     cached.Answer,
					"sources":    sources,
					"confidence": cached.Confidence,
					"cached":     true,
				})
				return
			}
			// If unmarshaling fails, proceed with normal flow
			deps.Log.Warn("failed to unmarshal cached sources", "err", err)
		}

		// Cache miss - proceed with normal flow
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
		contextQuality := calculateAvgSimilarity(results)
		answer, confidence, err := deps.LLM.Answer(ctx, req.Question, context, contextQuality)
		if err != nil {
			httputil.Fail(deps.Log, w, "llm failed", err, http.StatusInternalServerError)
			return
		}

		sources := buildSources(results)

		// Store in cache
		sourcesJSON, err := json.Marshal(sources)
		if err != nil {
			deps.Log.Warn("failed to marshal sources, skipping cache", "err", err)
		} else {
			cacheTTL := time.Duration(deps.Config.CacheTTL) * time.Second
			if err := deps.Cache.SetQueryResult(ctx, cacheKey, &cache.QueryResult{
				Answer:     answer,
				Confidence: confidence,
				Sources:    sourcesJSON,
			}, cacheTTL); err != nil {
				// Log cache write failure but don't fail the request
				deps.Log.Warn("failed to cache result", "err", err)
			}
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"answer":     answer,
			"sources":    sources,
			"confidence": confidence,
			"cached":     false,
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

// calculateAvgSimilarity computes the average similarity score from search results.
// Returns 0.0 if no results.
func calculateAvgSimilarity(results []store.SearchResult) float32 {
	if len(results) == 0 {
		return 0.0
	}
	var sum float32
	for _, res := range results {
		sum += res.Score
	}
	return sum / float32(len(results))
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
