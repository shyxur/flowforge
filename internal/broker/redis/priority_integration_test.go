//go:build integration

package redis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
)

func TestPriorityDispatchIntegration(t *testing.T) {
	if os.Getenv("QUEUEFLOW_INTEGRATION") != "1" {
		t.Skip("run with make test-integration")
	}
	addr := os.Getenv("INTEGRATION_REDIS_ADDR")
	if addr == "" {
		t.Fatal("INTEGRATION_REDIS_ADDR is required")
	}
	ctx := context.Background()
	broker := NewRedisBroker(addr, "", 0)
	defer broker.Close()
	orgID := uuid.New()

	t.Run("higher priority first", func(t *testing.T) {
		flush(t, ctx, broker)
		low := integrationTask(orgID, "priority", 1)
		high := integrationTask(orgID, "priority", 9)
		mustEnqueue(t, ctx, broker, low)
		mustEnqueue(t, ctx, broker, high)
		assertDequeue(t, ctx, broker, high)
		assertDequeue(t, ctx, broker, low)
	})

	t.Run("same priority FIFO", func(t *testing.T) {
		flush(t, ctx, broker)
		first := integrationTask(orgID, "fifo", 5)
		second := integrationTask(orgID, "fifo", 5)
		mustEnqueue(t, ctx, broker, first)
		mustEnqueue(t, ctx, broker, second)
		assertDequeue(t, ctx, broker, first)
		assertDequeue(t, ctx, broker, second)
	})

	t.Run("delayed task keeps priority", func(t *testing.T) {
		flush(t, ctx, broker)
		low := integrationTask(orgID, "delayed", 1)
		high := integrationTask(orgID, "delayed", 9)
		mustEnqueue(t, ctx, broker, low)
		if err := broker.EnqueueDelayed(ctx, high, 20*time.Millisecond); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
		if promoted, err := broker.PromoteDueDelayed(ctx, orgID, "delayed"); err != nil || promoted != 1 {
			t.Fatalf("promoted=%d err=%v", promoted, err)
		}
		assertDequeue(t, ctx, broker, high)
		assertDequeue(t, ctx, broker, low)
	})

	t.Run("organization isolation", func(t *testing.T) {
		flush(t, ctx, broker)
		otherOrg := uuid.New()
		first := integrationTask(orgID, "isolated", 3)
		other := integrationTask(otherOrg, "isolated", 9)
		mustEnqueue(t, ctx, broker, first)
		mustEnqueue(t, ctx, broker, other)
		assertDequeue(t, ctx, broker, other)
		if depth, err := broker.QueueDepth(ctx, orgID, "isolated"); err != nil || depth != 1 {
			t.Fatalf("first org depth=%d err=%v", depth, err)
		}
	})
}

func integrationTask(orgID uuid.UUID, queue string, priority int) *domain.Task {
	return &domain.Task{ID: uuid.New(), OrgID: orgID, Queue: queue, Priority: priority}
}

func mustEnqueue(t *testing.T, ctx context.Context, broker *RedisBroker, task *domain.Task) {
	t.Helper()
	if err := broker.Enqueue(ctx, task); err != nil {
		t.Fatal(err)
	}
}

func assertDequeue(t *testing.T, ctx context.Context, broker *RedisBroker, expected *domain.Task) {
	t.Helper()
	id, err := broker.Dequeue(ctx, expected.OrgID, expected.Queue, time.Second)
	if err != nil || id != expected.ID {
		t.Fatalf("dequeued=%s want=%s err=%v", id, expected.ID, err)
	}
	if err := broker.Ack(ctx, expected.OrgID, expected.Queue, id); err != nil {
		t.Fatal(err)
	}
}

func flush(t *testing.T, ctx context.Context, broker *RedisBroker) {
	t.Helper()
	if err := broker.Client().FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
}
