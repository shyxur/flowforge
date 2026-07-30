package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	redisbroker "github.com/shyxur/flowforge/internal/broker/redis"
	"github.com/shyxur/flowforge/internal/config"
	"github.com/shyxur/flowforge/internal/storage/postgres"
	"github.com/shyxur/flowforge/internal/usecase"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "redis-rebuild" {
		fmt.Fprintln(os.Stderr, "usage: maintenance redis-rebuild")
		os.Exit(2)
	}
	ctx := context.Background()
	cfg := config.Load()
	storage, err := postgres.NewPostgresStorage(ctx, cfg.DBDSN)
	if err != nil {
		fatal(err)
	}
	defer storage.Close()
	broker := redisbroker.NewRedisBroker(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer broker.Close()

	result, err := usecase.NewRedisRebuilder(storage, broker).Rebuild(ctx)
	if err != nil {
		fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
