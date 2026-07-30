package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisbroker "github.com/shyxur/flowforge/internal/broker/redis"
	"github.com/shyxur/flowforge/internal/config"
	"github.com/shyxur/flowforge/internal/domain"
	"github.com/shyxur/flowforge/internal/engine"
	"github.com/shyxur/flowforge/internal/ports"
	"github.com/shyxur/flowforge/internal/storage/postgres"
	"github.com/shyxur/flowforge/internal/worker"
	"go.uber.org/zap"
)

// exampleHandler is a placeholder job handler — replace with real business
// logic per queue. Register additional handlers via engine.HandlerRegistry
// if you run multiple queues in one process.
type exampleHandler struct {
	queue string
}

func (h *exampleHandler) QueueName() string { return h.queue }

func (h *exampleHandler) Handle(ctx context.Context, payload []byte) error {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return fmt.Errorf("invalid payload: %w", err) // fatal: won't fix itself on retry, but still consumes retry budget
	}
	// TODO: business logic here.
	return nil
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := config.Load()
	ctx := context.Background()
	orgID, err := uuid.Parse(cfg.OrgID)
	if err != nil {
		logger.Fatal("invalid ORG_ID", zap.Error(err))
	}

	storage, err := postgres.NewPostgresStorage(ctx, cfg.DBDSN)
	if err != nil {
		logger.Fatal("failed to connect to postgres", zap.Error(err))
	}
	defer storage.Close()

	broker := redisbroker.NewRedisBroker(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer broker.Close()

	limiter := redisbroker.NewTokenBucketLimiter(broker.Client(), cfg.RateLimitPerSec, cfg.RateLimitPerSec*2)

	retryPolicy := domain.DefaultRetryPolicy()
	eng := engine.NewEngine(storage, broker, retryPolicy, cfg.TaskTimeout, logger)

	handler := &exampleHandler{queue: cfg.QueueName}

	workerCfg := domain.WorkerConfig{
		WorkerID:          fmt.Sprintf("worker-%s", uuid.New().String()[:8]),
		OrgID:             orgID,
		Concurrency:       cfg.WorkerConcurrency,
		RateLimitPerSec:   cfg.RateLimitPerSec,
		HeartbeatInterval: cfg.HeartbeatInterval,
		ShutdownTimeout:   cfg.ShutdownTimeout,
	}

	pool := worker.NewPool(workerCfg, cfg.QueueName, broker, storage, eng, handler, limiter, logger)

	runCtx, stop := worker.ContextWithSignals(ctx, logger)
	defer stop()

	// Background loops: crashed-worker reclaim + delayed(backoff) promotion.
	go eng.ReclaimLoop(runCtx, orgID, cfg.QueueName, cfg.ReclaimInterval)
	go eng.DelayedPromotionLoop(runCtx, orgID, cfg.QueueName, cfg.PromoteInterval)
	go eng.ReconciliationLoop(runCtx, orgID, cfg.QueueName, cfg.ReconcileInterval)
	go workerHeartbeatLoop(runCtx, storage, workerCfg.WorkerID, orgID, cfg.QueueName, cfg.HeartbeatInterval, logger)

	logger.Info("worker starting", zap.String("worker_id", workerCfg.WorkerID), zap.String("queue", cfg.QueueName))
	pool.Run(runCtx) // blocks until drained after SIGTERM
	logger.Info("worker exited cleanly")
}

func workerHeartbeatLoop(ctx context.Context, storage ports.Storage, workerID string, orgID uuid.UUID, queue string, interval time.Duration, logger *zap.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	send := func() {
		now := time.Now().UTC()
		err := storage.UpsertWorkerHeartbeat(ctx, &domain.Worker{
			ID: workerID, OrgID: orgID, Queue: queue, Status: "online",
			LastHeartbeatAt: now, CreatedAt: now,
		})
		if err != nil {
			logger.Warn("worker registry heartbeat failed", zap.Error(err))
		}
	}
	send()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}
