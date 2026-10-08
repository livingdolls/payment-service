package application

import (
	"testing"

	"github.com/livingdolls/payment-service/internal/modules/payment"
)

func TestIsStaleAttemptState_CapturedDoesNotRegress(
	t *testing.T,
) {
	stale, err :=
		isStaleAttemptState(
			payment.AttemptStatusCaptured,
			payment.AttemptStatusRequiresAction,
		)

	if err != nil {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}

	if !stale {
		t.Fatal(
			"expected stale provider result",
		)
	}
}

func TestIsStaleAttemptState_AuthorizedCanAdvanceToCaptured(
	t *testing.T,
) {
	stale, err :=
		isStaleAttemptState(
			payment.AttemptStatusAuthorized,
			payment.AttemptStatusCaptured,
		)

	if err != nil {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}

	if stale {
		t.Fatal(
			"expected incoming capture to be applicable",
		)
	}
}

func TestIsStaleAttemptState_AuthorizedIgnoresRequiresAction(
	t *testing.T,
) {
	stale, err :=
		isStaleAttemptState(
			payment.AttemptStatusAuthorized,
			payment.AttemptStatusRequiresAction,
		)

	if err != nil {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}

	if !stale {
		t.Fatal(
			"expected REQUIRES_ACTION to be stale",
		)
	}
}
