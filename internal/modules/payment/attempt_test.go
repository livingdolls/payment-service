package payment

import "testing"

func TestPaymentAttemptTransition(
	t *testing.T,
) {
	attempt := &PaymentAttempt{
		Status: AttemptStatusCreated,
	}

	err := attempt.TransitionTo(
		AttemptStatusRequestingProvider,
	)

	if err != nil {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}

	if attempt.Status !=
		AttemptStatusRequestingProvider {

		t.Fatalf(
			"expected REQUESTING_PROVIDER, got %s",
			attempt.Status,
		)
	}
}

func TestPaymentAttemptTransition_Invalid(
	t *testing.T,
) {
	attempt := &PaymentAttempt{
		Status: AttemptStatusCreated,
	}

	err := attempt.TransitionTo(
		AttemptStatusCaptured,
	)

	if err == nil {
		t.Fatal(
			"expected transition error",
		)
	}
}
