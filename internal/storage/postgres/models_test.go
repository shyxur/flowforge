package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursorRoundTrip(t *testing.T) {
	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	id := uuid.New()
	gotTime, gotID, err := decodeCursor(encodeCursor(createdAt, id))
	if err != nil {
		t.Fatal(err)
	}
	if !gotTime.Equal(createdAt) || gotID != id {
		t.Fatalf("got %s/%s, want %s/%s", gotTime, gotID, createdAt, id)
	}
}
