# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,id=windylane-go-mod,target=/go/pkg/mod go mod download

COPY cmd/producer ./cmd/producer
COPY cmd/worker ./cmd/worker
COPY cmd/webhook-worker ./cmd/webhook-worker
COPY internal ./internal

RUN --mount=type=cache,id=windylane-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=windylane-go-build,target=/root/.cache/go-build \
    mkdir -p /out && \
    CGO_ENABLED=0 go build -o /out/ \
      ./cmd/producer ./cmd/worker ./cmd/webhook-worker

FROM alpine:3.20 AS runtime
RUN adduser -D -u 1000 appuser
USER appuser

FROM runtime AS producer
COPY --from=builder /out/producer /producer
EXPOSE 8080
ENTRYPOINT ["/producer"]

FROM runtime AS worker
COPY --from=builder /out/worker /worker
ENTRYPOINT ["/worker"]

FROM runtime AS webhook-worker
COPY --from=builder /out/webhook-worker /webhook-worker
ENTRYPOINT ["/webhook-worker"]
