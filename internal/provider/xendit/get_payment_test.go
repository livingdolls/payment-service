package xendit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/livingdolls/payment-service/internal/provider"
)

func TestGetPaymentRequest(t *testing.T) {
	const requestID = "pr-test-001"

	server := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}

			if r.URL.Path != "/v3/payment_requests/"+requestID {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			username, password, ok := r.BasicAuth()

			if !ok || username != "test-key" || password != "" {
				t.Error("invalid basic authentication")
			}

			if r.Header.Get("api-version") != apiVersion {
				t.Error("invalid API version")
			}

			w.Header().Set("Content-Type", "application/json")

			_, _ = w.Write([]byte(`{
				"payment_request_id": "pr-test-001",
				"reference_id": "PAY-001",
				"request_amount": 250000,
				"currency": "IDR",
				"status": "SUCCEEDED",
				"latest_payment_id": "py-test-001"
			}`))
		}),
	)
	defer server.Close()

	client := NewClient("test-key", server.URL)

	result, err := client.GetPaymentRequest(
		context.Background(),
		requestID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.Status != provider.PaymentStatusSucceeded {
		t.Fatalf("unexpected status: %s", result.Status)
	}

	if result.Amount != 250000 {
		t.Fatalf("unexpected amount: %d", result.Amount)
	}

	if result.ReferenceID != "PAY-001" {
		t.Fatalf(
			"unexpected reference: %s",
			result.ReferenceID,
		)
	}
}
