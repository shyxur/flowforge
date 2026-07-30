package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/testutil"
)

func TestRedisRebuildRestoresPendingAndDelayedTasks(t *testing.T) {
	now := time.Now().UTC()
	orgID := uuid.New()
	pending := &domain.Task{ID: uuid.New(), OrgID: orgID, Queue: "default", VisibleAt: now.Add(-time.Second)}
	delayed := &domain.Task{ID: uuid.New(), OrgID: orgID, Queue: "email", VisibleAt: now.Add(time.Minute)}
	tasks := []*domain.Task{pending, delayed}
	removed := make(map[uuid.UUID]int)
	active := make(map[uuid.UUID]int)
	future := make(map[uuid.UUID]int)
	marked := make(map[uuid.UUID]int)

	storage := &testutil.StorageStub{
		ListDispatchableTasksFunc: func(_ context.Context, afterID uuid.UUID, _ int) ([]*domain.Task, error) {
			if afterID != uuid.Nil {
				return nil, nil
			}
			return tasks, nil
		},
		MarkDispatchedFunc: func(_ context.Context, _ uuid.UUID, id uuid.UUID, _ time.Time) error {
			marked[id]++
			return nil
		},
	}
	broker := &testutil.BrokerStub{
		RemoveFunc: func(_ context.Context, _ uuid.UUID, _ string, id uuid.UUID) error {
			removed[id]++
			active[id] = 0
			future[id] = 0
			return nil
		},
		EnqueueFunc: func(_ context.Context, task *domain.Task) error {
			active[task.ID]++
			return nil
		},
		EnqueueDelayedFunc: func(_ context.Context, task *domain.Task, _ time.Duration) error {
			future[task.ID]++
			return nil
		},
	}
	rebuilder := NewRedisRebuilder(storage, broker)
	rebuilder.now = func() time.Time { return now }

	for run := 0; run < 2; run++ {
		result, err := rebuilder.Rebuild(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if result.Scanned != 2 || result.Pending != 1 || result.Delayed != 1 {
			t.Fatalf("result = %+v", result)
		}
	}
	if active[pending.ID] != 1 || future[delayed.ID] != 1 {
		t.Fatalf("idempotent state active=%v delayed=%v", active, future)
	}
	if removed[pending.ID] != 2 || removed[delayed.ID] != 2 ||
		marked[pending.ID] != 2 || marked[delayed.ID] != 2 {
		t.Fatalf("remove=%v marked=%v", removed, marked)
	}
}
