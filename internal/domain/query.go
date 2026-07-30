package domain

import (
	"time"

	"github.com/google/uuid"
)

type Principal struct {
	OrgID  uuid.UUID
	Scopes []string
}

type APIKey struct {
	OrgID     uuid.UUID
	Scopes    []string
	RevokedAt *time.Time
}

type TaskFilter struct {
	Queue  string
	Status TaskStatus
	Cursor string
	Limit  int
}

type TaskPage struct {
	Tasks      []*Task `json:"items"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

type QueueStats struct {
	Queue    string           `json:"queue"`
	ByStatus map[string]int64 `json:"by_status"`
	Total    int64            `json:"total"`
}

type Worker struct {
	ID              string    `json:"id"`
	OrgID           uuid.UUID `json:"org_id"`
	Queue           string    `json:"queue"`
	Status          string    `json:"status"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type QueueScope struct {
	OrgID uuid.UUID `json:"org_id"`
	Queue string    `json:"queue"`
}

func (s QueueScope) Key() string {
	return s.OrgID.String() + "/" + s.Queue
}
