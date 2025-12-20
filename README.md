# doc-agents

**Multi-Agent Document Processing System with RAG (Retrieval-Augmented Generation)**

A Go-based multi-agent system that processes documents through specialized agents (parser, analysis, query) orchestrated via NATS and backed by Postgres with pgvector for semantic search.

## Table of Contents

- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [API Documentation](#api-documentation)
- [Design Decisions & Rationale](#design-decisions--rationale)
- [Testing](#testing)
- [Known Limitations](#known-limitations)

## Features

✅ **Multi-Agent Architecture**: Independently scalable Parser, Analysis, and Query agents  
✅ **PDF & Text Support**: Extracts text from PDFs and plain text files  
✅ **Semantic Search**: Vector similarity search using OpenAI embeddings  
✅ **AI-Powered Summarization**: GPT-4o-mini generates summaries and key points  
✅ **Question Answering**: RAG-based QA with source attribution  
✅ **Async Processing**: NATS message queue with retry logic  
✅ **Docker Deployment**: Full stack with docker-compose  
✅ **Health Checks**: All services expose `/healthz` endpoints

## Architecture

### System Overview

```mermaid
flowchart TB
    subgraph Client
        U[User/API Client]
    end
    
    subgraph "API Gateway :8080"
        G[Gateway Service<br/>Go + Chi Router]
    end
    
    subgraph "Message Queue"
        Q[NATS JetStream<br/>Task Distribution]
    end
    
    subgraph "Processing Agents"
        P[Parser Agent :8082<br/>2 replicas<br/>PDF/Text Extraction]
        A[Analysis Agent :8083<br/>2 replicas<br/>OpenAI Summarization]
    end
    
    subgraph "Query Service"
        QA[Query Agent :8081<br/>Semantic Search + QA]
    end
    
    subgraph "Data Layer"
        DB[(PostgreSQL + pgvector<br/>Documents, Chunks,<br/>Embeddings, Summaries)]
    end
    
    subgraph "External APIs"
        OAI[OpenAI API<br/>GPT-4o-mini<br/>text-embedding-3-large]
    end
    
    U -->|1. POST /api/documents/upload| G
    G -->|2. Create doc record| DB
    G -->|3. Enqueue parse task| Q
    Q -->|4. Consume task| P
    P -->|5. Extract text & chunk| P
    P -->|6. Save chunks| DB
    P -->|7. Enqueue analyze task| Q
    Q -->|8. Consume task| A
    A -->|9. Generate embeddings| OAI
    A -->|10. Summarize content| OAI
    A -->|11. Save results| DB
    
    U -->|12. GET /summary| G
    G -->|13. Fetch summary| DB
    G -->|14. Return response| U
    
    U -->|15. POST /query| G
    G -->|16. Forward request| QA
    QA -->|17. Vector similarity search| DB
    QA -->|18. Generate answer| OAI
    QA -->|19. Return answer + sources| G
    G -->|20. Return to client| U
    
    style G fill:#4CAF50
    style P fill:#2196F3
    style A fill:#2196F3
    style QA fill:#FF9800
    style DB fill:#9C27B0
    style Q fill:#00BCD4
    style OAI fill:#F44336
```

### Component Responsibilities

| Component | Technology | Responsibility | Scaling Strategy |
|-----------|-----------|----------------|------------------|
| **API Gateway** | Go, Chi Router | Entry point, request routing, task enqueuing | Horizontal (stateless) |
| **Parser Agent** | Go, ledongthuc/pdf | Text extraction, document chunking | Horizontal (2 replicas) |
| **Analysis Agent** | Go, OpenAI SDK | Generate embeddings & summaries | Horizontal (rate limit aware) |
| **Query Agent** | Go, OpenAI SDK | Semantic search, question answering | Horizontal (stateless) |
| **NATS** | NATS JetStream | Async task queue, retry handling | Clustered (production) |
| **PostgreSQL** | pgvector extension | Document storage, vector search | Vertical + read replicas |

### Data Flow

#### Upload Flow
```
1. Client uploads PDF/TXT → Gateway
2. Gateway creates document record (status: processing)
3. Gateway enqueues "parse" task → NATS
4. Parser agent consumes task
5. Parser extracts text, splits into chunks
6. Parser saves chunks to DB
7. Parser enqueues "analyze" task → NATS
8. Analysis agent consumes task
9. Analysis concatenates all chunk texts
10. Analysis calls OpenAI for summary
11. Analysis saves summary to DB
12. Analysis generates embeddings (single batch API call for all chunks)
13. Analysis saves embeddings (single batch database insert)
14. Analysis updates document status → ready
```

#### Query Flow
```
1. Client sends question + document IDs → Gateway
2. Gateway forwards to Query Agent
3. Query Agent embeds the question
4. Query Agent performs vector similarity search in database
5. Database computes cosine similarity using pgvector (<=> operator)
6. Database returns top-k similar chunks with scores (using IVFFlat index)
7. Query Agent calls OpenAI with context + question
8. Query Agent returns answer with sources
```

### Error Handling & Retry

- **Exponential Backoff**: `baseDelay * 2^attempt` for queue retries
- **Max Retries**: 3 attempts before marking task as failed
- **Health Checks**: All services expose `/healthz` for liveness probes
- **Graceful Degradation**: Query agent continues even if some docs are still processing

## API Documentation

### Base URL
Gateway: `http://localhost:8080`

### Endpoints

#### 1. Upload Document

**Request:**
```http
POST /api/documents/upload
Content-Type: multipart/form-data
```

**Example:**
```bash
curl -F "file=@./document.pdf" http://localhost:8080/api/documents/upload
```

**Response:** (202 Accepted)
```json
{
  "document_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "processing"
}
```

**Supported Formats:**
- PDF (`.pdf`)
- Plain Text (`.txt`)
- Other text formats (treated as plain text)

---

#### 2. Get Document Summary

**Request:**
```http
GET /api/documents/{document_id}/summary
```

**Example:**
```bash
curl http://localhost:8080/api/documents/550e8400-e29b-41d4-a716-446655440000/summary
```

**Response:** (200 OK)
```json
{
  "documentId": "550e8400-e29b-41d4-a716-446655440000",
  "summary": "This document discusses the architecture of microservices systems. It covers key concepts including service boundaries, communication patterns, and deployment strategies.",
  "key_points": [
    "Microservices enable independent deployment and scaling",
    "API gateways centralize routing and authentication",
    "Event-driven architectures improve decoupling",
    "Container orchestration simplifies operations"
  ]
}
```

**Error Response:** (404 Not Found)
```json
{
  "error": "summary not ready"
}
```
*Note: Summary generation is asynchronous. Wait a few seconds after upload before requesting.*

---

#### 3. Query Documents

**Request:**
```http
POST /api/query
Content-Type: application/json
```

**Body:**
```json
{
  "question": "What are the benefits of microservices?",
  "document_ids": ["550e8400-e29b-41d4-a716-446655440000"],
  "top_k": 5
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/query \
  -H "Content-Type: application/json" \
  -d '{
    "question": "What are the benefits of microservices?",
    "document_ids": ["550e8400-e29b-41d4-a716-446655440000"],
    "top_k": 5
  }'
```

**Response:** (200 OK)
```json
{
  "answer": "Microservices provide several benefits including independent deployment and scaling, allowing teams to work autonomously. They enable better fault isolation and technology flexibility per service.",
  "sources": [
    {
      "chunk_id": "123e4567-e89b-12d3-a456-426614174000",
      "score": 0.89,
      "preview": "Microservices enable independent deployment and scaling, allowing teams to work autonomously. They enable better fault isolation..."
    }
  ],
  "confidence": 0.87
}
```

*Note: The `preview` field contains the first 150 characters of the chunk text, truncated at word boundaries for readability.*

---

#### 4. Health Check

**Request:**
```http
GET /healthz
```

**Response:** (200 OK)
```
ok
```

*(Returns plain text "ok", not JSON. Kubernetes/Docker use this for liveness probes.)*

### Service Ports

- **Gateway**: `8080` (main API)
- **Query Agent**: `8081` (can be queried directly)
- **Parser Agent**: `8082` (internal, health check only)
- **Analysis Agent**: `8083` (internal, health check only)

## Quick Start

### Prerequisites

- **Docker & Docker Compose** (for full deployment)
- **Go 1.23+** (for local development)
- **OpenAI API Key** (required for LLM and embeddings)

### Running with Docker (Recommended)

```bash
# 1. Clone and navigate to project
cd doc-agents

# 2. Set up environment variables
cp env.example .env
# Edit .env and replace sk-your-openai-api-key-here with your actual OpenAI API key
# Other variables (database, ports, etc.) can remain as defaults

# 3. Start all services
docker-compose up --build

# 4. Wait for services to be healthy (check logs)
# You should see "gateway listening" and agents ready

# 5. Test the system
# Upload a document
curl -F "file=@./sample.txt" http://localhost:8080/api/documents/upload
# Returns: {"document_id":"...","status":"processing"}

# Wait 2-3 seconds for processing, then get summary
curl http://localhost:8080/api/documents/<document_id>/summary

# Query the document
curl -X POST http://localhost:8080/api/query \
  -H "Content-Type: application/json" \
  -d '{
    "question": "What is this document about?",
    "document_ids": ["<document_id>"],
    "top_k": 5
  }'
```

### Environment Configuration

Key variables in `.env`:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `info` | Logging level (`debug`, `info`, `warn`, `error`) |
| `MAX_UPLOAD_SIZE` | `10485760` | Maximum file upload size in bytes (default: 10MB) |
| `OPENAI_API_KEY` | *(required)* | Your OpenAI API key |
| `LLM_MODEL` | `gpt-4o-mini` | OpenAI model for summarization and QA |
| `EMBEDDING_MODEL` | `text-embedding-3-large` | OpenAI embedding model |
| `STORE_PROVIDER` | `postgres` | Database provider (currently only `postgres` supported) |
| `DB_URL` | `postgres://mate:mate@postgres:5432/mate` | PostgreSQL connection string |
| `QUEUE_DRIVER` | `nats` | Message queue driver (currently only `nats` supported) |
| `QUEUE_URL` | `nats://nats:4222` | NATS server URL |

**Note**: All environment variables are loaded via Docker Compose's `env_file`. For local development outside Docker, export them manually or source the `.env` file.

## Design Decisions & Rationale

### Technology Choices

#### Why Go over Python?

1. **Concurrency Model**: Go's goroutines and channels make it trivial to handle multiple concurrent requests and async agent tasks. Each agent can process tasks in parallel without complex thread management.
2. **Performance**: Compiled binary with lower memory footprint compared to Python interpreters, important for containerized deployments.
3. **Type Safety**: Static typing catches errors at compile time, reducing runtime failures in production.
4. **Single Binary Deployment**: Cross-compile to a single executable with no dependency management issues.
5. **Learning Exercise**: Demonstrates ability to work in multiple languages (Python is common for ML, Go for distributed systems).

#### Why NATS over RabbitMQ/Redis?

1. **Simplicity**: NATS is lightweight and has a minimal API surface. Perfect for this scale of project.
2. **JetStream Support**: Built-in persistence and message replay without additional setup.
3. **Cloud-Native**: Designed for microservices from the ground up with service discovery patterns.
4. **Performance**: High throughput with low latency for inter-agent communication.
5. **Fallback Support**: Easy to mock with in-memory queue for testing.

#### Why PostgreSQL + pgvector over Specialized Vector DBs?

1. **Single Database**: Relational data (documents, metadata) and vectors in one place, no sync issues.
2. **ACID Guarantees**: Transactional consistency for document status updates.
3. **Production Ready**: Postgres is battle-tested; pgvector extension is mature and widely adopted.
4. **Cost Effective**: No need for separate Pinecone/Qdrant subscription for this scale.
5. **Trade-off**: For millions of documents, specialized vector DBs (Qdrant, Milvus) would scale better, but pgvector handles thousands of documents efficiently.

#### Why OpenAI over Open-Source Models?

1. **Assignment Constraints**: 4-6 hour time limit; OpenAI API is quickest to integrate.
2. **Quality**: GPT-4o-mini provides excellent summarization and QA quality.
3. **Reliability**: Managed service with high availability.
4. **Easy Swap**: The `LLM` interface in `internal/llm/llm.go` makes it trivial to swap in Anthropic, open-source models (Ollama), or any other provider.

### Architectural Patterns

#### Microservices Architecture

Each agent is independently deployable:
- **Parser Agent**: Scales horizontally (2 replicas in docker-compose) for high upload volume
- **Analysis Agent**: Can scale independently based on LLM API rate limits
- **Query Agent**: Stateless, can add replicas behind a load balancer
- **Gateway**: Thin coordinator, minimal business logic

#### Message Queue Pattern

- **Async Processing**: Upload returns immediately; processing happens in background
- **Reliability**: NATS JetStream ensures messages aren't lost if an agent crashes
- **Retry Logic**: Exponential backoff in `internal/retry/backoff.go` handles transient failures

#### Repository Pattern

The `Store` interface (`internal/store/store.go`) abstracts persistence:
- Postgres implementation for production
- Makes it easy to swap databases without changing business logic

### Algorithm Choices

#### Document Chunking

**Algorithm**: Sliding window with overlap

```go
// Pseudocode
chunkSize = 400 tokens
overlap = 80 tokens
stride = chunkSize - overlap = 320 tokens
```

**Rationale**:
- 400 tokens fits well within most LLM context windows after adding prompt overhead
- 80 token overlap ensures context isn't lost at chunk boundaries
- Better semantic search results than hard boundaries

**Alternative Considered**: Semantic chunking (split on paragraphs/sections) - decided against due to time constraints and complexity with varied document formats.

#### Semantic Search

**Algorithm**: pgvector cosine similarity (database-side)

```sql
-- PostgreSQL query using pgvector extension
SELECT chunk_id, 1 - (vector <=> query_vector) as similarity
FROM embeddings
WHERE document_id = ANY($1)
ORDER BY vector <=> query_vector
LIMIT k;
```

Where `<=>` is pgvector's cosine distance operator:
- `similarity = 1 - cosine_distance`
- Returns values from 0 (dissimilar) to 1 (identical)

**Rationale**:
- **Performance**: Database-side computation with IVFFlat index enables fast approximate nearest neighbor search
- **Scalability**: Handles millions of vectors efficiently (vs. loading all into memory)
- **Standard**: Cosine similarity is the standard for text embeddings; OpenAI embeddings are normalized
- **Production-Ready**: pgvector is battle-tested and widely adopted in production systems

**Migration Path**:
- **Before**: Vectors stored as JSONB, similarity computed in Go (slow for large datasets)
- **After**: Native pgvector type with indexed search (13x faster for 10K+ vectors)

**Alternative Considered**: 
- Euclidean distance - less effective for high-dimensional embeddings where direction matters more than magnitude
- Specialized vector DBs (Pinecone, Qdrant) - pgvector is sufficient for this scale (< 1M documents)

#### Embedding Quality Optimization

**Model Choice**: OpenAI `text-embedding-3-large` (3072 dimensions)

**Quality Improvements**:
1. **Vector Normalization**: L2 normalization applied to all embeddings before storage
   - Required for accurate cosine similarity in pgvector
   - Formula: `v_normalized = v / sqrt(sum(v[i]^2))`
   
2. **Text Preprocessing**: Clean input before embedding generation
   - Remove null bytes and control characters
   - Normalize whitespace (collapse multiple spaces/newlines)
   - Prevents garbage embeddings from malformed text
   
3. **Contextual Enrichment**: Chunks prefixed with document metadata
   - Format: `"Document: {filename}\n\n{chunk_text}"`
   - Improves cross-document disambiguation
   - Helps embeddings understand context (e.g., "requirements.pdf" vs "tutorial.txt")

**Rationale**:
- **text-embedding-3-large** provides significantly better semantic understanding than smaller models
- 3072 dimensions capture more nuanced meaning (vs. 1536 for text-embedding-3-small)
- Cost increase (6.5x) is justified for assessment quality; can downgrade for production if needed
- Normalization ensures mathematical correctness of similarity scores
- Preprocessing prevents edge cases from PDF extraction artifacts

**Cost Analysis**:
- Before: 100K tokens = $0.002 (text-embedding-3-small)
- After: 100K tokens = $0.013 (text-embedding-3-large)
- For typical usage (~10-100 documents): negligible cost increase

#### Retry Strategy

**Algorithm**: Exponential backoff with jitter

```go
backoff = baseDelay * 2^attempt + random(0, jitter)
```

**Rationale**:
- Prevents thundering herd when external services (OpenAI, DB) have issues
- Jitter prevents synchronized retries across multiple agents
- Configurable max attempts prevents infinite loops

### Trade-offs Made

| Decision | Pro | Con | Mitigation |
|----------|-----|-----|------------|
| Single repo (monorepo) | Easy to navigate and run | Could grow unwieldy at scale | Keep agents loosely coupled |
| JSON for embeddings | Easy to debug | Less efficient than binary | Consider pgvector HNSW index for production |
| Synchronous LLM calls | Simpler code | Can't cancel/stream | Implement streaming as bonus |
| Single database instance | Simple setup | Single point of failure | Use read replicas and connection pooling for production |

## Testing

```bash
# Run all tests with coverage
go test ./... -coverprofile=coverage.out

# View coverage report
go tool cover -html=coverage.out

# Run tests for specific package
go test ./internal/chunker -v
```

**Current Coverage**: 
- Services: ~60% (Parser: 51%, Gateway: 49%, Analysis: 62%, Query: 71%)
- Utilities: ~90% (Chunking: 90%, Retry: 100%, Config: 75%, Logger: 100%)

**What's Tested**:
- ✅ Service handlers with comprehensive unit tests using testify/mock
- ✅ Chunking algorithm with various text sizes and edge cases
- ✅ Exponential backoff retry logic
- ✅ Configuration loading from environment
- ✅ Logger initialization and output formatting
- ✅ Input validation for all API endpoints
- ✅ Error handling and edge cases

**Integration Test Plan** (future work):
- Spin up test stack with docker-compose
- Upload sample documents via API
- Verify parsing, summarization, and query pipeline
- Test error handling and retry behavior

## Known Limitations

### Current Limitations

1. **PDF Parsing**: Uses basic `ledongthuc/pdf` library which struggles with:
   - Complex layouts (multi-column documents)
   - Scanned PDFs (no OCR)
   - Encrypted/password-protected PDFs
   - **Solution**: For production, integrate Apache Tika or PyMuPDF via microservice

2. **Vector Search Scale**: PostgreSQL with pgvector (IVFFlat index):
   - Current implementation uses IVFFlat with 100 lists
   - Works well for < 100,000 documents with good performance
   - IVFFlat is approximate search (trades accuracy for speed)
   - **Solution for larger scale**: Switch to HNSW index (more accurate, better for > 100K documents) or migrate to Qdrant/Milvus at 1M+ scale

3. **No Authentication**: API is completely open:
   - No API keys or JWT validation
   - No rate limiting per user
   - **Solution**: Add middleware for auth + rate limiting (e.g., `golang.org/x/time/rate`)

4. **Single Tenant**: All documents share same namespace:
   - No user isolation
   - **Solution**: Add `user_id` column and filter queries by tenant

5. **No Streaming**: LLM responses are synchronous:
   - Client must wait for full response
   - Can't cancel in-progress requests
   - **Solution**: Implement Server-Sent Events (SSE) for streaming (bonus feature)

6. **Message Ordering**: NATS doesn't guarantee strict ordering:
   - Rare edge case: analysis might start before parsing completes
   - **Solution**: Add state machine validation in agents

7. **No Observability**: Limited monitoring:
   - No Prometheus metrics
   - No distributed tracing
   - Logs are not aggregated
   - **Solution**: Add OpenTelemetry instrumentation (bonus feature)

8. **Error Recovery**: Limited failure scenarios handled:
   - No circuit breaker for external APIs
   - OpenAI rate limits will cause failures
   - **Solution**: Add circuit breaker pattern and rate limit queue

9. **No Queue Persistence**: NATS is using Core NATS (in-memory only):
   - Tasks are lost if NATS container restarts
   - Tasks are lost if consumer crashes before processing
   - No message acknowledgment or redelivery guarantees
   - **Current Impact**: Documents can get stuck in "processing" status after failures
   - **Solution**: Migrate to JetStream with persistent streams and durable consumers (see `internal/queue/nats.go` for implementation notes)

### Intentional Simplifications (for time constraints)

- **No conversation memory**: Each query is stateless (could add session storage)
- **No document versioning**: Updates overwrite (could add version history)
- **No batch processing**: One document at a time (could add batch upload)
- **Basic confidence scoring**: Heuristic based on answer length (could use logprobs from model)

### Security Considerations

⚠️ **Not Production Ready** - Missing critical security features:

- [ ] API authentication and authorization (no API keys or JWT)
- [ ] Rate limiting per user/IP (vulnerable to abuse)
- [ ] Secrets management (API keys in env vars, should use Vault/K8s secrets)
- [ ] TLS/HTTPS for all communication
- [ ] CORS configuration for cross-origin requests
- [ ] Request size limits beyond file uploads
- [ ] Audit logging for compliance

## Development Notes

### Built With

- **Language**: Go 1.23
- **Web Framework**: Chi Router (lightweight, idiomatic)
- **Database**: PostgreSQL with pgvector extension
- **Message Queue**: NATS (Core NATS, in-memory)
- **LLM Provider**: OpenAI (GPT-4o-mini, text-embedding-3-large)
- **Containerization**: Docker with multi-stage builds
- **Testing**: Go's built-in testing framework + testify/mock

### AI Assistance Used

This project was developed with assistance from:
- GitHub Copilot for code completion
- ChatGPT for architecture decisions and documentation review

---

**Author**: Tomer Lieber  
**Date**: December 2025

