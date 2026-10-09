package application

import (
	"testing"

	"github.com/livingdolls/payment-service/internal/modules/payment"
)

func TestIsTerminalAttempt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   payment.AttemptStatus
		terminal bool
	}{
		{
			name:   "created",
			status: payment.AttemptStatusCreated,
		},
		{
			name:   "requesting provider",
			status: payment.AttemptStatusRequestingProvider,
		},
		{
			name:   "pending",
			status: payment.AttemptStatusPending,
		},
		{
			name:   "requires action",
			status: payment.AttemptStatusRequiresAction,
		},
		{
			name:   "authorized",
			status: payment.AttemptStatusAuthorized,
		},
		{
			name:     "captured",
			status:   payment.AttemptStatusCaptured,
			terminal: true,
		},
		{
			name:     "failed",
			status:   payment.AttemptStatusFailed,
			terminal: true,
		},
		{
			name:     "expired",
			status:   payment.AttemptStatusExpired,
			terminal: true,
		},
		{
			name:     "canceled",
			status:   payment.AttemptStatusCanceled,
			terminal: true,
		},
		{
			name:   "unknown",
			status: payment.AttemptStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isTerminalAttempt(tt.status); got != tt.terminal {
				t.Errorf("isTerminalAttempt(%q) = %t, want %t", tt.status, got, tt.terminal)
			}
		})
	}
}
