package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/ports"
)

// Key layout:
//   queueflow:v1:org:{org_id}:queue:{queue}:pending     -> priority ZSET
//   queueflow:v1:org:{org_id}:queue:{queue}:processing  -> in-flight ZSET
//   queueflow:v1:org:{org_id}:queue:{queue}:delayed     -> ready-time ZSET
//   queueflow:v1:org:{org_id}:queue:{queue}:priorities  -> message priority HASH
//   queueflow:v1:org:{org_id}:queue:{queue}:sequence    -> FIFO sequence STRING
//   queueflow:v1:org:{org_id}:queue:{queue}:dlq         -> dead-letter LIST

const priorityBucketSize int64 = 1_000_000_000_000

type RedisBroker struct {
	client *redis.Client
}

var _ ports.Broker = (*RedisBroker)(nil)

func NewRedisBroker(addr, password string, db int) *RedisBroker {
	client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
	return &RedisBroker{client: client}
}

func queueKey(orgID uuid.UUID, queue, suffix string) string {
	return fmt.Sprintf("queueflow:v1:org:%s:queue:%s:%s", orgID, queue, suffix)
}

func pendingKey(orgID uuid.UUID, queue string) string    { return queueKey(orgID, queue, "pending") }
func processingKey(orgID uuid.UUID, queue string) string { return queueKey(orgID, queue, "processing") }
func delayedKey(orgID uuid.UUID, queue string) string    { return queueKey(orgID, queue, "delayed") }
func priorityKey(orgID uuid.UUID, queue string) string   { return queueKey(orgID, queue, "priorities") }
func sequenceKey(orgID uuid.UUID, queue string) string   { return queueKey(orgID, queue, "sequence") }
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

var enqueueScript = redis.NewScript(`
local priority = tonumber(ARGV[2])
if priority < 0 then priority = 0 end
if priority > 9 then priority = 9 end
redis.call("HSET", KEYS[2], ARGV[1], priority)
local sequence = redis.call("INCR", KEYS[3])
local score = ((9 - priority) * tonumber(ARGV[3])) + sequence
redis.call("ZADD", KEYS[1], score, ARGV[1])
return score
`)

var dequeueScript = redis.NewScript(`
local item = redis.call("ZRANGE", KEYS[1], 0, 0, "WITHSCORES")
if #item == 0 then return nil end
if redis.call("ZREM", KEYS[1], item[1]) == 0 then return nil end
redis.call("ZADD", KEYS[2], item[2], item[1])
return item[1]
`)

var immediateNackScript = redis.NewScript(`
redis.call("ZREM", KEYS[1], ARGV[1])
local priority = tonumber(redis.call("HGET", KEYS[3], ARGV[1])) or 0
local sequence = redis.call("INCR", KEYS[4])
local score = ((9 - priority) * tonumber(ARGV[2])) + sequence
redis.call("ZADD", KEYS[2], score, ARGV[1])
return score
`)

var promoteDelayedScript = redis.NewScript(`
local members = redis.call("ZRANGEBYSCORE", KEYS[1], "-inf", ARGV[1], "LIMIT", 0, 100)
local promoted = 0
for _, member in ipairs(members) do
  if redis.call("ZREM", KEYS[1], member) == 1 then
    local priority = tonumber(redis.call("HGET", KEYS[3], member)) or 0
    local sequence = redis.call("INCR", KEYS[4])
    local score = ((9 - priority) * tonumber(ARGV[2])) + sequence
    redis.call("ZADD", KEYS[2], score, member)
    promoted = promoted + 1
  end
end
return promoted
`)

func (b *RedisBroker) Enqueue(ctx context.Context, task *domain.Task) error {
	member := message(task.OrgID, task.ID)
	return enqueueScript.Run(ctx, b.client, []string{
		pendingKey(task.OrgID, task.Queue),
		priorityKey(task.OrgID, task.Queue),
		sequenceKey(task.OrgID, task.Queue),
	}, member, task.Priority, priorityBucketSize).Err()
}

func (b *RedisBroker) Dequeue(ctx context.Context, orgID uuid.UUID, queue string, timeout time.Duration) (uuid.UUID, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var timeoutC <-chan time.Time
	var timer *time.Timer
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		defer timer.Stop()
		timeoutC = timer.C
	}
	for {
		raw, err := dequeueScript.Run(ctx, b.client, []string{
			pendingKey(orgID, queue), processingKey(orgID, queue),
		}).Text()
		if err == nil {
			id, parseErr := parseMessage(raw, orgID)
			if parseErr != nil {
				return uuid.Nil, fmt.Errorf("redis dequeue: invalid task message %q: %w", raw, parseErr)
			}
			return id, nil
		}
		if !errors.Is(err, redis.Nil) {
			return uuid.Nil, fmt.Errorf("redis dequeue: %w", err)
		}
		select {
		case <-ctx.Done():
			return uuid.Nil, ctx.Err()
		case <-timeoutC:
			return uuid.Nil, domain.ErrQueueEmpty
		case <-ticker.C:
		}
	}
}

