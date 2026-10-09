//go:build integration

package e2e_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	paymentpostgres "github.com/livingdolls/payment-service/internal/modules/payment/postgres"
)

func TestProviderRejectsPayment(t *testing.T) {
	h := newHarness(t, "PENDING")
	h.xendit.createCode = http.StatusUnprocessableEntity
	created := h.create()
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusUnprocessableEntity)
	h.expectState(
		created,
		"FAILED",
		"FAILED",
		0,
	)
	_, attempt := h.state(created)
	if attempt.ErrorCode == nil || *attempt.ErrorCode != "INTEGRATION_PROVIDER_ERROR" {
		t.Fatal("provider rejection code was not persisted")
	}
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusConflict)
	if h.xendit.createCalls.Load() != 1 {
		t.Fatal("rejected attempt was resubmitted to the provider")
	}
}

func TestUnknownOutcomeRecoversByReference(t *testing.T) {
	h := newHarness(t, "SUCCEEDED")
	h.xendit.createCode = http.StatusServiceUnavailable
	created := h.create()
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusAccepted)
	h.expectState(
		created,
		"PROCESSING",
		"UNKNOWN",
		0,
	)
	_, before := h.state(created)
	if before.ProviderPaymentRequestID != nil {
		t.Fatal("unknown provider result should not invent a payment request ID")
	}
	h.due(created.AttemptID)
	processed, err := h.reconcile.ProcessNext(t.Context())
	if !processed || err != nil {
		t.Fatalf("reconcile unknown outcome: processed=%t error=%v", processed, err)
	}
	h.expectState(
		created,
		"CAPTURED",
		"CAPTURED",
		amount,
	)
	if h.xendit.lookupCalls.Load() != 1 || h.xendit.getCalls.Load() != 1 || h.xendit.createCalls.Load() != 1 {
		t.Fatal("reference recovery must look up and read the existing payment without creating a second payment")
	}
	_, after := h.state(created)
	if after.ProviderPaymentRequestID == nil || *after.ProviderPaymentRequestID != "pr-"+created.AttemptID {
		t.Fatal("reference recovery did not persist the provider request ID")
	}
}

func TestLostWebhookRecoversByProviderRequestID(t *testing.T) {
	h := newHarness(t, "PENDING")
	created := h.create()
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusOK)
	h.xendit.setStatus("SUCCEEDED")
	h.due(created.AttemptID)
	if processed, err := h.reconcile.ProcessNext(t.Context()); !processed || err != nil {
		t.Fatalf("recover missing webhook: processed=%t error=%v", processed, err)
	}
	h.expectState(
		created,
		"CAPTURED",
		"CAPTURED",
		amount,
	)
	if h.xendit.lookupCalls.Load() != 0 || h.xendit.getCalls.Load() != 1 {
		t.Fatal("known provider ID recovery unexpectedly performed a reference lookup")
	}
	var events int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM webhook_events").Scan(&events); err != nil || events != 0 {
		t.Fatalf("lost webhook scenario unexpectedly had %d events: %v", events, err)
	}
}

func TestReconciliationRejectsMismatchedProviderSnapshots(t *testing.T) {
	cases := []struct {
		name       string
		field      string
		value      any
		errorMatch string
	}{
		{name: "reference", field: "reference_id", value: "PAY-OTHER", errorMatch: "reference ID mismatch"},
		{name: "amount", field: "request_amount", value: amount + 1, errorMatch: "amount mismatch"},
		{name: "currency", field: "currency", value: "USD", errorMatch: "currency mismatch"},
		{
			name: "request_id", field: "payment_request_id",
			value: "pr-00000000-0000-0000-0000-000000000001", errorMatch: "unexpected payment request id",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t, "PENDING")
			created := h.create()
			expectHTTP(t, h.process(created, "QRIS", nil), http.StatusOK)
			beforeIntent, beforeAttempt := h.state(created)
			h.xendit.setStatus("SUCCEEDED")
			h.xendit.setGetOverrides(map[string]any{test.field: test.value})
			h.due(created.AttemptID)
			processed, err := h.reconcile.ProcessNext(t.Context())
			if !processed || err == nil || !strings.Contains(err.Error(), test.errorMatch) {
				t.Fatalf("mismatched provider snapshot: processed=%t error=%v", processed, err)
			}
			h.expectState(
				created,
				"PROCESSING",
				"PENDING",
				0,
			)
			afterIntent, afterAttempt := h.state(created)
			versionChanged := beforeIntent.Version != afterIntent.Version || beforeAttempt.Version != afterAttempt.Version
			requestIDChanged := *beforeAttempt.ProviderPaymentRequestID != *afterAttempt.ProviderPaymentRequestID
			paymentIDChanged := *beforeAttempt.ProviderPaymentID != *afterAttempt.ProviderPaymentID
			if versionChanged || requestIDChanged || paymentIDChanged {
				t.Fatal("mismatched snapshot changed payment state, version, or provider identity")
			}
			var failures int
			var message string
			var next time.Time
			const query = `
				SELECT reconcile_failures,last_reconcile_error,next_reconcile_at
				FROM payment_attempts WHERE id=$1
			`
			if err := h.db.QueryRow(t.Context(), query, created.AttemptID).Scan(&failures, &message, &next); err != nil {
				t.Fatal(err)
			}
			if failures != 1 || !strings.Contains(message, test.errorMatch) || !next.After(time.Now()) {
				t.Fatalf(
					"snapshot mismatch did not record a retry: failures=%d message=%q next=%v",
					failures,
					message,
					next,
				)
			}
		})
	}
}

