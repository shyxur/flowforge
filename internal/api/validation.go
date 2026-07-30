package api

import (
	"strings"

	"github.com/shyxur/distributed-task-queue/internal/domain"
)

func validateCreateTask(req createTaskRequest) map[string]any {
	details := make(map[string]any)
	if strings.TrimSpace(req.Queue) == "" || len(req.Queue) > 64 {
		details["queue"] = "must be between 1 and 64 characters"
	}
	if len(req.Payload) == 0 || string(req.Payload) == "null" {
		details["payload"] = "is required"
	}
	if req.Priority < 0 || req.Priority > 9 {
		details["priority"] = "must be between 0 and 9"
	}
	if req.MaxRetries != nil && (*req.MaxRetries < 0 || *req.MaxRetries > 99) {
		details["max_retries"] = "must be between 0 and 99"
	}
	if req.TimeoutSeconds < 0 || req.TimeoutSeconds > 86400 {
		details["timeout_seconds"] = "must be between 1 and 86400"
	}
	if req.VisibilityTimeoutSeconds < 0 || req.VisibilityTimeoutSeconds > 86400 {
		details["visibility_timeout_seconds"] = "must be between 1 and 86400"
	}
	if req.BackoffStrategy != "" && req.BackoffStrategy != "exponential" &&
		req.BackoffStrategy != "linear" && req.BackoffStrategy != "fixed" {
		details["backoff_strategy"] = "must be exponential, linear, or fixed"
	}
	return details
}

func validStatus(status domain.TaskStatus) bool {
	switch status {
	case domain.StatusPending, domain.StatusProcessing, domain.StatusCompleted,
		domain.StatusFailed, domain.StatusDeadLetter, domain.StatusCancelled:
		return true
	default:
		return false
	}
}
