package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisbroker "github.com/shyxur/windylane/internal/broker/redis"
	"github.com/shyxur/windylane/internal/config"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/engine"
	metricspkg "github.com/shyxur/windylane/internal/metrics"
	"github.com/shyxur/windylane/internal/ports"
	"github.com/shyxur/windylane/internal/storage/postgres"
	"github.com/shyxur/windylane/internal/usecase"
	"github.com/shyxur/windylane/internal/worker"
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
	if err := cfg.Validate(); err != nil {
		logger.Fatal("invalid configuration", zap.Error(err))
	}
	ctx := context.Background()

	storage, err := postgres.NewPostgresStorage(ctx, cfg.DBDSN)
	if err != nil {
		logger.Fatal("failed to connect to postgres", zap.Error(err))
	}
	defer storage.Close()
	var metricRecorder ports.MetricRecorder
	var bufferedMetrics *metricspkg.BufferedRecorder
	if cfg.MetricsEnabled {
		bufferedMetrics = metricspkg.NewBufferedRecorder(
			usecase.NewMetricsService(storage),
			metricspkg.Config{
				Capacity: cfg.MetricsBufferCapacity, BatchSize: cfg.MetricsBatchSize,
				FlushInterval: cfg.MetricsFlushInterval, WriteTimeout: cfg.MetricsWriteTimeout,
			},
			logger,
		)
		metricRecorder = bufferedMetrics
		defer func() {
			closeCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
			defer cancel()
			if err := bufferedMetrics.Close(closeCtx); err != nil {
				logger.Warn("metrics shutdown incomplete", zap.Error(err))
			}
		}()
	}

	broker := redisbroker.NewRedisBroker(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer broker.Close()

	limiter := redisbroker.NewTokenBucketLimiter(broker.Client(), cfg.RateLimitPerSec, cfg.RateLimitPerSec*2)

	retryPolicy := domain.DefaultRetryPolicy()
	eventService := usecase.NewWebhookEventService(storage, storage, cfg.WebhookDeliveryMaxAttempts).
		WithMetricRecorder(metricRecorder)
	eng := engine.NewEngine(storage, broker, retryPolicy, cfg.TaskTimeout, logger, eventService).
		WithMetricRecorder(metricRecorder)

	runCtx, stop := worker.ContextWithSignals(ctx, logger)
	defer stop()
	baseWorkerID := fmt.Sprintf("worker-%s", uuid.New().String()[:8])

	if cfg.WorkerDiscoveryEnabled {
		logger.Info("worker discovery starting", zap.Duration("refresh_interval", cfg.WorkerDiscoveryInterval))
		discovery := worker.NewScopeDiscovery(storage, cfg.WorkerDiscoveryInterval, func(scopeCtx context.Context, scope domain.QueueScope) {
			runScope(scopeCtx, cfg, scopeWorkerID(baseWorkerID, scope), scope, broker, storage, eng, limiter, metricRecorder, logger)
		}, logger)
		discovery.Run(runCtx)
	} else {
		orgID, err := uuid.Parse(cfg.OrgID)
		if err != nil {
			logger.Fatal("invalid ORG_ID", zap.Error(err))
		}
		runScope(runCtx, cfg, baseWorkerID, domain.QueueScope{OrgID: orgID, Queue: cfg.QueueName}, broker, storage, eng, limiter, metricRecorder, logger)
	}
	logger.Info("worker exited cleanly")
}

func runScope(
	ctx context.Context,
	cfg *config.Config,
	workerID string,
	scope domain.QueueScope,
	broker ports.Broker,
	storage ports.Storage,
	eng *engine.Engine,
	limiter ports.RateLimiter,
	metricRecorder ports.MetricRecorder,
	logger *zap.Logger,
) {
	handler := &exampleHandler{queue: scope.Queue}
	workerCfg := domain.WorkerConfig{
		WorkerID: workerID, OrgID: scope.OrgID,
		Concurrency: cfg.WorkerConcurrency, RateLimitPerSec: cfg.RateLimitPerSec,
		HeartbeatInterval: cfg.HeartbeatInterval, ShutdownTimeout: cfg.ShutdownTimeout,
	}
	pool := worker.NewPool(workerCfg, scope.Queue, broker, storage, eng, handler, limiter, logger)
	go eng.ReclaimLoop(ctx, scope.OrgID, scope.Queue, cfg.ReclaimInterval)
	go eng.DelayedPromotionLoop(ctx, scope.OrgID, scope.Queue, cfg.PromoteInterval)
	go eng.ReconciliationLoop(ctx, scope.OrgID, scope.Queue, cfg.ReconcileInterval)
	go workerHeartbeatLoop(ctx, storage, metricRecorder, workerID, scope.OrgID, scope.Queue, cfg.HeartbeatInterval, logger)
	logger.Info("worker scope running",
		zap.String("worker_id", workerID), zap.String("org_id", scope.OrgID.String()), zap.String("queue", scope.Queue))
	pool.Run(ctx)
	recordWorkerMetric(metricRecorder, scope.OrgID, workerID, scope.Queue, domain.MetricWorkerStopped, time.Now().UTC())
}

func scopeWorkerID(base string, scope domain.QueueScope) string {
	sum := sha256.Sum256([]byte(scope.Key()))
	return fmt.Sprintf("%s-%x", base, sum[:4])
}

func workerHeartbeatLoop(ctx context.Context, storage ports.Storage, metricRecorder ports.MetricRecorder, workerID string, orgID uuid.UUID, queue string, interval time.Duration, logger *zap.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	registered := false
	send := func() {
		now := time.Now().UTC()
		err := storage.UpsertWorkerHeartbeat(ctx, &domain.Worker{
			ID: workerID, OrgID: orgID, Queue: queue, Status: "online",
			LastHeartbeatAt: now, CreatedAt: now,
		})
		if err != nil {
			logger.Warn("worker registry heartbeat failed", zap.Error(err))
			return
		}
		eventType := domain.MetricWorkerHeartbeat
		if !registered {
			eventType = domain.MetricWorkerRegistered
			registered = true
		}
		recordWorkerMetric(metricRecorder, orgID, workerID, queue, eventType, now)
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

func recordWorkerMetric(
	recorder ports.MetricRecorder,
	orgID uuid.UUID,
	workerID, queue string,
	eventType domain.MetricEventType,
	now time.Time,
) {
	status := "online"
	if eventType == domain.MetricWorkerStopped {
		status = "offline"
	} else if eventType == domain.MetricWorkerStale {
		status = "stale"
	}
	metricspkg.Record(recorder, domain.NewMetricEventInput{
		OrganizationID: orgID, Source: domain.MetricSourceWorker,
		EventType: eventType, ResourceType: domain.MetricResourceWorker,
		ResourceID: workerID, Queue: queue, Status: status, OccurredAt: now,
		TransitionKey: domain.MetricTransitionKey(eventType, now.UnixNano()),
	})
}