func TestReconciliationCooldownAndForcedEligibility(t *testing.T) {
	cases := []struct {
		name     string
		fixture  string
		eligible bool
	}{
		{name: "recent_not_due", fixture: `updated_at=NOW(),next_reconcile_at=NULL`, eligible: false},
		{
			name: "old_future_cooldown", eligible: false,
			fixture: `updated_at=NOW()-INTERVAL '3 minutes',next_reconcile_at=NOW()+INTERVAL '1 hour'`,
		},
		{
			name: "old_needs_review", eligible: false,
			fixture: `updated_at=NOW()-INTERVAL '3 minutes',reconcile_state='NEEDS_REVIEW',next_reconcile_at=NULL`,
		},
		{
			name: "forced_recent_active", eligible: true,
			fixture: `updated_at=NOW(),reconcile_force=TRUE,next_reconcile_at=NOW()`,
		},
		{
			name: "forced_needs_review", eligible: false,
			fixture: `updated_at=NOW(),reconcile_force=TRUE,reconcile_state='NEEDS_REVIEW',next_reconcile_at=NOW()`,
		},
		{
			name: "forced_created", eligible: false,
			fixture: `updated_at=NOW(),reconcile_force=TRUE,status='CREATED',next_reconcile_at=NOW()`,
		},
		{
			name: "forced_captured", eligible: false,
			fixture: `updated_at=NOW(),reconcile_force=TRUE,status='CAPTURED',next_reconcile_at=NOW()`,
		},
		{
			name: "forced_future_cooldown", eligible: false,
			fixture: `updated_at=NOW(),reconcile_force=TRUE,next_reconcile_at=NOW()+INTERVAL '1 hour'`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t, "PENDING")
			created := h.create()
			expectHTTP(t, h.process(created, "QRIS", nil), http.StatusOK)
			query := "UPDATE payment_attempts SET " + test.fixture + " WHERE id=$1"
			if _, err := h.db.Exec(t.Context(), query, created.AttemptID); err != nil {
				t.Fatal(err)
			}
			processed, err := h.reconcile.ProcessNext(t.Context())
			if err != nil || processed != test.eligible {
				t.Fatalf(
					"eligibility processed=%t want=%t error=%v",
					processed,
					test.eligible,
					err,
				)
			}
			if test.eligible {
				if processed, err := h.reconcile.ProcessNext(t.Context()); processed || err != nil {
					t.Fatalf("cooldown allowed an immediate second claim: processed=%t error=%v", processed, err)
				}
				var next time.Time
				const nextReconcileQuery = "SELECT next_reconcile_at FROM payment_attempts WHERE id=$1"
				row := h.db.QueryRow(t.Context(), nextReconcileQuery, created.AttemptID)
				if err := row.Scan(&next); err != nil || !next.After(time.Now()) {
					t.Fatalf("successful pending reconciliation did not set a future cooldown: %v", err)
				}
			} else if h.xendit.getCalls.Load() != 0 || h.xendit.lookupCalls.Load() != 0 {
				t.Fatal("ineligible attempt contacted the provider")
			}
		})
	}
}

