package application

import (
	"testing"
	"time"
)

func TestReconciliationBackoff(t *testing.T) {
	tests := []struct {
		failures int
		expected time.Duration
	}{
		{1, 30 * time.Second},
		{2, 1 * time.Minute},
		{3, 2 * time.Minute},
		{4, 4 * time.Minute},
		{5, 8 * time.Minute},
		{6, 16 * time.Minute},
		{7, 30 * time.Minute},
		{8, 30 * time.Minute},
	}

	for _, tt := range tests {
		got := reconciliationBackoff(tt.failures)

		if got != tt.expected {
			t.Errorf(
				"failures=%d: expected %s, got %s",
				tt.failures,
				tt.expected,
				got,
			)
		}
	}
}
