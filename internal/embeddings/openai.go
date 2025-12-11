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

func (e *OpenAIEmbedder) Embed(text string) Vector {
	if e == nil || e.client == nil {
		return nil
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
		return nil
	}
	if len(resp.Data) == 0 {
		return nil
	}
	// Convert []float64 to []float32
	embedding := resp.Data[0].Embedding
	vec := make(Vector, len(embedding))
	for i, v := range embedding {
		vec[i] = float32(v)
	}
	return vec
}

