package config

import (
	"strings"
	"testing"
)

func TestMetricsConfigDefaultsAreValid(t *testing.T) {
	if err := Load().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMetricsConfigValidation(t *testing.T) {
	tests := []struct {
		name, env, value, message string
	}{
		{"capacity", "METRICS_BUFFER_CAPACITY", "0", "BUFFER_CAPACITY"},
		{"batch", "METRICS_BATCH_SIZE", "5000", "BATCH_SIZE"},
		{"flush", "METRICS_FLUSH_INTERVAL_SEC", "0", "FLUSH_INTERVAL"},
		{"timeout", "METRICS_WRITE_TIMEOUT_SEC", "0", "WRITE_TIMEOUT"},
		{"enabled", "METRICS_ENABLED", "sometimes", "ENABLED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.env, test.value)
			err := Load().Validate()
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("got %v, want error containing %q", err, test.message)
			}
		})
	}
}

func TestDisabledMetricsIgnoreProducerLimits(t *testing.T) {
	t.Setenv("METRICS_ENABLED", "false")
	t.Setenv("METRICS_BUFFER_CAPACITY", "0")
	if err := Load().Validate(); err != nil {
		t.Fatal(err)
	}
}
