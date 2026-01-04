package app

import (
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/openai/openai-go/v3"

	"doc-agents/internal/cache"
	"doc-agents/internal/config"
	"doc-agents/internal/embeddings"
	"doc-agents/internal/llm"
	"doc-agents/internal/logger"
	"doc-agents/internal/queue"
	"doc-agents/internal/store"
)

// BaseDeps contains dependencies common to all services
type BaseDeps struct {
	Config config.Config
	Log    *slog.Logger
	Store  store.Store
}

// GetConfig implements httputil.Deps
func (d BaseDeps) GetConfig() config.Config {
	return d.Config
}

// GetLog implements httputil.Deps
func (d BaseDeps) GetLog() *slog.Logger {
	return d.Log
}

// ParserDeps contains dependencies for the parser service
type ParserDeps struct {
	BaseDeps
	Queue queue.Queue
}

// AnalysisDeps contains dependencies for the analysis service
type AnalysisDeps struct {
	BaseDeps
	Queue    queue.Queue
	LLM      llm.Client
	Embedder embeddings.Embedder
}

// QueryDeps contains dependencies for the query service
type QueryDeps struct {
	BaseDeps
	LLM      llm.Client
	Embedder embeddings.Embedder
	Cache    cache.Cache
}

// GatewayDeps contains dependencies for the gateway service
type GatewayDeps struct {
	BaseDeps
	Queue queue.Queue
}

// BuildParser initializes dependencies for the parser service
func BuildParser() (ParserDeps, error) {
	base, err := buildBase()
	if err != nil {
		return ParserDeps{}, err
	}

	q, err := buildQueue(base.Config, base.Log)
	if err != nil {
		return ParserDeps{}, fmt.Errorf("failed to initialize queue: %w", err)
	}

	return ParserDeps{
		BaseDeps: base,
		Queue:    q,
	}, nil
}

// BuildAnalysis initializes dependencies for the analysis service
func BuildAnalysis() (AnalysisDeps, error) {
	base, err := buildBase()
	if err != nil {
		return AnalysisDeps{}, err
	}

	q, err := buildQueue(base.Config, base.Log)
	if err != nil {
		return AnalysisDeps{}, fmt.Errorf("failed to initialize queue: %w", err)
	}

	llmClient, err := buildLLM(base.Config, base.Log)
	if err != nil {
		return AnalysisDeps{}, fmt.Errorf("failed to initialize LLM: %w", err)
	}

	embedder, err := buildEmbedder(base.Config, base.Log)
	if err != nil {
		return AnalysisDeps{}, fmt.Errorf("failed to initialize embedder: %w", err)
	}

	return AnalysisDeps{
		BaseDeps: base,
		Queue:    q,
		LLM:      llmClient,
		Embedder: embedder,
	}, nil
}

// BuildQuery initializes dependencies for the query service
func BuildQuery() (QueryDeps, error) {
	base, err := buildBase()
	if err != nil {
		return QueryDeps{}, err
	}

	llmClient, err := buildLLM(base.Config, base.Log)
	if err != nil {
		return QueryDeps{}, fmt.Errorf("failed to initialize LLM: %w", err)
	}

	embedder, err := buildEmbedder(base.Config, base.Log)
	if err != nil {
		return QueryDeps{}, fmt.Errorf("failed to initialize embedder: %w", err)
	}

	cacheClient, err := buildCache(base.Config, base.Log)
	if err != nil {
		// Cache is optional - degrade gracefully if unavailable
		base.Log.Warn("failed to initialize cache, continuing without caching", "err", err)
		cacheClient = cache.NewNoOpCache()
	}

	return QueryDeps{
		BaseDeps: base,
		LLM:      llmClient,
		Embedder: embedder,
		Cache:    cacheClient,
	}, nil
}

// BuildGateway initializes dependencies for the gateway service
func BuildGateway() (GatewayDeps, error) {
	base, err := buildBase()
	if err != nil {
		return GatewayDeps{}, err
	}

	q, err := buildQueue(base.Config, base.Log)
	if err != nil {
		return GatewayDeps{}, fmt.Errorf("failed to initialize queue: %w", err)
	}

	return GatewayDeps{
		BaseDeps: base,
		Queue:    q,
	}, nil
}

// buildBase creates the base dependencies common to all services
func buildBase() (BaseDeps, error) {
	cfg := config.Load()
	log := logger.New(cfg.LogLevel)

	st, err := buildStore(cfg, log)
	if err != nil {
		return BaseDeps{}, fmt.Errorf("failed to initialize store: %w", err)
	}

	return BaseDeps{
		Config: cfg,
		Log:    log,
		Store:  st,
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

func buildCache(cfg config.Config, log *slog.Logger) (cache.Cache, error) {
	switch cfg.CacheProvider {
	case "redis":
		cacheClient, err := cache.NewRedisCache(cfg.RedisAddr, cfg.RedisPassword)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Redis cache: %w", err)
		}
		log.Info("using Redis cache", "addr", cfg.RedisAddr, "ttl_seconds", cfg.CacheTTL)
		return cacheClient, nil
	default:
		return nil, fmt.Errorf("invalid CACHE_PROVIDER: %s (valid option: redis)", cfg.CacheProvider)
	}
}
