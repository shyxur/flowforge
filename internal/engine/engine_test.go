package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/distributed-task-queue/internal/domain"
	"github.com/shyxur/distributed-task-queue/internal/testutil"
	"go.uber.org/zap"
)

type failingHandler struct{}

func (failingHandler) QueueName() string { return "default" }
func (failingHandler) Handle(context.Context, []byte) error {
	return errors.New("failed")
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
	result := NewEngine(storage, broker, domain.DefaultRetryPolicy(), time.Second, zap.NewNop()).
		Execute(context.Background(), task, "worker-1", failingHandler{})
	if result.Outcome != "dead_letter" || !storageMoved || !brokerMoved {
		t.Fatalf("result=%+v storageMoved=%v brokerMoved=%v", result, storageMoved, brokerMoved)
	}
}
