package main

import (
	"context"
	"net/http"
	"time"

	"github.com/shyxur/windylane/internal/api"
	redisbroker "github.com/shyxur/windylane/internal/broker/redis"
	"github.com/shyxur/windylane/internal/config"
	metricspkg "github.com/shyxur/windylane/internal/metrics"
	"github.com/shyxur/windylane/internal/ports"
	"github.com/shyxur/windylane/internal/storage/postgres"
	"github.com/shyxur/windylane/internal/usecase"
	"github.com/shyxur/windylane/internal/webhook"
	"github.com/shyxur/windylane/internal/worker"
	"go.uber.org/zap"
)

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

	limiter := redisbroker.NewTokenBucketLimiter(broker.Client(), 2, 5)
	secretCipher := webhook.NewSecretCipher(cfg.WebhookSecretEncryptionKey)
	eventService := usecase.NewWebhookEventService(storage, storage, cfg.WebhookDeliveryMaxAttempts).
		WithMetricRecorder(metricRecorder)
	service := usecase.NewService(storage, broker, eventService).WithMetricRecorder(metricRecorder)
	webhookService := usecase.NewWebhookService(storage, secretCipher, cfg.AllowInsecureLocalWebhooks)
	webhookDeliveryService := usecase.NewWebhookDeliveryLogService(storage)
	workflowService := usecase.NewWorkflowService(storage)
	taskDispatcher := usecase.NewQueueFlowWorkflowTaskDispatcher(service)
	webhookDispatcher := usecase.NewEventForgeWorkflowWebhookDispatcher(
		storage, storage, cfg.WebhookDeliveryMaxAttempts,
	).WithMetricRecorder(metricRecorder)
	workflowExecutionService := usecase.NewWorkflowExecutionService(
		storage, storage, taskDispatcher, webhookDispatcher,
	).WithMetricRecorder(metricRecorder)
	auth := usecase.NewAuthService(storage)
	handler := api.NewHandler(service, logger, webhookService).
		WithWebhookDeliveryService(webhookDeliveryService).
		WithWorkflowService(workflowService).
		WithWorkflowExecutionService(workflowExecutionService)
	router := api.NewRouter(handler, auth, limiter, logger)

	srv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	runCtx, stop := worker.ContextWithSignals(ctx, logger)
	defer stop()
	go workflowExecutionService.ReconciliationLoop(
		runCtx, cfg.WorkflowReconcileInterval, 100,
	)
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("producer HTTP API starting", zap.String("port", cfg.HTTPPort))
		serverErrors <- srv.ListenAndServe()
	}()
	select {
	case <-runCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("producer graceful shutdown failed", zap.Error(err))
		}
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			logger.Fatal("http server failed", zap.Error(err))
		}
	}
}
