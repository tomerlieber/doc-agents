package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"doc-agents/internal/embeddings"
)

type DocumentStatus string

const (
	StatusProcessing DocumentStatus = "processing"
	StatusReady      DocumentStatus = "ready"
	StatusFailed     DocumentStatus = "failed"
)

var ErrSummaryNotFound = errors.New("summary not found")

type Document struct {
	ID        uuid.UUID
	Filename  string
	Status    DocumentStatus
	CreatedAt time.Time
}

type Chunk struct {
	ID         uuid.UUID
	DocumentID uuid.UUID
	Index      int
	Text       string
	TokenCount int
}

type Summary struct {
	DocumentID uuid.UUID
	Summary    string
	KeyPoints  []string
}

type Embedding struct {
	ChunkID uuid.UUID
	Vector  embeddings.Vector
	Model   string
}

type SearchResult struct {
	Chunk   Chunk
	Score   float32
	Summary Summary
}

// Store defines persistence contract; an external DB implementation can replace this.
type Store interface {
	CreateDocument(ctx context.Context, filename string) (Document, error)
	GetDocument(ctx context.Context, id uuid.UUID) (Document, error)
	UpdateDocumentStatus(ctx context.Context, id uuid.UUID, status DocumentStatus) error
	SaveChunks(ctx context.Context, docID uuid.UUID, chunks []Chunk) ([]Chunk, error)
	ListChunks(ctx context.Context, docID uuid.UUID) ([]Chunk, error)
	SaveSummary(ctx context.Context, docID uuid.UUID, summary Summary) error
	SaveEmbeddings(ctx context.Context, embs []Embedding) error
	GetSummary(ctx context.Context, docID uuid.UUID) (Summary, error)
	TopK(ctx context.Context, docIDs []uuid.UUID, vector embeddings.Vector, k int) ([]SearchResult, error)
}
