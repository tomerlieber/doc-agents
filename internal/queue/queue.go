package queue

import (
	"context"
	"time"

	"github.com/google/uuid"

	"doc-agents/internal/retry"
)

// TaskType enumerates supported task categories.
type TaskType string

const (
	TaskTypeParse   TaskType = "parse"
	TaskTypeAnalyze TaskType = "analyze"
)

// Task represents a unit of work shared across agents.
type Task struct {
	ID          uuid.UUID
	Type        TaskType
	Payload     []byte
	Attempts    int
	MaxAttempts int
	NotBefore   time.Time
}

type Handler func(context.Context, Task) error

// Queue exposes a minimal contract to enqueue and consume tasks.
type Queue interface {
	Enqueue(ctx context.Context, task Task) error
	Worker(ctx context.Context, taskType TaskType, handler Handler) error
}

// EnqueueWithRetry attempts to enqueue with retries and exponential backoff.
func EnqueueWithRetry(ctx context.Context, q Queue, task Task, attempts int, base time.Duration) error {
	if attempts <= 0 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if err := q.Enqueue(ctx, task); err == nil {
			return nil
		} else if attempt == attempts-1 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retry.ExponentialBackoff(attempt, base)):
		}
	}
	return nil
}
