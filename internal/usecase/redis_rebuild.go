package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/flowforge/internal/ports"
)

type RedisRebuildResult struct {
	Scanned int `json:"scanned"`
	Pending int `json:"pending"`
	Delayed int `json:"delayed"`
}

type RedisRebuilder struct {
	storage ports.Storage
	broker  ports.Broker
	now     func() time.Time
}

func NewRedisRebuilder(storage ports.Storage, broker ports.Broker) *RedisRebuilder {
	return &RedisRebuilder{storage: storage, broker: broker, now: time.Now}
}

func (r *RedisRebuilder) Rebuild(ctx context.Context) (RedisRebuildResult, error) {
	const batchSize = 250
	var result RedisRebuildResult
	afterID := uuid.Nil
	for {
		tasks, err := r.storage.ListDispatchableTasks(ctx, afterID, batchSize)
		if err != nil {
			return result, fmt.Errorf("scan dispatchable tasks: %w", err)
		}
		if len(tasks) == 0 {
			return result, nil
		}
		for _, task := range tasks {
			result.Scanned++
			if err := r.broker.Remove(ctx, task.OrgID, task.Queue, task.ID); err != nil {
				return result, fmt.Errorf("clear task %s broker state: %w", task.ID, err)
			}
			now := r.now().UTC()
			if task.VisibleAt.After(now) {
				if err := r.broker.EnqueueDelayed(ctx, task, task.VisibleAt.Sub(now)); err != nil {
					return result, fmt.Errorf("rebuild delayed task %s: %w", task.ID, err)
				}
				result.Delayed++
			} else {
				if err := r.broker.Enqueue(ctx, task); err != nil {
					return result, fmt.Errorf("rebuild pending task %s: %w", task.ID, err)
				}
				result.Pending++
			}
			if err := r.storage.MarkDispatched(ctx, task.OrgID, task.ID, now); err != nil {
				return result, fmt.Errorf("mark task %s dispatched: %w", task.ID, err)
			}
			afterID = task.ID
		}
		if len(tasks) < batchSize {
			return result, nil
		}
	}
}
