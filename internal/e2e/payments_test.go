//go:build integration

package e2e_test

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/livingdolls/payment-service/internal/testkit"
)

func TestAllRunnableChannelPayments(t *testing.T) {
	profiles, err := testkit.LoadChannels("")
	if err != nil {
		t.Fatal(err)
	}
	report := testkit.Report{Mode: "local", StartedAt: time.Now().UTC()}
	t.Cleanup(func() {
		report.Finalize()
		directory := os.Getenv("PAYMENT_TEST_REPORT_DIR")
		if directory == "" {
			directory = t.TempDir()
		}
		jsonPath, markdownPath, err := testkit.WriteReport(directory, report)
		if err != nil {
			t.Errorf("write local payment report: %v", err)
			return
		}
		t.Logf("Local channel report: %s; %s", jsonPath, markdownPath)
	})
	eligibleCount := 0
	for _, profile := range profiles {
		row := testkit.ChannelResult{ChannelCode: profile.ChannelCode, Amount: profile.Amount, SourceURLs: profile.SourceURLs}
		if !profile.Eligible {
			row.Status = "BLOCKED"
			row.Reason = profile.Reason
			report.Results = append(report.Results, row)
			continue
		}
		eligibleCount++
		passed := t.Run(profile.ChannelCode, func(t *testing.T) {
			status, intentStatus, attemptStatus := "PENDING", "PROCESSING", "PENDING"
			if profile.CompletionMode == "actions" {
				status, intentStatus, attemptStatus = "REQUIRES_ACTION", "REQUIRES_ACTION", "REQUIRES_ACTION"
			}
			if profile.ChannelCode == "CARDS" {
				status, intentStatus, attemptStatus = "SUCCEEDED", "CAPTURED", "CAPTURED"
			}
			h := newHarness(t, status)
			created := h.create()
			row.PaymentID, row.AttemptID = created.ID, created.AttemptID
			expectHTTP(t, h.process(created, profile.ChannelCode, profile.Properties), http.StatusOK)
			captured := int64(0)
			if status == "SUCCEEDED" {
				captured = amount
			}
			h.expectState(
				created,
				intentStatus,
				attemptStatus,
				captured,
			)
			input := h.xendit.input
			matchesProfile := input["channel_code"] == profile.ChannelCode &&
				input["request_amount"] == float64(profile.Amount)
			matchesLocale := input["country"] == "ID" && input["currency"] == "IDR"
			matchesFlow := input["type"] == "PAY" && input["capture_method"] == "AUTOMATIC"
			if !matchesProfile || !matchesLocale || !matchesFlow {
				t.Fatalf("provider request does not match the IDR channel profile")
			}
			if len(profile.Properties) > 0 && !reflect.DeepEqual(input["channel_properties"], profile.Properties) {
				t.Fatal("channel properties were not forwarded to the real Xendit HTTP client")
			}
			if h.xendit.createCalls.Load() != 1 {
				t.Fatal("payment processing must make exactly one provider create request")
			}
			h.finish(created)
			intent, attempt := h.state(created)
			row.PaymentStatus, row.AttemptStatus = string(intent.Status), string(attempt.Status)
			row.CapturedAmount, row.WebhookStatus = intent.CapturedAmount, "PROCESSED"
			row.ProviderPaymentRequestID = *attempt.ProviderPaymentRequestID
			row.CompletionPath = "mock " + status + " -> HTTP capture webhook -> PostgreSQL"
		})
		row.Status = "PASS"
		if !passed {
			row.Status = "FAIL"
			row.Reason = "Local HTTP/provider/database integration failed; inspect the Go test output."
		}
		report.Results = append(report.Results, row)
	}
	if eligibleCount != 22 {
		t.Fatalf("runnable channel fixture changed: found %d profiles, want 22", eligibleCount)
	}
}

