package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"doc-agents/internal/app"
	"doc-agents/internal/httputil"
	"doc-agents/internal/queue"
	"doc-agents/internal/store"
)

type analyzeTaskPayload struct {
	DocumentID string      `json:"document_id"`
	ChunkIDs   []uuid.UUID `json:"chunk_ids"`
}

func main() {
	deps, err := app.Build()
	if err != nil {
		slog.Default().Error("failed to build dependencies", "err", err)
		os.Exit(1)
	}
	deps.Log.Info("analysis worker starting")

	g, ctx := errgroup.WithContext(context.Background())

	// Run queue worker
	g.Go(func() error {
		return deps.Queue.Worker(ctx, queue.TaskTypeAnalyze, func(ctx context.Context, task queue.Task) error {
			var payload analyzeTaskPayload
			if err := json.Unmarshal(task.Payload, &payload); err != nil {
				return err
			}
			return handleAnalyze(ctx, deps, payload)
		})
	})

	// Run health check server
	g.Go(func() error {
		return httputil.ServeHealth(deps, "analysis")
	})

	// Wait for either to fail
	if err := g.Wait(); err != nil {
		deps.Log.Error("analysis service stopped", "err", err)
	}
}

func handleAnalyze(ctx context.Context, deps app.Deps, payload analyzeTaskPayload) error {
	docID, err := uuid.Parse(payload.DocumentID)
	if err != nil {
		return err
	}
	chunks, err := deps.Store.ListChunks(ctx, docID)
	if err != nil {
		return err
	}
	text := ""
	for _, c := range chunks {
		text += c.Text + "\n"
	}
	summaryText, keyPoints, err := deps.LLM.Summarize(ctx, text)
	if err != nil {
		return err
	}
	if err := deps.Store.SaveSummary(ctx, docID, store.Summary{
		Summary:   summaryText,
		KeyPoints: keyPoints,
	}); err != nil {
		return err
	}
	for _, c := range chunks {
		if err := deps.Store.SaveEmbedding(ctx, store.Embedding{
			ChunkID: c.ID,
			Vector:  deps.Embedder.Embed(c.Text),
			Model:   deps.Config.EmbeddingModel,
		}); err != nil {
			return err
		}
	}
	// Mark document ready.
	return deps.Store.UpdateDocumentStatus(ctx, docID, store.StatusReady)
}
