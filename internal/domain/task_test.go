package domain

import (
	"testing"
	"time"
)

func TestTerminalStates(t *testing.T) {
	for _, status := range []TaskStatus{StatusCompleted, StatusFailed, StatusDeadLetter, StatusCancelled} {
		if !(&Task{Status: status}).IsTerminal() {
			t.Errorf("%s should be terminal", status)
		}
	}
	for _, status := range []TaskStatus{StatusPending, StatusProcessing} {
		if (&Task{Status: status}).IsTerminal() {
			t.Errorf("%s should not be terminal", status)
		}
	}
}

func TestMarkProcessingTransition(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{Status: StatusPending, VisibilityTimeout: 30 * time.Second, MaxAttempts: 1}
	task.MarkProcessing("worker-1", now)
	if task.Status != StatusProcessing || task.Attempts != 1 || task.LockedBy != "worker-1" {
		t.Fatalf("unexpected transition: %+v", task)
	}
	if !task.VisibleAt.Equal(now.Add(30*time.Second)) || !task.IsExhausted() {
		t.Fatal("visibility or exhaustion state is incorrect")
	}
}
