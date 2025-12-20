package llm

import "context"

// Client is a minimal LLM interface to allow pluggable providers.
type Client interface {
	Summarize(ctx context.Context, text string) (string, []string, error)
	Answer(ctx context.Context, question, context string, contextQuality float32) (string, float32, error)
}
