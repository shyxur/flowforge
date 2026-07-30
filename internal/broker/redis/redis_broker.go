package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shyxur/flowforge/internal/domain"
	"github.com/shyxur/flowforge/internal/ports"
)

// Key layout:
//   queueflow:v1:org:{org_id}:queue:{queue}:pending    -> LIST
//   queueflow:v1:org:{org_id}:queue:{queue}:processing -> LIST
//   queueflow:v1:org:{org_id}:queue:{queue}:delayed    -> ZSET
//   queueflow:v1:org:{org_id}:queue:{queue}:dlq        -> LIST

type RedisBroker struct {
	client *redis.Client
}

var _ ports.Broker = (*RedisBroker)(nil)

func NewRedisBroker(addr, password string, db int) *RedisBroker {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	return &RedisBroker{client: client}
}

func queueKey(orgID uuid.UUID, queue, suffix string) string {
	return fmt.Sprintf("queueflow:v1:org:%s:queue:%s:%s", orgID, queue, suffix)
}

func pendingKey(orgID uuid.UUID, queue string) string    { return queueKey(orgID, queue, "pending") }
func processingKey(orgID uuid.UUID, queue string) string { return queueKey(orgID, queue, "processing") }
func delayedKey(orgID uuid.UUID, queue string) string    { return queueKey(orgID, queue, "delayed") }
func dlqKey(orgID uuid.UUID, queue string) string        { return queueKey(orgID, queue, "dlq") }

type brokerMessage struct {
	OrgID  uuid.UUID `json:"org_id"`
	TaskID uuid.UUID `json:"task_id"`
}

func message(orgID, taskID uuid.UUID) string {
	data, _ := json.Marshal(brokerMessage{OrgID: orgID, TaskID: taskID})
	return string(data)
}

func parseMessage(raw string, expectedOrgID uuid.UUID) (uuid.UUID, error) {
	var msg brokerMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return uuid.Nil, err
	}
	if msg.OrgID != expectedOrgID || msg.TaskID == uuid.Nil {
		return uuid.Nil, errors.New("broker message tenant mismatch")
	}
	return msg.TaskID, nil
}

func (b *RedisBroker) Enqueue(ctx context.Context, task *domain.Task) error {
	return b.client.LPush(ctx, pendingKey(task.OrgID, task.Queue), message(task.OrgID, task.ID)).Err()
}

func (b *RedisBroker) Dequeue(ctx context.Context, orgID uuid.UUID, queue string, timeout time.Duration) (uuid.UUID, error) {
	// BRPOPLPUSH: atomically move ID from pending -> processing staging list.
	// The processing list acts as a safety net; ReclaimExpired (storage-side,
	// source of truth) handles actual visibility-timeout logic, but this
	// staging list lets us detect broker-level crashes too if needed later.
	res, err := b.client.BRPopLPush(ctx, pendingKey(orgID, queue), processingKey(orgID, queue), timeout).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, domain.ErrQueueEmpty
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("redis dequeue: %w", err)
	}
	id, err := parseMessage(res, orgID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("redis dequeue: invalid task id %q: %w", res, err)
	}
	return id, nil
}

func (b *RedisBroker) Ack(ctx context.Context, orgID uuid.UUID, queue string, taskID uuid.UUID) error {
	return b.client.LRem(ctx, processingKey(orgID, queue), 1, message(orgID, taskID)).Err()
}

func (b *RedisBroker) Nack(ctx context.Context, orgID uuid.UUID, queue string, taskID uuid.UUID, delay time.Duration) error {
	pipe := b.client.TxPipeline()
	member := message(orgID, taskID)
	pipe.LRem(ctx, processingKey(orgID, queue), 1, member)
	if delay <= 0 {
		pipe.LPush(ctx, pendingKey(orgID, queue), member)
	} else {
		score := float64(time.Now().Add(delay).Unix())
		pipe.ZAdd(ctx, delayedKey(orgID, queue), redis.Z{Score: score, Member: member})
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (b *RedisBroker) EnqueueDelayed(ctx context.Context, task *domain.Task, delay time.Duration) error {
	score := float64(time.Now().Add(delay).Unix())
	return b.client.ZAdd(ctx, delayedKey(task.OrgID, task.Queue), redis.Z{
		Score:  score,
		Member: message(task.OrgID, task.ID),
	}).Err()
}

func (b *RedisBroker) MoveToDeadLetter(ctx context.Context, orgID uuid.UUID, queue string, taskID uuid.UUID) error {
	pipe := b.client.TxPipeline()
	member := message(orgID, taskID)
	pipe.LRem(ctx, processingKey(orgID, queue), 1, member)
	pipe.ZRem(ctx, delayedKey(orgID, queue), member)
	pipe.LPush(ctx, dlqKey(orgID, queue), member)
	_, err := pipe.Exec(ctx)
	return err
}

// PromoteDueDelayed moves ready delayed tasks back to pending. Uses ZRANGEBYSCORE
// + atomic removal per member to avoid double-promotion races across replicas
// running this loop concurrently.
func (b *RedisBroker) PromoteDueDelayed(ctx context.Context, orgID uuid.UUID, queue string) (int, error) {
	now := float64(time.Now().Unix())
	ids, err := b.client.ZRangeByScore(ctx, delayedKey(orgID, queue), &redis.ZRangeBy{
		Min: "-inf", Max: fmt.Sprintf("%f", now), Count: 100,
	}).Result()
	if err != nil {
		return 0, fmt.Errorf("redis promote delayed: %w", err)
	}
	promoted := 0
	for _, id := range ids {
		// ZREM returns 1 only if this instance won the race to remove it.
		removed, err := b.client.ZRem(ctx, delayedKey(orgID, queue), id).Result()
		if err != nil {
			return promoted, err
		}
		if removed == 0 {
			continue // another process already promoted it
		}
		if err := b.client.LPush(ctx, pendingKey(orgID, queue), id).Err(); err != nil {
			return promoted, err
		}
		promoted++
	}
	return promoted, nil
}

func (b *RedisBroker) QueueDepth(ctx context.Context, orgID uuid.UUID, queue string) (int64, error) {
	return b.client.LLen(ctx, pendingKey(orgID, queue)).Result()
}

func (b *RedisBroker) Remove(ctx context.Context, orgID uuid.UUID, queue string, taskID uuid.UUID) error {
	member := message(orgID, taskID)
	pipe := b.client.TxPipeline()
	pipe.LRem(ctx, pendingKey(orgID, queue), 0, member)
	pipe.LRem(ctx, processingKey(orgID, queue), 0, member)
	pipe.ZRem(ctx, delayedKey(orgID, queue), member)
	pipe.LRem(ctx, dlqKey(orgID, queue), 0, member)
	_, err := pipe.Exec(ctx)
	return err
}

func (b *RedisBroker) Ping(ctx context.Context) error {
	return b.client.Ping(ctx).Err()
}

func (b *RedisBroker) Close() error {
	return b.client.Close()
}

func (b *RedisBroker) Client() *redis.Client {
	return b.client
}
