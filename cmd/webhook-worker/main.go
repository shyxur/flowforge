package main

import (
	"context"
	"time"

	"github.com/shyxur/windylane/internal/config"
	metricspkg "github.com/shyxur/windylane/internal/metrics"
	"github.com/shyxur/windylane/internal/ports"
	"github.com/shyxur/windylane/internal/storage/postgres"
	"github.com/shyxur/windylane/internal/usecase"
	webhookinfra "github.com/shyxur/windylane/internal/webhook"
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

	secretCipher := webhookinfra.NewSecretCipher(cfg.WebhookSecretEncryptionKey)
	deliveryWorker := usecase.NewWebhookDeliveryWorker(
		storage,
		storage,
		secretCipher,
		webhookinfra.HMACSigner{},
		webhookinfra.NewHTTPClient(cfg.WebhookDeliveryTimeout, webhookinfra.DefaultResponseBodyLimit),
		cfg.WebhookDeliveryBatchSize,
		cfg.WebhookDeliveryInitialBackoff,
		cfg.WebhookDeliveryMaxBackoff,
	).WithMetricRecorder(metricRecorder)

	runCtx, stop := worker.ContextWithSignals(ctx, logger)
	defer stop()
	ticker := time.NewTicker(cfg.WebhookDeliveryPollInterval)
	defer ticker.Stop()

	logger.Info("webhook delivery worker starting",
		zap.Duration("poll_interval", cfg.WebhookDeliveryPollInterval),
		zap.Int("batch_size", cfg.WebhookDeliveryBatchSize))
	process := func() {
		count, processErr := deliveryWorker.ProcessDue(runCtx, time.Now().UTC())
		if processErr != nil {
			logger.Error("webhook delivery batch failed", zap.Error(processErr))
			return
		}
		if count > 0 {
			logger.Info("webhook delivery batch processed", zap.Int("count", count))
		}
	}
	process()
	for {
		select {
		case <-runCtx.Done():
			logger.Info("webhook delivery worker exited cleanly")
			return
		case <-ticker.C:
			process()
		}
	}
}
