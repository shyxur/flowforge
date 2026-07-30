package usecase

import (
	"sync"

	"github.com/shyxur/windylane/internal/domain"
)

type metricRecorderSpy struct {
	mu     sync.Mutex
	events []domain.MetricEvent
}

func (recorder *metricRecorderSpy) RecordMetric(event domain.MetricEvent) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.events = append(recorder.events, event)
}

func (recorder *metricRecorderSpy) has(eventType domain.MetricEventType) bool {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for _, event := range recorder.events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}
