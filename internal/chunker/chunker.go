package chunker

import (
	"strings"
)

// Options controls how text is chunked.
type Options struct {
	MaxTokens int
	Overlap   int
}

// Chunk represents a slice of the document text.
type Chunk struct {
	Index      int
	Text       string
	TokenCount int
}

// ChunkText performs a simple token-based sliding window with overlap.
// Tokens are approximated by whitespace-delimited words to avoid heavy dependencies.
func ChunkText(text string, opts Options) []Chunk {
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 400
	}
	if opts.Overlap < 0 {
		opts.Overlap = 0
	}

	words := strings.Fields(text)
	var chunks []Chunk
	if len(words) == 0 {
		return chunks
	}

	step := opts.MaxTokens - opts.Overlap
	if step <= 0 {
		step = opts.MaxTokens
	}

	for start := 0; start < len(words); start += step {
		end := start + opts.MaxTokens
		if end > len(words) {
			end = len(words)
		}
		segment := strings.Join(words[start:end], " ")
		chunks = append(chunks, Chunk{
			Index:      len(chunks),
			Text:       segment,
			TokenCount: end - start,
		})
		if end == len(words) {
			break
		}
	}
	return chunks
}

