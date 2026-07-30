package redis

import (
	"testing"

	"github.com/google/uuid"
)

func TestTenantScopedKeysAndMessage(t *testing.T) {
	orgID, taskID := uuid.New(), uuid.New()
	const queue = "email-send"
	want := "queueflow:v1:org:" + orgID.String() + ":queue:email-send:pending"
	if got := pendingKey(orgID, queue); got != want {
		t.Fatalf("pending key = %q, want %q", got, want)
	}
	gotTaskID, err := parseMessage(message(orgID, taskID), orgID)
	if err != nil || gotTaskID != taskID {
		t.Fatalf("message roundtrip = %s, %v", gotTaskID, err)
	}
	if _, err := parseMessage(message(orgID, taskID), uuid.New()); err == nil {
		t.Fatal("cross-tenant message should be rejected")
	}
	rateScope := "org:" + orgID.String() + ":queue:" + queue
	wantRate := "queueflow:v1:" + rateScope + ":ratelimit"
	if got := rateLimitKey(rateScope); got != wantRate {
		t.Fatalf("rate limit key = %q, want %q", got, wantRate)
	}
}
