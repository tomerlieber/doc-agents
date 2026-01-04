package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

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
	deps, err := app.BuildAnalysis()
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

func handleAnalyze(ctx context.Context, deps app.AnalysisDeps, payload analyzeTaskPayload) error {
	// Parse and fetch chunks
	docID, err := uuid.Parse(payload.DocumentID)
	if err != nil {
		return err
	}

	chunks, err := deps.Store.ListChunks(ctx, docID)
	if err != nil {
		return err
	}

	// Generate and save summary
	text := concatenateChunks(chunks)
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

	// Generate and save embeddings with contextual enrichment
	// Get document for contextual enrichment
	doc, err := deps.Store.GetDocument(ctx, docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		// Enrich chunk with document context for better embeddings
		texts[i] = fmt.Sprintf("Document: %s\n\n%s", doc.Filename, c.Text)
	}
	vectors, err := deps.Embedder.EmbedBatch(texts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}
	embeddings := make([]store.Embedding, len(chunks))
	for i, c := range chunks {
		embeddings[i] = store.Embedding{
			ChunkID: c.ID,
			Vector:  vectors[i],
			Model:   deps.Config.EmbeddingModel,
		}
	}
	if err := deps.Store.SaveEmbeddings(ctx, embeddings); err != nil {
		return err
	}

	// Mark document ready
	return deps.Store.UpdateDocumentStatus(ctx, docID, store.StatusReady)
}

// concatenateChunks combines all chunk texts into a single string for summarization.
func concatenateChunks(chunks []store.Chunk) string {
	var builder strings.Builder
	for _, c := range chunks {
		builder.WriteString(c.Text)
		builder.WriteString("\n")
	}
	return builder.String()
}
