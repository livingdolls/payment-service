//go:build integration

package e2e_test

import (
	"net/http"
	"testing"
)

func TestDatabaseFailureIsVisibleToReadinessAndWorkers(t *testing.T) {
	h := newHarness(t, "PENDING")
	h.db.Close()
	expectHTTP(t, h.request(
		http.MethodGet,
		"/health/ready",
		nil,
		nil,
	), http.StatusServiceUnavailable)
	if processed, err := h.webhooks.ProcessNext(t.Context()); processed || err == nil {
		t.Fatalf("webhook claim hid a database error: processed=%t error=%v", processed, err)
	}
	if processed, err := h.reconcile.ProcessNext(t.Context()); processed || err == nil {
		t.Fatalf("reconciliation claim hid a database error: processed=%t error=%v", processed, err)
	}
}

func TestReconciliationBookkeepingFailureIsVisible(t *testing.T) {
	h := newHarness(t, "PENDING")
	created := h.create()
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusOK)
	h.xendit.setStatus("SUCCEEDED")
	h.due(created.AttemptID)
	// This isolated-schema trigger fails only the bookkeeping write that follows
	// an already committed provider state update, not the claim or capture.
	const failBookkeeping = `
		CREATE FUNCTION fail_reconciliation_bookkeeping()
		RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF OLD.status='CAPTURED' AND NEW.status='CAPTURED' THEN
				RAISE EXCEPTION 'synthetic reconciliation bookkeeping failure';
			END IF;
			RETURN NEW;
		END
	$$`
	if _, err := h.db.Exec(t.Context(), failBookkeeping); err != nil {
		t.Fatal(err)
	}
	const failBookkeepingTrigger = `
		CREATE TRIGGER fail_reconciliation_bookkeeping
		BEFORE UPDATE ON payment_attempts
		FOR EACH ROW EXECUTE FUNCTION fail_reconciliation_bookkeeping()
	`
	if _, err := h.db.Exec(t.Context(), failBookkeepingTrigger); err != nil {
		t.Fatal(err)
	}
	processed, err := h.reconcile.ProcessNext(t.Context())
	if !processed || err == nil {
		t.Fatalf("reconciliation silently accepted a bookkeeping database failure: processed=%t error=%v", processed, err)
	}
	h.expectState(
		created,
		"CAPTURED",
		"CAPTURED",
		amount,
	)
}
