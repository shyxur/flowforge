package metrics

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/ports"
	"go.uber.org/zap"
)

type BatchWriter interface {
	AppendMetricEvents(context.Context, []domain.MetricEvent) error
}

type Config struct {
	Capacity      int
	BatchSize     int
	FlushInterval time.Duration
	WriteTimeout  time.Duration
}

type BufferedRecorder struct {
	writer  BatchWriter
	logger  *zap.Logger
	config  Config
	events  chan domain.MetricEvent
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
	closed  atomic.Bool
	dropped atomic.Uint64
}

var _ ports.MetricRecorder = (*BufferedRecorder)(nil)

func NewBufferedRecorder(writer BatchWriter, config Config, logger *zap.Logger) *BufferedRecorder {
	if logger == nil {
		logger = zap.NewNop()
	}
	recorder := &BufferedRecorder{
		writer: writer, logger: logger, config: config,
		events: make(chan domain.MetricEvent, config.Capacity),
		stop:   make(chan struct{}), done: make(chan struct{}),
	}
	go recorder.run()
	return recorder
}

func (recorder *BufferedRecorder) RecordMetric(event domain.MetricEvent) {
	if recorder == nil || recorder.closed.Load() {
		return
	}
	select {
	case recorder.events <- event:
	default:
		recorder.dropped.Add(1)
	}
}

func (recorder *BufferedRecorder) Dropped() uint64 {
	if recorder == nil {
		return 0
	}
	return recorder.dropped.Load()
}

func (recorder *BufferedRecorder) Close(ctx context.Context) error {
	if recorder == nil {
		return nil
	}
	recorder.once.Do(func() {
		recorder.closed.Store(true)
		close(recorder.stop)
	})
	select {
	case <-recorder.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (recorder *BufferedRecorder) run() {
	defer close(recorder.done)
	ticker := time.NewTicker(recorder.config.FlushInterval)
	defer ticker.Stop()
	batch := make([]domain.MetricEvent, 0, recorder.config.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), recorder.config.WriteTimeout)
		err := recorder.writer.AppendMetricEvents(ctx, batch)
		cancel()
		if err != nil {
			recorder.logger.Warn("metric batch write failed",
				zap.Int("event_count", len(batch)), zap.Error(err))
		}
		batch = batch[:0]
	}
	for {
		select {
		case event := <-recorder.events:
			batch = append(batch, event)
			if len(batch) >= recorder.config.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-recorder.stop:
			for {
				select {
				case event := <-recorder.events:
					batch = append(batch, event)
					if len(batch) >= recorder.config.BatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func Record(recorder ports.MetricRecorder, input domain.NewMetricEventInput) {
	if recorder == nil {
		return
	}
	event, err := domain.NewMetricEvent(input)
	if err == nil {
		recorder.RecordMetric(event)
	}
}
