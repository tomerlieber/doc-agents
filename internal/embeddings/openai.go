package embeddings

import (
	"context"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// OpenAIEmbedder calls OpenAI's embeddings API.
type OpenAIEmbedder struct {
	model  openai.EmbeddingModel
	client *openai.Client
}

const defaultEmbeddingTimeout = 30 * time.Second

// NewOpenAIEmbedder creates a new OpenAI embedder.
func NewOpenAIEmbedder(apiKey string, model openai.EmbeddingModel) (*OpenAIEmbedder, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("api key required")
	}
	if model == "" {
		model = openai.EmbeddingModelTextEmbedding3Small
	}
	cli := openai.NewClient(option.WithAPIKey(apiKey))
	return &OpenAIEmbedder{
		model:  model,
		client: &cli,
	}, nil
}

func (e *OpenAIEmbedder) Embed(text string) (Vector, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("embedder not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultEmbeddingTimeout)
	defer cancel()

	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(text),
		},
		Model: e.model,
	})
	if err != nil {
		return nil, fmt.Errorf("openai embedding failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data returned")
	}
	// Convert []float64 to []float32
	embedding := resp.Data[0].Embedding
	vec := make(Vector, len(embedding))
	for i, v := range embedding {
		vec[i] = float32(v)
	}
	return vec, nil
}

// EmbedBatch generates embeddings for multiple texts in a single API call.
func (e *OpenAIEmbedder) EmbedBatch(texts []string) ([]Vector, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("embedder not initialized")
	}
	if len(texts) == 0 {
		return []Vector{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultEmbeddingTimeout)
	defer cancel()

	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: texts,
		},
		Model: e.model,
	})
	if err != nil {
		return nil, fmt.Errorf("openai batch embedding failed: %w", err)
	}
	if len(resp.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(resp.Data))
	}

	// Convert [][]float64 to []Vector ([]float32)
	vectors := make([]Vector, len(resp.Data))
	for i, data := range resp.Data {
		embedding := data.Embedding
		vec := make(Vector, len(embedding))
		for j, v := range embedding {
			vec[j] = float32(v)
		}
		vectors[i] = vec
	}

	return vectors, nil
}
