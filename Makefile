.PHONY: up down migrate test test-integration lint smoke api worker webhook-worker dashboard redis-rebuild

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

smoke:
	./scripts/smoke.sh

api:
	go run ./cmd/producer

worker:
	go run ./cmd/worker

webhook-worker:
	go run ./cmd/webhook-worker

dashboard:
	npm --prefix apps/dashboard run dev

redis-rebuild:
	go run ./cmd/maintenance redis-rebuild
