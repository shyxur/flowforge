package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/testutil"
	"go.uber.org/zap"
)

type failingHandler struct{}

func (failingHandler) QueueName() string { return "default" }
func (failingHandler) Handle(context.Context, []byte) error {
	return errors.New("failed")
}

type successfulHandler struct{}

func (successfulHandler) QueueName() string                    { return "default" }
func (successfulHandler) Handle(context.Context, []byte) error { return nil }

type metricRecorderSpy struct {
	events []domain.MetricEvent
}

func (recorder *metricRecorderSpy) RecordMetric(event domain.MetricEvent) {
	recorder.events = append(recorder.events, event)
}

func (recorder *metricRecorderSpy) has(eventType domain.MetricEventType) bool {
	for _, event := range recorder.events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func TestExhaustedTaskMovesToDLQ(t *testing.T) {
	orgID, taskID := uuid.New(), uuid.New()
	storageMoved, brokerMoved := false, false
	storage := &testutil.StorageStub{
		MoveToDeadLetterFunc: func(_ context.Context, gotOrg, gotID uuid.UUID, _ string, _ time.Time) error {
			storageMoved = gotOrg == orgID && gotID == taskID
			return nil
		},
	}
	broker := &testutil.BrokerStub{
		MoveToDeadLetterFunc: func(_ context.Context, gotOrg uuid.UUID, queue string, gotID uuid.UUID) error {
			brokerMoved = gotOrg == orgID && queue == "default" && gotID == taskID
			return nil
		},
	}
	task := &domain.Task{
		ID: taskID, OrgID: orgID, Queue: "default", Attempts: 1, MaxAttempts: 1,
		Status: domain.StatusProcessing, TaskTimeout: time.Second,
	}
	metrics := &metricRecorderSpy{}
	result := NewEngine(storage, broker, domain.DefaultRetryPolicy(), time.Second, zap.NewNop()).
		WithMetricRecorder(metrics).
		Execute(context.Background(), task, "worker-1", failingHandler{})
	if result.Outcome != "dead_letter" || !storageMoved || !brokerMoved {
		t.Fatalf("result=%+v storageMoved=%v brokerMoved=%v", result, storageMoved, brokerMoved)
	}
	if !metrics.has(domain.MetricTaskFailed) || !metrics.has(domain.MetricTaskDeadLettered) {
		t.Fatal("task failure lifecycle metrics were not recorded")
	}
}

func TestTaskSuccessAndRetryMetrics(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		metrics := &metricRecorderSpy{}
		task := &domain.Task{
			ID: uuid.New(), OrgID: uuid.New(), Queue: "default",
			Status: domain.StatusProcessing, Attempts: 1, MaxAttempts: 3,
			TaskTimeout: time.Second, UpdatedAt: time.Now().Add(-time.Second),
		}
		result := NewEngine(
			&testutil.StorageStub{}, &testutil.BrokerStub{},
			domain.DefaultRetryPolicy(), time.Second, zap.NewNop(),
		).WithMetricRecorder(metrics).Execute(
			context.Background(), task, "worker", successfulHandler{},
		)
		if result.Err != nil || !metrics.has(domain.MetricTaskSucceeded) {
			t.Fatalf("result=%+v metrics=%+v", result, metrics.events)
		}
	})

	t.Run("retry", func(t *testing.T) {
		metrics := &metricRecorderSpy{}
		task := &domain.Task{
			ID: uuid.New(), OrgID: uuid.New(), Queue: "default",
			Status: domain.StatusProcessing, Attempts: 1, MaxAttempts: 3,
			TaskTimeout: time.Second, UpdatedAt: time.Now().Add(-time.Second),
		}
		result := NewEngine(
			&testutil.StorageStub{}, &testutil.BrokerStub{},
			domain.DefaultRetryPolicy(), time.Second, zap.NewNop(),
		).WithMetricRecorder(metrics).Execute(
			context.Background(), task, "worker", failingHandler{},
		)
		if result.Outcome != "retried" ||
			!metrics.has(domain.MetricTaskFailed) ||
			!metrics.has(domain.MetricTaskRetryScheduled) {
			t.Fatalf("result=%+v metrics=%+v", result, metrics.events)
		}
	})
}
