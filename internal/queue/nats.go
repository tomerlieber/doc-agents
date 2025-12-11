package queue

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"doc-agents/internal/retry"
)

// NewNATS constructs a thin NATS-based queue.
func NewNATS(log *slog.Logger, nc *nats.Conn) Queue {
	return &natsQueue{log: log, nc: nc}
}

type natsQueue struct {
	log *slog.Logger
	nc  *nats.Conn
}

func (q *natsQueue) Enqueue(_ context.Context, task Task) error {
	if task.ID == uuid.Nil {
		task.ID = uuid.New()
	}
	if task.Type == "" {
		return errors.New("task type required")
	}
	body, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return q.nc.Publish("tasks."+string(task.Type), body)
}

func (q *natsQueue) Worker(ctx context.Context, taskType TaskType, handler Handler) error {
	subject := "tasks." + string(taskType)
	group := "workers-" + string(taskType)
	sub, err := q.nc.QueueSubscribe(subject, group, func(msg *nats.Msg) {
		q.handleMessage(ctx, msg, handler)
	})
	if err != nil {
		return err
	}
	<-ctx.Done()
	return sub.Unsubscribe()
}

func (q *natsQueue) handleMessage(ctx context.Context, msg *nats.Msg, handler Handler) {
	var task Task
	if err := json.Unmarshal(msg.Data, &task); err != nil {
		q.log.Error("failed to decode task", "err", err)
		return
	}

	if task.NotBefore.After(time.Now()) {
		time.Sleep(time.Until(task.NotBefore))
	}

	if err := handler(ctx, task); err != nil {
		q.retryTask(ctx, task, err)
	}
}

func (q *natsQueue) retryTask(ctx context.Context, task Task, handlerErr error) {
	task.Attempts++
	if task.MaxAttempts == 0 {
		task.MaxAttempts = 5
	}

	if task.Attempts < task.MaxAttempts {
		task.NotBefore = time.Now().Add(retry.ExponentialBackoff(task.Attempts, time.Second))
		if err := q.Enqueue(ctx, task); err != nil {
			q.log.Error("failed to re-enqueue task after failure", "id", task.ID, "type", task.Type, "original_err", handlerErr, "enqueue_err", err)
		}
	} else {
		q.log.Error("task permanently failed", "id", task.ID, "type", task.Type, "original_err", handlerErr)
	}
}
