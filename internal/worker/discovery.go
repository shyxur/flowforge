package worker

import (
	"context"
	"sync"
	"time"

	"github.com/shyxur/windylane/internal/domain"
	"go.uber.org/zap"
)

type ScopeSource interface {
	ListActiveQueueScopes(ctx context.Context) ([]domain.QueueScope, error)
}

type ScopeRunner func(ctx context.Context, scope domain.QueueScope)

type ScopeDiscovery struct {
	source   ScopeSource
	interval time.Duration
	runner   ScopeRunner
	logger   *zap.Logger

	mu      sync.Mutex
	running map[string]context.CancelFunc
	wg      sync.WaitGroup
}

func NewScopeDiscovery(source ScopeSource, interval time.Duration, runner ScopeRunner, logger *zap.Logger) *ScopeDiscovery {
	return &ScopeDiscovery{
		source: source, interval: interval, runner: runner, logger: logger,
		running: make(map[string]context.CancelFunc),
	}
}

func (d *ScopeDiscovery) Run(ctx context.Context) {
	if err := d.refresh(ctx); err != nil {
		d.logger.Error("worker scope discovery failed", zap.Error(err))
	}
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			d.stopAll()
			return
		case <-ticker.C:
			if err := d.refresh(ctx); err != nil {
				d.logger.Error("worker scope discovery failed", zap.Error(err))
			}
		}
	}
}

func (d *ScopeDiscovery) refresh(parent context.Context) error {
	scopes, err := d.source.ListActiveQueueScopes(parent)
	if err != nil {
		return err
	}
	wanted := make(map[string]domain.QueueScope, len(scopes))
	for _, scope := range scopes {
		wanted[scope.Key()] = scope
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	for key, cancel := range d.running {
		if _, ok := wanted[key]; !ok {
			cancel()
			delete(d.running, key)
			d.logger.Info("worker scope stopped", zap.String("scope", key))
		}
	}
	for key, scope := range wanted {
		if _, ok := d.running[key]; ok {
			continue
		}
		scopeCtx, cancel := context.WithCancel(parent)
		d.running[key] = cancel
		d.wg.Add(1)
		go func(scope domain.QueueScope) {
			defer d.wg.Done()
			d.runner(scopeCtx, scope)
		}(scope)
		d.logger.Info("worker scope started", zap.String("scope", key))
	}
	return nil
}

func (d *ScopeDiscovery) stopAll() {
	d.mu.Lock()
	for key, cancel := range d.running {
		cancel()
		delete(d.running, key)
	}
	d.mu.Unlock()
	d.wg.Wait()
}
