package config

import (
	"log/slog"

	"github.com/caarlos0/env/v10"
)

// Config holds minimal runtime configuration. Extend as needed.
type Config struct {
	// Server
	Port     int    `env:"PORT" envDefault:"8080"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	
	// Upload limits
	MaxUploadSize int64 `env:"MAX_UPLOAD_SIZE" envDefault:"10485760"` // 10MB in bytes

	// Store
	StoreProvider string `env:"STORE_PROVIDER" envDefault:"postgres"` // "postgres" (production database)
	DBURL         string `env:"DB_URL"`

	// Queue
	QueueProvider string `env:"QUEUE_PROVIDER" envDefault:"nats"` // "nats" (required for inter-service communication)
	QueueURL      string `env:"QUEUE_URL"`

	// LLM & Embeddings
	LLMProvider    string `env:"LLM_PROVIDER" envDefault:"openai"` // "openai" (uses OpenAI API) or "stub" (for testing)
	OpenAIKey      string `env:"OPENAI_API_KEY"`
	LLMModel       string `env:"LLM_MODEL" envDefault:"gpt-4o-mini"`
	EmbeddingModel string `env:"EMBEDDING_MODEL" envDefault:"text-embedding-3-small"`
}

// Load reads configuration from environment variables with defaults.
func Load() Config {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		slog.Warn("failed to parse env; using defaults where set", "err", err)
	}
	return cfg
}
