package main

import (
	"context"
	"net/http"
	"time"

	"github.com/shyxur/flowforge/internal/api"
	redisbroker "github.com/shyxur/flowforge/internal/broker/redis"
	"github.com/shyxur/flowforge/internal/config"
	"github.com/shyxur/flowforge/internal/storage/postgres"
	"github.com/shyxur/flowforge/internal/usecase"
	"github.com/shyxur/flowforge/internal/webhook"
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

	broker := redisbroker.NewRedisBroker(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer broker.Close()

	limiter := redisbroker.NewTokenBucketLimiter(broker.Client(), 2, 5)
	service := usecase.NewService(storage, broker)
	secretCipher := webhook.NewSecretCipher(cfg.WebhookSecretEncryptionKey)
	webhookService := usecase.NewWebhookService(storage, secretCipher, cfg.AllowInsecureLocalWebhooks)
	auth := usecase.NewAuthService(storage)
	handler := api.NewHandler(service, logger, webhookService)
	router := api.NewRouter(handler, auth, limiter, logger)

	srv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	logger.Info("producer HTTP API starting", zap.String("port", cfg.HTTPPort))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("http server failed", zap.Error(err))
	}
}
