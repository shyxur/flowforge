//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/storage/postgres"
)

func TestMetricRepository(t *testing.T) {
	if os.Getenv("QUEUEFLOW_INTEGRATION") != "1" {
		t.Skip("run with make test-integration")
	}
	ctx := context.Background()
	dsn := os.Getenv("INTEGRATION_DB_DSN")
	if dsn == "" {
		t.Fatal("integration database address is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "TRUNCATE metric_events"); err != nil {
		t.Fatal(err)
	}
	storage, err := postgres.NewPostgresStorage(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()

	orgID := uuid.MustParse(devOrgID)
	now := time.Now().UTC().Truncate(time.Microsecond)
	events := make([]domain.MetricEvent, 1000)
	for index := range events {
		event, eventErr := domain.NewMetricEvent(domain.NewMetricEventInput{
			OrganizationID: orgID, Source: domain.MetricSourceQueueFlow,
			EventType: domain.MetricTaskStarted, ResourceType: domain.MetricResourceTask,
			ResourceID: uuid.NewString(), Queue: "high-volume", Status: "processing",
			OccurredAt:    now.Add(time.Duration(index) * time.Microsecond),
			TransitionKey: "attempt:1",
		})
		if eventErr != nil {
			t.Fatal(eventErr)
		}
		events[index] = event
	}
	if err := storage.AppendMetricEvents(ctx, events); err != nil {
		t.Fatal(err)
	}
	if err := storage.AppendMetricEvent(ctx, events[0]); err != nil {
		t.Fatalf("duplicate append must be idempotent: %v", err)
	}

	filter := domain.MetricEventFilter{
		From: now.Add(-time.Second), To: now.Add(time.Second),
		Source: domain.MetricSourceQueueFlow, EventType: domain.MetricTaskStarted, Limit: 101,
	}
	first, err := storage.ListMetricEvents(ctx, orgID, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 101 || first.NextCursor == "" {
		t.Fatalf("first page count=%d cursor=%q", len(first.Items), first.NextCursor)
	}
	for index := 1; index < len(first.Items); index++ {
		previous, current := first.Items[index-1], first.Items[index]
		if previous.OccurredAt.Before(current.OccurredAt) {
			t.Fatal("metric ordering is not deterministic descending")
		}
	}
	filter.Cursor = first.NextCursor
	second, err := storage.ListMetricEvents(ctx, orgID, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 101 || second.Items[0].ID == first.Items[len(first.Items)-1].ID {
		t.Fatal("cursor page overlaps or is incomplete")
	}
	otherTenant, err := storage.ListMetricEvents(ctx, uuid.New(), domain.MetricEventFilter{
		From: filter.From, To: filter.To, Limit: 1000,
	})
	if err != nil || len(otherTenant.Items) != 0 {
		t.Fatalf("cross-tenant metrics=%d err=%v", len(otherTenant.Items), err)
	}
	noMatches, err := storage.ListMetricEvents(ctx, orgID, domain.MetricEventFilter{
		From: filter.From, To: filter.To, Source: domain.MetricSourceEventForge,
		EventType: domain.MetricDeliveryStarted, Limit: 100,
	})
	if err != nil || len(noMatches.Items) != 0 {
		t.Fatalf("source filter metrics=%d err=%v", len(noMatches.Items), err)
	}

	if _, err := pool.Exec(ctx, "UPDATE metric_events SET status='changed' WHERE id=$1", events[0].ID); err == nil {
		t.Fatal("append-only update unexpectedly succeeded")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM metric_events WHERE id=$1", events[0].ID); err == nil {
		t.Fatal("append-only delete unexpectedly succeeded")
	}
}

func assertMetricTypes(
	t *testing.T,
	ctx context.Context,
	storage *postgres.PostgresStorage,
	orgID uuid.UUID,
	source domain.MetricSource,
	want ...domain.MetricEventType,
) {
	t.Helper()
	page, err := storage.ListMetricEvents(ctx, orgID, domain.MetricEventFilter{
		From: time.Now().UTC().Add(-24 * time.Hour), To: time.Now().UTC().Add(24 * time.Hour),
		Source: source, Limit: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	present := make(map[domain.MetricEventType]bool)
	for _, event := range page.Items {
		present[event.EventType] = true
	}
	for _, eventType := range want {
		if !present[eventType] {
			t.Fatalf("metric %s not found; present=%v", eventType, present)
		}
	}
}
