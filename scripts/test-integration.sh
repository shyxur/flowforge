#!/bin/sh
set -eu

project="flowforge-integration"
compose_file="docker-compose.integration.yml"

cleanup() {
  docker compose -p "$project" -f "$compose_file" down -v --remove-orphans >/dev/null 2>&1 || true
}

trap cleanup EXIT INT TERM
cleanup

docker compose -p "$project" -f "$compose_file" up -d --wait postgres redis
docker compose -p "$project" -f "$compose_file" run --rm migrate

QUEUEFLOW_INTEGRATION=1 \
INTEGRATION_DB_DSN="postgres://taskqueue:taskqueue@localhost:55432/taskqueue?sslmode=disable" \
INTEGRATION_REDIS_ADDR="localhost:56379" \
go test -tags=integration ./internal/integration ./internal/broker/redis -count=1
