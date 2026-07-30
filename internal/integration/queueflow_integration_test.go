//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	redisbroker "github.com/shyxur/flowforge/internal/broker/redis"
	"github.com/shyxur/flowforge/internal/domain"
	"github.com/shyxur/flowforge/internal/engine"
	"github.com/shyxur/flowforge/internal/storage/postgres"
	"github.com/shyxur/flowforge/internal/usecase"
	"go.uber.org/zap"
)

const devOrgID = "00000000-0000-4000-8000-000000000001"

type failingHandler struct{ queue string }

func (h failingHandler) QueueName() string { return h.queue }
func (h failingHandler) Handle(context.Context, []byte) error {
	return errors.New("integration failure")
}

func TestQueueFlowLifecycle(t *testing.T) {
	if os.Getenv("QUEUEFLOW_INTEGRATION") != "1" {
		t.Skip("run with make test-integration")
	}

	ctx := context.Background()
	dsn := os.Getenv("INTEGRATION_DB_DSN")
	redisAddr := os.Getenv("INTEGRATION_REDIS_ADDR")
	if dsn == "" || redisAddr == "" {
		t.Fatal("integration database and Redis addresses are required")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "TRUNCATE tasks, workers"); err != nil {
		t.Fatal(err)
	}

	assertMigrationsAndSeed(t, ctx, pool)

	storage, err := postgres.NewPostgresStorage(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	broker := redisbroker.NewRedisBroker(redisAddr, "", 0)
	defer broker.Close()
	if err := broker.Client().FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	orgID := uuid.MustParse(devOrgID)
	queue := "integration-" + uuid.NewString()[:8]
	service := usecase.NewService(storage, broker)
	input := usecase.CreateTaskInput{
		OrgID: orgID, IdempotencyKey: "integration-idempotency", Queue: queue,
		Payload: json.RawMessage(`{"kind":"integration"}`), MaxRetries: 0,
		Timeout: time.Second, VisibilityTimeout: 30 * time.Second,
		BackoffStrategy: "exponential",
	}

	task, replay, err := service.CreateTask(ctx, input)
	if err != nil || replay {
		t.Fatalf("create task: replay=%v err=%v", replay, err)
	}
	replayed, replay, err := service.CreateTask(ctx, input)
	if err != nil || !replay || replayed.ID != task.ID {
		t.Fatalf("idempotent replay: task=%v replay=%v err=%v", replayed, replay, err)
	}
	input.Payload = json.RawMessage(`{"kind":"different"}`)
	if _, _, err := service.CreateTask(ctx, input); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("fingerprint conflict error = %v", err)
	}

	pendingKey := "queueflow:v1:org:" + orgID.String() + ":queue:" + queue + ":pending"
	if depth, err := broker.Client().ZCard(ctx, pendingKey).Result(); err != nil || depth != 1 {
		t.Fatalf("tenant pending key depth=%d err=%v", depth, err)
	}

	taskID, err := broker.Dequeue(ctx, orgID, queue, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := storage.ClaimForProcessing(ctx, orgID, taskID, "integration-worker", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Status != domain.StatusProcessing || claimed.Attempts != 1 {
		t.Fatalf("claim state = %s attempts=%d", claimed.Status, claimed.Attempts)
	}

	eng := engine.NewEngine(storage, broker, domain.DefaultRetryPolicy(), time.Second, zap.NewNop())
	result := eng.Execute(ctx, claimed, "integration-worker", failingHandler{queue: queue})
	if result.Outcome != "dead_letter" {
		t.Fatalf("engine outcome = %s, err=%v", result.Outcome, result.Err)
	}
	dead, err := storage.GetByID(ctx, orgID, task.ID)
	if err != nil || dead.Status != domain.StatusDeadLetter {
		t.Fatalf("DLQ state = %v err=%v", dead, err)
	}

	requeued, err := service.RequeueDLQ(ctx, orgID, task.ID)
	if err != nil || requeued.Status != domain.StatusPending {
		t.Fatalf("DLQ requeue = %v err=%v", requeued, err)
	}
	if _, err := storage.GetByID(ctx, uuid.New(), task.ID); !errors.Is(err, domain.ErrTaskNotFound) {
		t.Fatalf("cross-org read error = %v", err)
	}
	page, err := storage.ListTasks(ctx, uuid.New(), domain.TaskFilter{Limit: 10})
	if err != nil || len(page.Tasks) != 0 {
		t.Fatalf("cross-org list = %+v err=%v", page, err)
	}
}

func assertMigrationsAndSeed(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var version int
	var dirty bool
	if err := pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatal(err)
	}
	if version != 2 || dirty {
		t.Fatalf("migration version=%d dirty=%v", version, dirty)
	}
	var orgCount, keyCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM organizations WHERE id=$1", devOrgID).Scan(&orgCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM api_keys WHERE org_id=$1", devOrgID).Scan(&keyCount); err != nil {
		t.Fatal(err)
	}
	if orgCount != 1 || keyCount == 0 {
		t.Fatalf("seed orgs=%d api_keys=%d", orgCount, keyCount)
	}
}
