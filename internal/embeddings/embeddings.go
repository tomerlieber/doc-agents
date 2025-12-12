package embeddings

// Vector is a simple float32 slice wrapper.
type Vector []float32

// Embedder defines the embedding interface.
type Embedder interface {
	Embed(text string) Vector
}
