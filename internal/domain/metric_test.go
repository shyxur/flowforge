package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func validMetricInput() NewMetricEventInput {
	return NewMetricEventInput{
		OrganizationID: uuid.New(), Source: MetricSourceQueueFlow,
		EventType: MetricTaskStarted, ResourceType: MetricResourceTask,
		ResourceID: uuid.NewString(), Queue: "email", Status: "processing",
		OccurredAt: time.Now().UTC(), TransitionKey: "attempt:1",
	}
}

func TestNewMetricEventDeterministicIdentity(t *testing.T) {
	input := validMetricInput()
	first, err := NewMetricEvent(input)
	if err != nil {
		t.Fatal(err)
	}
	input.OccurredAt = input.OccurredAt.Add(time.Minute)
	second, err := NewMetricEvent(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("same transition must have same identity: %s != %s", first.ID, second.ID)
	}
	input.TransitionKey = "attempt:2"
	third, err := NewMetricEvent(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == third.ID {
		t.Fatal("different transitions must have different identities")
	}
}

func TestMetricEventValidationRejectsInvalidSourcePairAndMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*NewMetricEventInput)
	}{
		{"source event mismatch", func(input *NewMetricEventInput) {
			input.EventType = MetricDeliveryStarted
		}},
		{"resource mismatch", func(input *NewMetricEventInput) {
			input.ResourceType = MetricResourceWorker
		}},
		{"negative duration", func(input *NewMetricEventInput) {
			value := int64(-1)
			input.DurationMS = &value
		}},
		{"oversized error category", func(input *NewMetricEventInput) {
			input.Metadata.ErrorCode = string(make([]byte, 65))
		}},
		{"resource too long", func(input *NewMetricEventInput) {
			input.ResourceID = string(make([]byte, 256))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validMetricInput()
			test.mutate(&input)
			if _, err := NewMetricEvent(input); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestMetricEventContainsNoArbitraryMetadata(t *testing.T) {
	event, err := NewMetricEvent(validMetricInput())
	if err != nil {
		t.Fatal(err)
	}
	if event.Metadata != (MetricMetadata{}) {
		t.Fatalf("unexpected metadata: %#v", event.Metadata)
	}
}

func TestMetricEventFilterBounds(t *testing.T) {
	now := time.Now().UTC()
	valid := MetricEventFilter{From: now.Add(-time.Hour), To: now, Limit: 100}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.From = now.Add(-32 * 24 * time.Hour)
	if err := valid.Validate(); err == nil {
		t.Fatal("expected range validation error")
	}
	valid.From = now.Add(-time.Hour)
	valid.Source = "unknown"
	if err := valid.Validate(); err == nil {
		t.Fatal("expected source validation error")
	}
}
