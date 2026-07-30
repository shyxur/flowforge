package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shyxur/windylane/internal/domain"
	"go.uber.org/zap"
)

type scopeSourceStub struct {
	mu     sync.Mutex
	scopes []domain.QueueScope
}

func (s *scopeSourceStub) ListActiveQueueScopes(context.Context) ([]domain.QueueScope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.QueueScope(nil), s.scopes...), nil
}

func (s *scopeSourceStub) set(scopes ...domain.QueueScope) {
	s.mu.Lock()
	s.scopes = scopes
	s.mu.Unlock()
}

func TestScopeDiscoveryStartsAndStopsScopes(t *testing.T) {
	orgID := uuid.New()
	first := domain.QueueScope{OrgID: orgID, Queue: "first"}
	second := domain.QueueScope{OrgID: orgID, Queue: "second"}
	source := &scopeSourceStub{scopes: []domain.QueueScope{first}}
	started := make(chan string, 2)
	stopped := make(chan string, 2)
	discovery := NewScopeDiscovery(source, time.Hour, func(ctx context.Context, scope domain.QueueScope) {
		started <- scope.Key()
		<-ctx.Done()
		stopped <- scope.Key()
	}, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		discovery.Run(ctx)
		close(done)
	}()
	if got := receive(t, started); got != first.Key() {
		t.Fatalf("started = %s, want %s", got, first.Key())
	}

	source.set(second)
	if err := discovery.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, stopped); got != first.Key() {
		t.Fatalf("stopped = %s, want %s", got, first.Key())
	}
	if got := receive(t, started); got != second.Key() {
		t.Fatalf("started = %s, want %s", got, second.Key())
	}

	cancel()
	<-done
	if got := receive(t, stopped); got != second.Key() {
		t.Fatalf("shutdown scope = %s, want %s", got, second.Key())
	}
}

func receive(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for discovery event")
		return ""
	}
}