func (b *RedisBroker) Ack(ctx context.Context, orgID uuid.UUID, queue string, taskID uuid.UUID) error {
	member := message(orgID, taskID)
	pipe := b.client.TxPipeline()
	pipe.ZRem(ctx, processingKey(orgID, queue), member)
	pipe.HDel(ctx, priorityKey(orgID, queue), member)
	_, err := pipe.Exec(ctx)
	return err
}

func (b *RedisBroker) Nack(ctx context.Context, orgID uuid.UUID, queue string, taskID uuid.UUID, delay time.Duration) error {
	member := message(orgID, taskID)
	if delay <= 0 {
		return immediateNackScript.Run(ctx, b.client, []string{
			processingKey(orgID, queue), pendingKey(orgID, queue),
			priorityKey(orgID, queue), sequenceKey(orgID, queue),
		}, member, priorityBucketSize).Err()
	}
	pipe := b.client.TxPipeline()
	pipe.ZRem(ctx, processingKey(orgID, queue), member)
	pipe.ZAdd(ctx, delayedKey(orgID, queue), redis.Z{
		Score: float64(time.Now().Add(delay).UnixMilli()), Member: member,
	})
	_, err := pipe.Exec(ctx)
	return err
}

func (b *RedisBroker) EnqueueDelayed(ctx context.Context, task *domain.Task, delay time.Duration) error {
	member := message(task.OrgID, task.ID)
	pipe := b.client.TxPipeline()
	pipe.HSet(ctx, priorityKey(task.OrgID, task.Queue), member, normalizePriority(task.Priority))
	pipe.ZAdd(ctx, delayedKey(task.OrgID, task.Queue), redis.Z{
		Score: float64(time.Now().Add(delay).UnixMilli()), Member: member,
	})
	_, err := pipe.Exec(ctx)
	return err
}

func (b *RedisBroker) MoveToDeadLetter(ctx context.Context, orgID uuid.UUID, queue string, taskID uuid.UUID) error {
	member := message(orgID, taskID)
	pipe := b.client.TxPipeline()
	pipe.ZRem(ctx, processingKey(orgID, queue), member)
	pipe.ZRem(ctx, pendingKey(orgID, queue), member)
	pipe.ZRem(ctx, delayedKey(orgID, queue), member)
	pipe.HDel(ctx, priorityKey(orgID, queue), member)
	pipe.LPush(ctx, dlqKey(orgID, queue), member)
	_, err := pipe.Exec(ctx)
	return err
}

func (b *RedisBroker) PromoteDueDelayed(ctx context.Context, orgID uuid.UUID, queue string) (int, error) {
	result, err := promoteDelayedScript.Run(ctx, b.client, []string{
		delayedKey(orgID, queue), pendingKey(orgID, queue),
		priorityKey(orgID, queue), sequenceKey(orgID, queue),
	}, time.Now().UnixMilli(), priorityBucketSize).Int()
	if err != nil {
		return 0, fmt.Errorf("redis promote delayed: %w", err)
	}
	return result, nil
}

func (b *RedisBroker) QueueDepth(ctx context.Context, orgID uuid.UUID, queue string) (int64, error) {
	return b.client.ZCard(ctx, pendingKey(orgID, queue)).Result()
}

func (b *RedisBroker) Remove(ctx context.Context, orgID uuid.UUID, queue string, taskID uuid.UUID) error {
	member := message(orgID, taskID)
	pipe := b.client.TxPipeline()
	pipe.ZRem(ctx, pendingKey(orgID, queue), member)
	pipe.ZRem(ctx, processingKey(orgID, queue), member)
	pipe.ZRem(ctx, delayedKey(orgID, queue), member)
	pipe.HDel(ctx, priorityKey(orgID, queue), member)
	pipe.LRem(ctx, dlqKey(orgID, queue), 0, member)
	_, err := pipe.Exec(ctx)
	return err
}

func normalizePriority(priority int) int {
	if priority < 0 {
		return 0
	}
	if priority > 9 {
		return 9
	}
	return priority
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
