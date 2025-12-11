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

func (c *OpenAIClient) Answer(ctx context.Context, question, contextText string) (string, float32, error) {
	if c == nil || c.client == nil {
		return "", 0, fmt.Errorf("nil openai client")
	}
	reqCtx, cancel := context.WithTimeout(ctx, defaultChatTimeout)
	defer cancel()
	messages := buildMessages(
		"You answer questions concisely based only on the provided context.",
		fmt.Sprintf("Context:\n%s\n\nQuestion: %s", contextText, question),
	)
	resp, err := c.client.Chat.Completions.New(reqCtx, openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    messages,
		Temperature: openai.Float(defaultChatTemperature),
	})
	if err != nil {
		return "", 0, err
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return "", 0, fmt.Errorf("openai: no choices returned")
	}
	answer := resp.Choices[0].Message.Content
	conf := deriveConfidence(answer)
	return answer, conf, nil
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

// deriveConfidence returns a simple heuristic confidence based on answer length.
// This is not a model-provided probability; it just scales with content size.
func deriveConfidence(answer string) float32 {
	if answer == "" {
		return 0
	}
	score := 0.5 + 0.5*math.Tanh(float64(len(answer))/200.0)
	return float32(score)
}
