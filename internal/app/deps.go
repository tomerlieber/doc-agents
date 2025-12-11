package app

import (
	"fmt"
	"log/slog"

	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
	"github.com/openai/openai-go/v3"

	"doc-agents/internal/config"
	"doc-agents/internal/embeddings"
	"doc-agents/internal/llm"
	"doc-agents/internal/logger"
	"doc-agents/internal/queue"
	"doc-agents/internal/store"
)

// Deps bundles common runtime dependencies for services.
type Deps struct {
	Config   config.Config
	Log      *slog.Logger
	Store    store.Store
	Queue    queue.Queue
	Embedder embeddings.Embedder
	LLM      llm.Client
}

// Build loads env, config, and shared components.
func Build() (Deps, error) {
	if err := godotenv.Load(); err != nil {
		return Deps{}, fmt.Errorf("failed to load environment variables: %w", err)
	}
	cfg := config.Load()
	log := logger.New(cfg.LogLevel)

	st, err := buildStore(cfg, log)
	if err != nil {
		return Deps{}, fmt.Errorf("failed to initialize store: %w", err)
	}
	q, err := buildQueue(cfg, log)
	if err != nil {
		return Deps{}, fmt.Errorf("failed to initialize queue: %w", err)
	}
	llmClient, err := buildLLM(cfg, log)
	if err != nil {
		return Deps{}, fmt.Errorf("failed to initialize LLM: %w", err)
	}
	embedder, err := buildEmbedder(cfg, log)
	if err != nil {
		return Deps{}, fmt.Errorf("failed to initialize embedder: %w", err)
	}
	return Deps{
		Config:   cfg,
		Log:      log,
		Store:    st,
		Queue:    q,
		Embedder: embedder,
		LLM:      llmClient,
	}, nil
}

func buildStore(cfg config.Config, log *slog.Logger) (store.Store, error) {
	switch cfg.StoreProvider {
	case "postgres":
		if cfg.DBURL == "" {
			return nil, fmt.Errorf("DB_URL is required when STORE_PROVIDER=postgres")
		}
		db, err := store.NewPostgres(cfg.DBURL)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Postgres: %w", err)
		}
		log.Info("using Postgres store")
		return db, nil
	default:
		return nil, fmt.Errorf("invalid STORE_PROVIDER: %s (valid option: postgres)", cfg.StoreProvider)
	}
}

func buildQueue(cfg config.Config, log *slog.Logger) (queue.Queue, error) {
	switch cfg.QueueProvider {
	case "nats":
		if cfg.QueueURL == "" {
			return nil, fmt.Errorf("QUEUE_URL is required when QUEUE_PROVIDER=nats")
		}
		nc, err := nats.Connect(cfg.QueueURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to NATS: %w", err)
		}
		log.Info("using NATS queue")
		return queue.NewNATS(log, nc), nil
	default:
		return nil, fmt.Errorf("invalid QUEUE_PROVIDER: %s (valid option: nats)", cfg.QueueProvider)
	}
}

func buildLLM(cfg config.Config, log *slog.Logger) (llm.Client, error) {
	switch cfg.LLMProvider {
	case "openai":
		if cfg.OpenAIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required when LLM_PROVIDER=openai")
		}
		client, err := llm.NewOpenAIClient(cfg.OpenAIKey, openai.ChatModel(cfg.LLMModel))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OpenAI client: %w", err)
		}
		log.Info("using OpenAI LLM client", "model", cfg.LLMModel)
		return client, nil
	default:
		return nil, fmt.Errorf("invalid LLM_PROVIDER: %s (valid option: openai)", cfg.LLMProvider)
	}
}

func buildEmbedder(cfg config.Config, log *slog.Logger) (embeddings.Embedder, error) {
	switch cfg.LLMProvider {
	case "openai":
		if cfg.OpenAIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required when LLM_PROVIDER=openai")
		}
		embedder, err := embeddings.NewOpenAIEmbedder(cfg.OpenAIKey, openai.EmbeddingModel(cfg.EmbeddingModel))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OpenAI embedder: %w", err)
		}
		log.Info("using OpenAI embedder", "model", cfg.EmbeddingModel)
		return embedder, nil
	default:
		return nil, fmt.Errorf("invalid LLM_PROVIDER: %s (valid option: openai)", cfg.LLMProvider)
	}
}