func TestReviewEscalationAndAdminRequeueAudit(t *testing.T) {
	h := newHarness(t, "PENDING")
	created := h.create()
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusOK)
	h.xendit.getCode = http.StatusServiceUnavailable
	for failure := 1; failure <= 8; failure++ {
		h.due(created.AttemptID)
		processed, err := h.reconcile.ProcessNext(t.Context())
		if !processed || err == nil {
			t.Fatalf(
				"reconciliation failure %d: processed=%t error=%v",
				failure,
				processed,
				err,
			)
		}
		var count int
		var state string
		var next *time.Time
		const query = `
			SELECT reconcile_failures,reconcile_state,next_reconcile_at
			FROM payment_attempts WHERE id=$1
		`
		if err := h.db.QueryRow(t.Context(), query, created.AttemptID).Scan(&count, &state, &next); err != nil {
			t.Fatal(err)
		}
		if count != failure {
			t.Fatalf("consecutive failures=%d want=%d", count, failure)
		}
		if failure < 8 {
			minimumDelay := min(30*time.Second*time.Duration(1<<(failure-1)), 30*time.Minute) - 2*time.Second
			if state != "ACTIVE" || next == nil || time.Until(*next) < minimumDelay {
				t.Fatalf("failed reconciliation did not apply exponential backoff: state=%s next=%v", state, next)
			}
		} else if state != "NEEDS_REVIEW" || next != nil {
			t.Fatalf("exhaustion did not require operator review: state=%s next=%v", state, next)
		}
	}
	if processed, err := h.reconcile.ProcessNext(t.Context()); processed || err != nil {
		t.Fatalf("needs-review attempt was automatically claimed: processed=%t error=%v", processed, err)
	}
	path := "/admin/payment-attempts/" + created.AttemptID + "/requeue-reconciliation"
	reason := "Provider recovered; verified pending payment before retry."
	for _, header := range []string{"", "Bearer invalid"} {
		expectHTTP(t, h.request(
			http.MethodGet,
			"/admin/reconciliation-reviews",
			nil,
			map[string]string{"Authorization": header},
		), http.StatusUnauthorized)
		expectHTTP(t, h.request(
			http.MethodPost,
			path,
			map[string]any{"reason": reason},
			map[string]string{"Authorization": header},
		), http.StatusUnauthorized)
	}
	headers := map[string]string{"Authorization": "Bearer " + adminToken, "X-Admin-Actor": "spoofed-actor"}
	expectHTTP(t, h.request(
		http.MethodGet,
		"/admin/reconciliation-reviews",
		nil,
		headers,
	), http.StatusOK)
	expectHTTP(t, h.request(
		http.MethodPost,
		path,
		map[string]any{"reason": "short"},
		headers,
	), http.StatusBadRequest)
	const updateTimestamp = "UPDATE payment_attempts SET updated_at=NOW() WHERE id=$1"
	if _, err := h.db.Exec(t.Context(), updateTimestamp, created.AttemptID); err != nil {
		t.Fatal(err)
	}
	expectHTTP(t, h.request(
		http.MethodPost,
		path,
		map[string]any{"reason": reason},
		headers,
	), http.StatusAccepted)
	var actor, auditReason, previous, nextState string
	var previousFailures int
	const auditQuery = `
		SELECT actor,reason,previous_state,new_state,previous_failures
		FROM payment_reconciliation_actions WHERE payment_attempt_id=$1
	`
	if err := h.db.QueryRow(t.Context(), auditQuery, created.AttemptID).Scan(
		&actor,
		&auditReason,
		&previous,
		&nextState,
		&previousFailures,
	); err != nil {
		t.Fatal(err)
	}
	actorMatches := actor == adminActor
	reasonMatches := auditReason == reason
	statesMatch := previous == "NEEDS_REVIEW" && nextState == "ACTIVE"
	failuresMatch := previousFailures == 8
	if !actorMatches || !reasonMatches || !statesMatch || !failuresMatch {
		t.Fatalf(
			"operator audit mismatch: actor=%q reason=%q states=%s/%s failures=%d",
			actor,
			auditReason,
			previous,
			nextState,
			previousFailures,
		)
	}
	expectHTTP(t, h.request(
		http.MethodPost,
		path,
		map[string]any{"reason": reason},
		headers,
	), http.StatusConflict)
	h.xendit.getCode = 0
	h.xendit.setStatus("SUCCEEDED")
	if processed, err := h.reconcile.ProcessNext(t.Context()); !processed || err != nil {
		t.Fatalf("forced requeue should bypass minimum age: processed=%t error=%v", processed, err)
	}
	h.expectState(
		created,
		"CAPTURED",
		"CAPTURED",
		amount,
	)
}

func TestReconciliationStaleGenerationCannotOverwriteRequeue(t *testing.T) {
	h := newHarness(t, "PENDING")
	created := h.create()
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusOK)
	h.due(created.AttemptID)
	repository := paymentpostgres.NewReconciliationRepository(h.db)
	stale, err := repository.ClaimNext(t.Context(), 2*time.Minute, 5*time.Minute)
	if err != nil || stale == nil {
		t.Fatalf("claim fixture: job=%v error=%v", stale, err)
	}
	const newerGeneration = `
		UPDATE payment_attempts
		SET reconcile_generation=reconcile_generation+1,
			reconcile_force=TRUE,reconcile_failures=3,next_reconcile_at=NOW()
		WHERE id=$1
	`
	if _, err := h.db.Exec(t.Context(), newerGeneration, created.AttemptID); err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkSuccess(t.Context(), stale, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkFailure(
		t.Context(),
		stale,
		"old worker failure",
		time.Hour,
		8,
	); err != nil {
		t.Fatal(err)
	}
	var failures int
	var forced bool
	var next time.Time
	const query = `
		SELECT reconcile_failures,reconcile_force,next_reconcile_at
		FROM payment_attempts WHERE id=$1
	`
	err = h.db.QueryRow(t.Context(), query, created.AttemptID).Scan(&failures, &forced, &next)
	failuresMatch := failures == 3
	deadlinePreserved := !next.After(time.Now().Add(time.Second))
	if err != nil || !failuresMatch || !forced || !deadlinePreserved {
		t.Fatalf(
			"stale worker changed a newer generation: failures=%d force=%t next=%v error=%v",
			failures,
			forced,
			next,
			err,
		)
	}
}
