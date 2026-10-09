package application

import "time"

const (
	reconcileMaxFailures    = 8
	reconcileInitialBackoff = 30 * time.Second
	reconcileMaxBackoff     = 30 * time.Minute
)

func reconciliationBackoff(consecutiveFailures int) time.Duration {
	delay := reconcileInitialBackoff

	for i := 1; i < consecutiveFailures; i++ {
		if delay >= reconcileMaxBackoff/2 {
			return reconcileMaxBackoff
		}

		delay *= 2
	}

	return delay
}
