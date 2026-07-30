package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/testutil"
	"go.uber.org/zap"
)

type workerMetricRecorder struct {
	mu     sync.Mutex
	events []domain.MetricEventType
}

func (recorder *workerMetricRecorder) RecordMetric(event domain.MetricEvent) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.events = append(recorder.events, event.EventType)
}

func (recorder *workerMetricRecorder) has(eventType domain.MetricEventType) bool {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for _, current := range recorder.events {
		if current == eventType {
			return true
		}
	}
	return false
}

func TestWorkerLifecycleMetrics(t *testing.T) {
	orgID := uuid.New()
	heartbeats := make(chan struct{}, 3)
	storage := &testutil.StorageStub{
		UpsertWorkerHeartbeatFunc: func(context.Context, *domain.Worker) error {
			heartbeats <- struct{}{}
			return nil
		},
	}
	recorder := &workerMetricRecorder{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		workerHeartbeatLoop(
			ctx, storage, recorder, "worker-test", orgID, "default",
			5*time.Millisecond, zap.NewNop(),
		)
		close(done)
	}()
	<-heartbeats
	<-heartbeats
	cancel()
	<-done
	recordWorkerMetric(
		recorder, orgID, "worker-test", "default",
		domain.MetricWorkerStopped, time.Now().UTC(),
	)
	if !recorder.has(domain.MetricWorkerRegistered) ||
		!recorder.has(domain.MetricWorkerHeartbeat) ||
		!recorder.has(domain.MetricWorkerStopped) {
		t.Fatalf("worker lifecycle metrics=%v", recorder.events)
	}
}
