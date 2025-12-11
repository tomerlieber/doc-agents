package embeddings

// Vector is a simple float32 slice wrapper.
type Vector []float32

// Embedder defines the embedding interface.
type Embedder interface {
	Embed(text string) Vector
}

// CosineSimilarity computes similarity between two vectors.
func CosineSimilarity(a, b Vector) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float32
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (sqrt(na) * sqrt(nb))
}

// small sqrt to avoid bringing in math for float32.
func sqrt(v float32) float32 {
	z := v
	for i := 0; i < 5; i++ {
		z -= (z*z - v) / (2 * z)
	}
	return z
}
