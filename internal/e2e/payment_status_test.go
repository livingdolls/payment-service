//go:build integration

package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/livingdolls/payment-service/internal/provider"
)

func TestInternalPaymentStatusSelectsLatestAttemptForRequestedPayment(t *testing.T) {
	h := newHarness(t, "REQUIRES_ACTION")
	first := h.create()
	other := h.create()
	latestID := uuid.NewString()
	actions := []provider.Action{
		{Type: "PRESENT_TO_CUSTOMER", Descriptor: "QR_STRING", Value: "qr-for-first-payment"},
		{Type: "REDIRECT_CUSTOMER", Descriptor: "WEB_URL", Value: "https://checkout.example/pay"},
		{Type: "PRESENT_TO_CUSTOMER", Descriptor: "VIRTUAL_ACCOUNT_NUMBER", Value: "8808123456789"},
		{Type: "PRESENT_TO_CUSTOMER", Descriptor: "PAYMENT_CODE", Value: "123456789"},
		{Type: "PRESENT_TO_CUSTOMER", Descriptor: "UNSUPPORTED", Value: "private-provider-value"},
		{Type: "PRESENT_TO_CUSTOMER", Descriptor: "QR_STRING", Value: ""},
	}
	raw, err := json.Marshal(map[string]any{"actions": actions})
	if err != nil {
		t.Fatal(err)
	}
	const insertAttempt = `
		INSERT INTO payment_attempts (
			id, payment_intent_id, attempt_number, provider,
			provider_idempotency_key, status, provider_response
		) VALUES ($1, $2, $3, 'XENDIT', $4, 'REQUIRES_ACTION', $5)
	`
	if _, err := h.db.Exec(
		t.Context(),
		insertAttempt,
		latestID,
		first.ID,
		2,
		uuid.NewString(),
		raw,
	); err != nil {
		t.Fatal(err)
	}
	otherAttemptID := uuid.NewString()
	if _, err := h.db.Exec(
		t.Context(),
		insertAttempt,
		otherAttemptID,
		other.ID,
		9,
		uuid.NewString(),
		raw,
	); err != nil {
		t.Fatal(err)
	}

	var snapshot struct {
		PaymentIntentID string            `json:"payment_intent_id"`
		AttemptID       string            `json:"attempt_id"`
		ReferenceID     string            `json:"reference_id"`
		Amount          int64             `json:"amount"`
		AttemptStatus   string            `json:"attempt_status"`
		Actions         []provider.Action `json:"actions"`
	}
	headers := map[string]string{"Authorization": "Bearer " + internalToken}
	response := h.request(
		http.MethodGet,
		"/internal/payments/"+first.ID+"/status",
		nil,
		headers,
	)
	expectHTTP(t, response, http.StatusOK)
	if err := json.Unmarshal(response.Body, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.PaymentIntentID != first.ID || snapshot.AttemptID != latestID {
		t.Fatalf("wrong payment/attempt returned: payment=%s attempt=%s", snapshot.PaymentIntentID, snapshot.AttemptID)
	}
	if snapshot.ReferenceID != first.ReferenceID || snapshot.Amount != amount {
		t.Fatalf("wrong payment fields returned: %+v", snapshot)
	}
	if len(snapshot.Actions) != 4 || snapshot.Actions[0].Value != "qr-for-first-payment" {
		t.Fatalf("unexpected filtered actions: %+v", snapshot.Actions)
	}

	response = h.request(
		http.MethodGet,
		"/internal/payments/"+other.ID+"/status",
		nil,
		headers,
	)
	expectHTTP(t, response, http.StatusOK)
	if err := json.Unmarshal(response.Body, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.PaymentIntentID != other.ID || snapshot.AttemptID != otherAttemptID {
		t.Fatalf("status lookup leaked another payment: %+v", snapshot)
	}

	if _, err := h.db.Exec(t.Context(), "UPDATE payment_attempts SET status = 'CAPTURED' WHERE id = $1", latestID); err != nil {
		t.Fatal(err)
	}
	response = h.request(
		http.MethodGet,
		"/internal/payments/"+first.ID+"/status",
		nil,
		headers,
	)
	expectHTTP(t, response, http.StatusOK)
	if err := json.Unmarshal(response.Body, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.AttemptStatus != "CAPTURED" || snapshot.Actions == nil || len(snapshot.Actions) != 0 {
		t.Fatalf("completed attempt retained customer actions: %+v", snapshot)
	}
}

func TestInternalPaymentStatusRejectsInvalidMissingAndUnauthenticatedRequests(t *testing.T) {
	h := newHarness(t, "REQUIRES_ACTION")
	headers := map[string]string{"Authorization": "Bearer " + internalToken}
	invalid := h.request(
		http.MethodGet,
		"/internal/payments/invalid/status",
		nil,
		headers,
	)
	expectHTTP(t, invalid, http.StatusBadRequest)
	missing := h.request(
		http.MethodGet,
		"/internal/payments/"+uuid.NewString()+"/status",
		nil,
		headers,
	)
	expectHTTP(t, missing, http.StatusNotFound)
	created := h.create()
	unauthenticated := h.request(
		http.MethodGet,
		"/internal/payments/"+created.ID+"/status",
		nil,
		nil,
	)
	expectHTTP(t, unauthenticated, http.StatusUnauthorized)
}
