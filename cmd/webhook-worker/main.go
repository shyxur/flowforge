package main

import (
	"context"
	"time"

	"github.com/shyxur/flowforge/internal/config"
	"github.com/shyxur/flowforge/internal/storage/postgres"
	"github.com/shyxur/flowforge/internal/usecase"
	webhookinfra "github.com/shyxur/flowforge/internal/webhook"
	"github.com/shyxur/flowforge/internal/worker"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := config.Load()
	ctx := context.Background()
	storage, err := postgres.NewPostgresStorage(ctx, cfg.DBDSN)
	if err != nil {
		logger.Fatal("failed to connect to postgres", zap.Error(err))
	}
	defer storage.Close()

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
	)

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
