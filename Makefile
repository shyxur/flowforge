.PHONY: up down migrate test test-integration lint api worker

up:
	docker compose up --build -d

down:
	docker compose down

migrate:
	docker compose run --rm migrate

test:
	go test ./...

test-integration:
	sh ./scripts/test-integration.sh

lint:
	go vet ./...

api:
	go run ./cmd/producer

worker:
	go run ./cmd/worker
