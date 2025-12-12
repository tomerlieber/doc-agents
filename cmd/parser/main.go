package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"doc-agents/internal/app"
	"doc-agents/internal/chunker"
	"doc-agents/internal/httputil"
	"doc-agents/internal/queue"
	"doc-agents/internal/store"
)

type parseTaskPayload struct {
	DocumentID string `json:"document_id"`
	Filename   string `json:"filename"`
	Content    string `json:"content"`
}

func main() {
	deps, err := app.Build()
	if err != nil {
		slog.Default().Error("failed to build dependencies", "err", err)
		os.Exit(1)
	}
	deps.Log.Info("parser worker starting")

	g, ctx := errgroup.WithContext(context.Background())

	// Run queue worker
	g.Go(func() error {
		return deps.Queue.Worker(ctx, queue.TaskTypeParse, func(ctx context.Context, task queue.Task) error {
			var payload parseTaskPayload
			if err := json.Unmarshal(task.Payload, &payload); err != nil {
				return err
			}
			return handleParse(ctx, deps, payload)
		})
	})

	// Run health check server
	g.Go(func() error {
		return httputil.ServeHealth(deps, "parser")
	})

	// Wait for either to fail
	if err := g.Wait(); err != nil {
		deps.Log.Error("parser service stopped", "err", err)
	}
}

func handleParse(ctx context.Context, deps app.Deps, payload parseTaskPayload) error {
	docID, err := uuid.Parse(payload.DocumentID)
	if err != nil {
		return err
	}
	text := payload.Content
	chunks := chunker.ChunkText(text, chunker.Options{MaxTokens: 400, Overlap: 80})
	var storeChunks []store.Chunk
	for _, c := range chunks {
		storeChunks = append(storeChunks, store.Chunk{
			Index:      c.Index,
			Text:       c.Text,
			TokenCount: c.TokenCount,
		})
	}
	chunksWithIDs, err := deps.Store.SaveChunks(ctx, docID, storeChunks)
	if err != nil {
		return err
	}
	// Enqueue analysis task with chunk ids.
	var chunkIDs []uuid.UUID
	for _, c := range chunksWithIDs {
		chunkIDs = append(chunkIDs, c.ID)
	}
	body, err := json.Marshal(map[string]any{
		"document_id": docID.String(),
		"chunk_ids":   chunkIDs,
	})
	if err != nil {
		return err
	}
	task := queue.Task{Type: queue.TaskTypeAnalyze, Payload: body, NotBefore: time.Now()}
	return queue.EnqueueWithRetry(ctx, deps.Queue, task, 3, 200*time.Millisecond)
}
