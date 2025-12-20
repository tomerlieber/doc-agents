package embeddings

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
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

	// Preprocess text before embedding
	text = preprocessText(text)
	if text == "" {
		return nil, fmt.Errorf("text is empty after preprocessing")
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
	// Normalize vector for cosine similarity
	normalize(vec)
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

	// Preprocess all texts
	processedTexts := make([]string, 0, len(texts))
	for _, text := range texts {
		processed := preprocessText(text)
		if processed != "" {
			processedTexts = append(processedTexts, processed)
		}
	}

	if len(processedTexts) == 0 {
		return []Vector{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultEmbeddingTimeout)
	defer cancel()

	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: processedTexts,
		},
		Model: e.model,
	})
	if err != nil {
		return nil, fmt.Errorf("openai batch embedding failed: %w", err)
	}
	if len(resp.Data) != len(processedTexts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(processedTexts), len(resp.Data))
	}

	// Convert [][]float64 to []Vector ([]float32)
	vectors := make([]Vector, len(resp.Data))
	for i, data := range resp.Data {
		embedding := data.Embedding
		vec := make(Vector, len(embedding))
		for j, v := range embedding {
			vec[j] = float32(v)
		}
		// Normalize vector for cosine similarity
		normalize(vec)
		vectors[i] = vec
	}

	return vectors, nil
}

// preprocessText cleans and normalizes text before embedding.
// Removes excessive whitespace, control characters, and validates non-empty content.
func preprocessText(text string) string {
	// Remove null bytes and control characters (except newlines and tabs)
	controlCharsRegex := regexp.MustCompile(`[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]`)
	text = controlCharsRegex.ReplaceAllString(text, "")

	// Normalize whitespace: collapse multiple spaces, newlines, tabs into single space
	text = strings.TrimSpace(text)
	whitespaceRegex := regexp.MustCompile(`\s+`)
	text = whitespaceRegex.ReplaceAllString(text, " ")

	return text
}

// normalize applies L2 normalization to a vector in-place.
// Required for accurate cosine similarity in pgvector.
func normalize(vec Vector) {
	var sumSquares float64
	for _, v := range vec {
		sumSquares += float64(v) * float64(v)
	}
	if sumSquares == 0 {
		return // Avoid division by zero
	}
	magnitude := float32(math.Sqrt(sumSquares))
	for i := range vec {
		vec[i] /= magnitude
	}
}
