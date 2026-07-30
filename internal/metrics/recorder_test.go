package metrics

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shyxur/windylane/internal/domain"
	"go.uber.org/zap"
)

type writerStub struct {
	mu      sync.Mutex
	batches [][]domain.MetricEvent
	err     error
	block   chan struct{}
}

func (writer *writerStub) AppendMetricEvents(_ context.Context, events []domain.MetricEvent) error {
	if writer.block != nil {
		<-writer.block
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.batches = append(writer.batches, append([]domain.MetricEvent(nil), events...))
	return writer.err
}

func (writer *writerStub) count() int {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	total := 0
	for _, batch := range writer.batches {
		total += len(batch)
	}
	return total
}

func TestBufferedRecorderBatchesAndDrainsOnClose(t *testing.T) {
	writer := &writerStub{}
	recorder := NewBufferedRecorder(writer, Config{
		Capacity: 10, BatchSize: 2, FlushInterval: time.Hour, WriteTimeout: time.Second,
	}, zap.NewNop())
	recorder.RecordMetric(domain.MetricEvent{})
	recorder.RecordMetric(domain.MetricEvent{})
	recorder.RecordMetric(domain.MetricEvent{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := recorder.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if got := writer.count(); got != 3 {
		t.Fatalf("wrote %d events, want 3", got)
	}
}

func TestBufferedRecorderDropsWhenBoundedBufferIsFull(t *testing.T) {
	block := make(chan struct{})
	writer := &writerStub{block: block}
	recorder := NewBufferedRecorder(writer, Config{
		Capacity: 1, BatchSize: 1, FlushInterval: time.Hour, WriteTimeout: time.Second,
	}, zap.NewNop())
	recorder.RecordMetric(domain.MetricEvent{})
	time.Sleep(10 * time.Millisecond)
	recorder.RecordMetric(domain.MetricEvent{})
	recorder.RecordMetric(domain.MetricEvent{})
	if recorder.Dropped() == 0 {
		t.Fatal("expected at least one bounded-buffer drop")
	}
	close(block)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := recorder.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestBufferedRecorderWriterFailureIsIsolated(t *testing.T) {
	writer := &writerStub{err: errors.New("unavailable")}
	recorder := NewBufferedRecorder(writer, Config{
		Capacity: 2, BatchSize: 1, FlushInterval: time.Hour, WriteTimeout: time.Second,
	}, zap.NewNop())
	recorder.RecordMetric(domain.MetricEvent{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := recorder.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if writer.count() != 1 {
		t.Fatal("writer was not invoked")
	}
}
