package chunker

import (
	"strings"
	"testing"
)

func TestChunkTextOverlap(t *testing.T) {
	text := "one two three four five six seven eight nine ten"
	chunks := ChunkText(text, Options{MaxTokens: 4, Overlap: 1})
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].Text == chunks[1].Text {
		t.Fatal("expected overlap but not identical chunks")
	}
	if chunks[0].TokenCount != 4 {
		t.Fatalf("expected token count 4, got %d", chunks[0].TokenCount)
	}
}

func TestChunkTextEmptyInput(t *testing.T) {
	chunks := ChunkText("", Options{MaxTokens: 10})
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}

func TestChunkTextNoOverlap(t *testing.T) {
	text := "one two three four five six"
	chunks := ChunkText(text, Options{MaxTokens: 3, Overlap: 0})

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}

	// First chunk should be "one two three"
	if chunks[0].TokenCount != 3 {
		t.Errorf("expected first chunk to have 3 tokens, got %d", chunks[0].TokenCount)
	}

	// Second chunk should be "four five six"
	if chunks[1].TokenCount != 3 {
		t.Errorf("expected second chunk to have 3 tokens, got %d", chunks[1].TokenCount)
	}
}

func TestChunkTextDefaults(t *testing.T) {
	text := "word " + strings.Repeat("test ", 500)
	chunks := ChunkText(text, Options{}) // No options, should use defaults

	if len(chunks) == 0 {
		t.Error("expected chunks with default options")
	}

	// Default MaxTokens should be applied
	for _, chunk := range chunks {
		if chunk.TokenCount > 400 {
			t.Errorf("chunk exceeded default max tokens (400): got %d", chunk.TokenCount)
		}
	}
}
