package llm

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// OpenAIClient calls the OpenAI Chat Completions API.
type OpenAIClient struct {
	model  openai.ChatModel
	client *openai.Client
}

const (
	defaultChatTimeout     = 30 * time.Second
	defaultChatTemperature = 0.2
)

// NewOpenAIClient builds a client with defaults against api.openai.com.
func NewOpenAIClient(apiKey string, model openai.ChatModel) (*OpenAIClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("api key required")
	}
	if model == "" {
		model = openai.ChatModelGPT4oMini
	}
	cli := openai.NewClient(option.WithAPIKey(apiKey))
	return &OpenAIClient{
		model:  model,
		client: &cli,
	}, nil
}

func (c *OpenAIClient) Summarize(ctx context.Context, text string) (string, []string, error) {
	if c == nil || c.client == nil {
		return "", nil, fmt.Errorf("nil openai client")
	}
	reqCtx, cancel := context.WithTimeout(ctx, defaultChatTimeout)
	defer cancel()
	messages := buildMessages(
		"You are a concise assistant. First provide a brief summary paragraph, then list the key points as bullet points (using - or *).",
		text,
	)
	resp, err := c.client.Chat.Completions.New(reqCtx, openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    messages,
		Temperature: openai.Float(defaultChatTemperature),
	})
	if err != nil {
		return "", nil, err
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return "", nil, fmt.Errorf("openai: no choices returned")
	}
	return extractSummary(resp.Choices[0].Message.Content)
}

func (c *OpenAIClient) Answer(ctx context.Context, question, contextText string, contextQuality float32) (string, float32, error) {
	if c == nil || c.client == nil {
		return "", 0, fmt.Errorf("nil openai client")
	}
	reqCtx, cancel := context.WithTimeout(ctx, defaultChatTimeout)
	defer cancel()

	systemPrompt := `You are a precise document Q&A assistant. Follow these rules strictly:

1. Answer ONLY using information from the provided context
2. If the answer is not in the context, respond with "I don't have enough information to answer this question"
3. Cite specific parts of the context when answering (e.g., "According to the documentation...")
4. Be concise but complete - include all relevant details from the context
5. If the context contains conflicting information, mention both perspectives
6. Never make assumptions or add information not present in the context`

	messages := buildMessages(
		systemPrompt,
		fmt.Sprintf("Context:\n%s\n\nQuestion: %s", contextText, question),
	)
	resp, err := c.client.Chat.Completions.New(reqCtx, openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    messages,
		Temperature: openai.Float(defaultChatTemperature),
		Logprobs:    openai.Bool(true), // Enable token probabilities
		TopLogprobs: openai.Int(1),     // Get top token probability
	})
	if err != nil {
		return "", 0, err
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return "", 0, fmt.Errorf("openai: no choices returned")
	}

	answer := resp.Choices[0].Message.Content

	// Combine retrieval quality with LLM generation confidence
	llmConfidence := calculateLLMConfidence(&resp.Choices[0].Logprobs)
	combinedConfidence := contextQuality * llmConfidence

	return answer, combinedConfidence, nil
}

func buildMessages(system, user string) []openai.ChatCompletionMessageParamUnion {
	return []openai.ChatCompletionMessageParamUnion{
		{
			OfSystem: &openai.ChatCompletionSystemMessageParam{
				Content: openai.ChatCompletionSystemMessageParamContentUnion{
					OfString: openai.String(system),
				},
			},
		},
		{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(user),
				},
			},
		},
	}
}

// extractSummary splits the model response into summary and bullet points heuristically.
func extractSummary(content string) (string, []string, error) {
	lines := strings.Split(content, "\n")
	var points []string
	var summaryLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") {
			points = append(points, strings.TrimLeft(trimmed, "-* "))
		} else {
			summaryLines = append(summaryLines, trimmed)
		}
	}
	summary := strings.Join(summaryLines, " ")
	return summary, points, nil
}

// calculateLLMConfidence computes confidence from token log probabilities.
// Returns average probability across all tokens (converting logprob -> probability).
// Higher values indicate the model was more certain about its token choices.
func calculateLLMConfidence(logprobs *openai.ChatCompletionChoiceLogprobs) float32 {
	if logprobs == nil || len(logprobs.Content) == 0 {
		// If logprobs unavailable, default to high confidence (don't penalize)
		return 1.0
	}

	var sumProb float64
	for _, tokenLogprob := range logprobs.Content {
		// Convert log probability to probability: p = e^(logprob)
		prob := math.Exp(tokenLogprob.Logprob)
		sumProb += prob
	}

	avgProb := sumProb / float64(len(logprobs.Content))
	return float32(avgProb)
}
