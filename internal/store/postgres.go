package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lib/pq"

	"doc-agents/internal/embeddings"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgres(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	s := &PostgresStore{db: db}
	if err := s.migrate(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PostgresStore) migrate(ctx context.Context) error {
	// Use advisory lock to prevent concurrent migrations from multiple services.
	// Note: In production, use dedicated migration tools (e.g., golang-migrate/migrate)
	// that run as a separate deployment step before app services start.
	const lockID = 123456789 // arbitrary number for this application's migration lock

	var acquired bool
	err := s.db.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, lockID).Scan(&acquired)
	if err != nil {
		return fmt.Errorf("failed to acquire migration lock: %w", err)
	}

	if !acquired {
		// Another service is running migrations; wait briefly and skip
		time.Sleep(2 * time.Second)
		return nil
	}

	// Ensure lock is released when done
	defer func() {
		_, _ = s.db.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID)
	}()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS documents (
			id UUID PRIMARY KEY,
			filename TEXT,
			status TEXT,
			created_at TIMESTAMPTZ DEFAULT now()
		);`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id UUID PRIMARY KEY,
			document_id UUID REFERENCES documents(id) ON DELETE CASCADE,
			ord INT,
			text TEXT,
			token_count INT
		);`,
		`CREATE TABLE IF NOT EXISTS summaries (
			document_id UUID PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
			summary TEXT,
			key_points TEXT[]
		);`,
		`CREATE TABLE IF NOT EXISTS embeddings (
			chunk_id UUID PRIMARY KEY REFERENCES chunks(id) ON DELETE CASCADE,
			vector JSONB,
			model TEXT
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) CreateDocument(ctx context.Context, filename string) (Document, error) {
	id := uuid.New()
	_, err := s.db.ExecContext(ctx, `INSERT INTO documents(id, filename, status) VALUES($1,$2,$3)`,
		id, filename, StatusProcessing)
	if err != nil {
		return Document{}, err
	}
	return Document{ID: id, Filename: filename, Status: StatusProcessing, CreatedAt: time.Now()}, nil
}

func (s *PostgresStore) UpdateDocumentStatus(ctx context.Context, id uuid.UUID, status DocumentStatus) error {
	res, err := s.db.ExecContext(ctx, `UPDATE documents SET status=$1 WHERE id=$2`, status, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("document not found")
	}
	return nil
}

func (s *PostgresStore) SaveChunks(ctx context.Context, docID uuid.UUID, chunks []Chunk) ([]Chunk, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := make([]Chunk, 0, len(chunks))
	for _, c := range chunks {
		cid := uuid.New()
		_, err := tx.ExecContext(ctx, `INSERT INTO chunks(id, document_id, ord, text, token_count) VALUES($1,$2,$3,$4,$5)`,
			cid, docID, c.Index, c.Text, c.TokenCount)
		if err != nil {
			return nil, err
		}
		c.ID = cid
		c.DocumentID = docID
		out = append(out, c)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *PostgresStore) SaveSummary(ctx context.Context, docID uuid.UUID, summary Summary) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO summaries(document_id, summary, key_points)
		VALUES($1,$2,$3)
		ON CONFLICT (document_id) DO UPDATE SET summary=excluded.summary, key_points=excluded.key_points`,
		docID, summary.Summary, pqStringArray(summary.KeyPoints))
	return err
}

func (s *PostgresStore) SaveEmbedding(ctx context.Context, emb Embedding) error {
	vecJSON, err := json.Marshal(emb.Vector)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO embeddings(chunk_id, vector, model)
		VALUES($1,$2,$3)
		ON CONFLICT (chunk_id) DO UPDATE SET vector=excluded.vector, model=excluded.model`,
		emb.ChunkID, vecJSON, emb.Model)
	return err
}

func (s *PostgresStore) GetSummary(ctx context.Context, docID uuid.UUID) (Summary, error) {
	var sum Summary
	var keyPoints []string
	row := s.db.QueryRowContext(ctx, `SELECT summary, key_points FROM summaries WHERE document_id=$1`, docID)
	if err := row.Scan(&sum.Summary, pq.Array(&keyPoints)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Summary{}, ErrSummaryNotFound
		}
		return Summary{}, fmt.Errorf("failed to get summary for doc %s: %w", docID, err)
	}
	sum.DocumentID = docID
	sum.KeyPoints = keyPoints
	return sum, nil
}

func (s *PostgresStore) TopK(ctx context.Context, docIDs []uuid.UUID, vector embeddings.Vector, k int) ([]SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.document_id, c.ord, c.text, c.token_count, e.vector, e.model,
		       COALESCE(s.summary,''), COALESCE(s.key_points, ARRAY[]::TEXT[])
		FROM embeddings e
		JOIN chunks c ON c.id = e.chunk_id
		LEFT JOIN summaries s ON s.document_id = c.document_id
		WHERE c.document_id = ANY($1)`, pqUUIDArray(docIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []SearchResult
	for rows.Next() {
		var (
			chunkID    uuid.UUID
			docID      uuid.UUID
			ord        int
			text       string
			tokens     int
			vecJSON    []byte
			model      string
			summaryTxt string
			keyPoints  []string
		)
		if err := rows.Scan(&chunkID, &docID, &ord, &text, &tokens, &vecJSON, &model, &summaryTxt, pq.Array(&keyPoints)); err != nil {
			return nil, err
		}
		var vec embeddings.Vector
		if err := json.Unmarshal(vecJSON, &vec); err != nil {
			return nil, err
		}
		score := embeddings.CosineSimilarity(vector, vec)
		results = append(results, SearchResult{
			Chunk: Chunk{
				ID:         chunkID,
				DocumentID: docID,
				Index:      ord,
				Text:       text,
				TokenCount: tokens,
			},
			Score: score,
			Summary: Summary{
				DocumentID: docID,
				Summary:    summaryTxt,
				KeyPoints:  keyPoints,
			},
		})
	}
	// naive sort
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	if k > len(results) {
		k = len(results)
	}
	return results[:k], nil
}

func (s *PostgresStore) ListChunks(ctx context.Context, docID uuid.UUID) ([]Chunk, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, ord, text, token_count FROM chunks WHERE document_id=$1`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.Index, &c.Text, &c.TokenCount); err != nil {
			return nil, err
		}
		c.DocumentID = docID
		out = append(out, c)
	}
	return out, nil
}

func pqStringArray(items []string) any {
	if len(items) == 0 {
		return []string{}
	}
	return items
}

func pqUUIDArray(items []uuid.UUID) any {
	if len(items) == 0 {
		return []uuid.UUID{}
	}
	return items
}