func TestCreatePaymentValidationRollsBack(t *testing.T) {
	h := newHarness(t, "PENDING")
	cases := []struct {
		name     string
		orderID  string
		amount   int64
		currency string
		key      string
		status   int
	}{
		{name: "missing_order", orderID: "", amount: amount, currency: "IDR", key: "missing-order", status: 422},
		{
			name: "long_order", orderID: strings.Repeat("x", 101), amount: amount,
			currency: "IDR", key: "long-order", status: 422,
		},
		{name: "zero_amount", orderID: "zero", amount: 0, currency: "IDR", key: "zero-amount", status: 422},
		{name: "negative_amount", orderID: "negative", amount: -1, currency: "IDR", key: "negative-amount", status: 422},
		{name: "invalid_currency", orderID: "currency", amount: amount, currency: "ID", key: "invalid-currency", status: 422},
		{
			name: "unsupported_currency", orderID: "currency", amount: amount,
			currency: "USD", key: "unsupported-currency", status: 422,
		},
		{name: "missing_key", orderID: "key", amount: amount, currency: "IDR", key: "", status: 400},
		{name: "long_key", orderID: "key", amount: amount, currency: "IDR", key: strings.Repeat("x", 256), status: 400},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := h.request(
				http.MethodPost,
				"/v1/payments",
				map[string]any{
					"order_id": test.orderID, "amount": test.amount, "currency": test.currency,
				},
				map[string]string{"Idempotency-Key": test.key},
			)
			expectHTTP(t, response, test.status)
			for _, table := range []string{"payment_intents", "payment_attempts", "idempotency_keys"} {
				var count int
				if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
					t.Fatalf(
						"invalid request left %d records in %s: %v",
						count,
						table,
						err,
					)
				}
			}
		})
	}
	expectHTTP(t, h.request(
		http.MethodPost,
		"/v1/payments",
		[]byte(`{"amount":`),
		nil,
	), http.StatusBadRequest)
	if h.xendit.createCalls.Load() != 0 {
		t.Fatal("invalid creation must never contact the provider")
	}
}

func TestCreatePaymentReplayAndConflict(t *testing.T) {
	h := newHarness(t, "PENDING")
	key := uuid.NewString()
	body := map[string]any{"order_id": " replay-order ", "amount": amount, "currency": " idr "}
	headers := map[string]string{"Idempotency-Key": key}
	first := h.request(
		http.MethodPost,
		"/v1/payments",
		body,
		headers,
	)
	expectHTTP(t, first, http.StatusCreated)
	body["order_id"], body["currency"] = "replay-order", "IDR"
	replay := h.request(
		http.MethodPost,
		"/v1/payments",
		body,
		headers,
	)
	expectHTTP(t, replay, http.StatusCreated)
	var firstJSON, replayJSON map[string]any
	if err := json.Unmarshal(first.Body, &firstJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(replay.Body, &replayJSON); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstJSON, replayJSON) {
		t.Fatal("idempotent replay must return the original payment and attempt")
	}
	body["amount"] = amount + 1
	expectHTTP(t, h.request(
		http.MethodPost,
		"/v1/payments",
		body,
		headers,
	), http.StatusConflict)
	for _, table := range []string{"payment_intents", "payment_attempts", "idempotency_keys"} {
		var count int
		if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 1 {
			t.Fatalf(
				"replay/conflict changed %s count=%d: %v",
				table,
				count,
				err,
			)
		}
	}
}

func TestProcessingErrorsAndRepeatedRequest(t *testing.T) {
	h := newHarness(t, "PENDING")
	created := h.create()
	expectHTTP(t, h.process(created, "", nil), http.StatusUnprocessableEntity)
	h.expectState(
		created,
		"CREATED",
		"CREATED",
		0,
	)
	missing := created
	missing.AttemptID = uuid.NewString()
	expectHTTP(t, h.process(missing, "QRIS", nil), http.StatusNotFound)
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusOK)
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusConflict)
	if h.xendit.createCalls.Load() != 1 {
		t.Fatal("repeated processing created a second provider payment")
	}
}

func TestReadinessChecksPostgres(t *testing.T) {
	h := newHarness(t, "PENDING")
	expectHTTP(t, h.request(
		http.MethodGet,
		"/health/ready",
		nil,
		nil,
	), http.StatusOK)
}
