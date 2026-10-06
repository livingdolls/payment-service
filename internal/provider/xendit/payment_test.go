package xendit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/livingdolls/payment-service/internal/provider"
)

func TestCreatePayment(
	t *testing.T,
) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				if r.URL.Path !=
					"/v3/payment_requests" {

					t.Fatalf(
						"unexpected path: %s",
						r.URL.Path,
					)
				}

				if r.Method != http.MethodPost {
					t.Fatalf(
						"unexpected method: %s",
						r.Method,
					)
				}

				w.Header().Set(
					"Content-Type",
					"application/json",
				)

				w.WriteHeader(
					http.StatusCreated,
				)

				_, _ = w.Write([]byte(`{
					"payment_request_id":
						"pr-test-001",

					"reference_id":
						"PAY-001",

					"status":
						"REQUIRES_ACTION",

					"actions": [
						{
							"type":
								"REDIRECT_CUSTOMER",

							"descriptor":
								"WEB_URL",

							"value":
								"https://example.com/pay"
						}
					]
				}`))
			},
		),
	)

	defer server.Close()

	client := NewClient(
		"test-secret-key",
		server.URL,
	)

	result, err := client.CreatePayment(
		context.Background(),
		provider.CreatePaymentInput{
			ReferenceID: "PAY-001",

			Amount: 250000,

			Currency: "IDR",

			Country: "ID",

			CaptureMethod: "AUTOMATIC",

			ChannelCode: "TEST_CHANNEL",
		},
	)

	if err != nil {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}

	if result.PaymentRequestID !=
		"pr-test-001" {

		t.Fatalf(
			"unexpected payment request id: %s",
			result.PaymentRequestID,
		)
	}

	if result.Status !=
		provider.PaymentStatusRequiresAction {

		t.Fatalf(
			"unexpected status: %s",
			result.Status,
		)
	}

	if len(result.Actions) != 1 {
		t.Fatalf(
			"expected 1 action",
		)
	}
}
