//go:build integration

package e2e_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCaptureWebhookDeduplication(t *testing.T) {
	h := newHarness(t, "PENDING")
	created := h.create()
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusOK)
	event := h.event(created, "payment.capture")
	expectHTTP(t, h.receive(event), http.StatusOK)
	processed, err := h.webhooks.ProcessNext(t.Context())
	if !processed || err != nil {
		t.Fatalf("process first webhook: processed=%t error=%v", processed, err)
	}
	beforeIntent, beforeAttempt := h.state(created)
	duplicate := h.receive(event)
	expectHTTP(t, duplicate, http.StatusOK)
	var receipt struct {
		Duplicate bool `json:"duplicate"`
	}
	if err := json.Unmarshal(duplicate.Body, &receipt); err != nil || !receipt.Duplicate {
		t.Fatalf("duplicate webhook was not acknowledged: %s error=%v", duplicate.Body, err)
	}
	processed, err = h.webhooks.ProcessNext(t.Context())
	if processed || err != nil {
		t.Fatalf("duplicate webhook must not enqueue another event: processed=%t error=%v", processed, err)
	}
	afterIntent, afterAttempt := h.state(created)
	intentVersionChanged := beforeIntent.Version != afterIntent.Version
	attemptVersionChanged := beforeAttempt.Version != afterAttempt.Version
	amountChanged := afterIntent.CapturedAmount != amount
	if intentVersionChanged || attemptVersionChanged || amountChanged {
		t.Fatal("duplicate webhook changed state or captured amount")
	}
	var events int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM webhook_events").Scan(&events); err != nil || events != 1 {
		t.Fatalf("duplicate webhook event count=%d error=%v", events, err)
	}
}

func TestAuthorizationCaptureAndOlderEvents(t *testing.T) {
	h := newHarness(t, "REQUIRES_ACTION")
	created := h.create()
	expectHTTP(t, h.process(created, "CARDS", nil), http.StatusOK)
	authorization := h.event(created, "payment.authorization")
	expectHTTP(t, h.receive(authorization), http.StatusOK)
	if processed, err := h.webhooks.ProcessNext(t.Context()); !processed || err != nil {
		t.Fatalf("apply authorization: processed=%t error=%v", processed, err)
	}
	h.expectState(
		created,
		"AUTHORIZED",
		"AUTHORIZED",
		0,
	)
	capture := h.event(created, "payment.capture")
	expectHTTP(t, h.receive(capture), http.StatusOK)
	if processed, err := h.webhooks.ProcessNext(t.Context()); !processed || err != nil {
		t.Fatalf("apply capture: processed=%t error=%v", processed, err)
	}
	for _, kind := range []string{"payment.authorization", "payment.failure"} {
		event := h.event(created, kind)
		event["created"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
		event["data"].(map[string]any)["failure_code"] = "OLD_FAILURE"
		expectHTTP(t, h.receive(event), http.StatusOK)
		if processed, err := h.webhooks.ProcessNext(t.Context()); !processed || err != nil {
			t.Fatalf(
				"apply older %s: processed=%t error=%v",
				kind,
				processed,
				err,
			)
		}
		h.expectState(
			created,
			"CAPTURED",
			"CAPTURED",
			amount,
		)
	}
}

func TestWebhookAuthenticationAndBodyValidation(t *testing.T) {
	h := newHarness(t, "PENDING")
	created := h.create()
	event := h.event(created, "payment.capture")
	for _, token := range []string{"", "wrong-token"} {
		t.Run("invalid_token_"+token, func(t *testing.T) {
			expectHTTP(t, h.request(
				http.MethodPost,
				"/webhooks/xendit/payments",
				event,
				map[string]string{"x-callback-token": token},
			), http.StatusUnauthorized)
		})
	}
	headers := map[string]string{"x-callback-token": webhookToken}
	expectHTTP(t, h.request(
		http.MethodPost,
		"/webhooks/xendit/payments",
		[]byte(`{"event":`),
		headers,
	), http.StatusBadRequest)
	expectHTTP(t, h.request(
		http.MethodPost,
		"/webhooks/xendit/payments",
		[]byte(strings.Repeat("x", (1<<20)+1)),
		headers,
	), http.StatusBadRequest)
	expectHTTP(t, h.request(
		http.MethodPost,
		"/webhooks/xendit/payments",
		map[string]any{"event": "payment.capture", "data": map[string]any{}},
		headers,
	), http.StatusBadRequest)
	event["event"] = "unrelated.event"
	expectHTTP(t, h.receive(event), http.StatusOK)
	var count int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM webhook_events").Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid/unsupported webhook persisted count=%d error=%v", count, err)
	}
}

func TestWebhookIdentityAndAmountRejectWithoutMutation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "reference", mutate: func(data map[string]any) { data["reference_id"] = "PAY-OTHER" }},
		{name: "request_id", mutate: func(data map[string]any) { data["payment_request_id"] = "pr-other" }},
		{name: "payment_id", mutate: func(data map[string]any) { data["payment_id"] = "py-other" }},
		{name: "currency", mutate: func(data map[string]any) { data["currency"] = "USD" }},
		{name: "request_amount", mutate: func(data map[string]any) { data["request_amount"] = amount + 1 }},
		{name: "contradictory_status", mutate: func(data map[string]any) { data["status"] = "FAILED" }},
		{name: "partial_automatic_capture", mutate: func(data map[string]any) {
			data["captures"] = []map[string]any{{"capture_id": uuid.NewString(), "capture_amount": amount - 1}}
		}},
		{name: "negative_capture", mutate: func(data map[string]any) {
			data["captures"] = []map[string]any{{"capture_id": uuid.NewString(), "capture_amount": -1}}
		}},
		{name: "excessive_capture", mutate: func(data map[string]any) {
			data["captures"] = []map[string]any{{"capture_id": uuid.NewString(), "capture_amount": amount + 1}}
		}},
		{name: "zero_capture", mutate: func(data map[string]any) {
			data["captures"] = []map[string]any{{"capture_id": uuid.NewString(), "capture_amount": 0}}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t, "PENDING")
			created := h.create()
			expectHTTP(t, h.process(created, "QRIS", nil), http.StatusOK)
			beforeIntent, beforeAttempt := h.state(created)
			event := h.event(created, "payment.capture")
			test.mutate(event["data"].(map[string]any))
			expectHTTP(t, h.receive(event), http.StatusOK)
			processed, err := h.webhooks.ProcessNext(t.Context())
			if !processed || err == nil {
				t.Fatalf("invalid capture must fail processing: processed=%t error=%v", processed, err)
			}
			afterIntent, afterAttempt := h.state(created)
			if afterIntent.Version != beforeIntent.Version || afterAttempt.Version != beforeAttempt.Version {
				t.Fatal("invalid webhook persisted a partial state update")
			}
			h.expectState(
				created,
				"PROCESSING",
				"PENDING",
				0,
			)
			var status string
			row := h.db.QueryRow(t.Context(), "SELECT status FROM webhook_events")
			if err := row.Scan(&status); err != nil || status != "RECEIVED" {
				t.Fatalf("invalid webhook retry status=%q error=%v", status, err)
			}
		})
	}
}

func TestWebhookRetriesStopAtConfiguredMaximum(t *testing.T) {
	h := newHarness(t, "PENDING")
	created := h.create()
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusOK)
	event := h.event(created, "payment.capture")
	event["data"].(map[string]any)["request_amount"] = amount + 1
	expectHTTP(t, h.receive(event), http.StatusOK)
	for attempt := 1; attempt <= 5; attempt++ {
		processed, err := h.webhooks.ProcessNext(t.Context())
		if !processed || err == nil {
			t.Fatalf(
				"invalid event retry %d: processed=%t error=%v",
				attempt,
				processed,
				err,
			)
		}
	}
	var status string
	var attempts int
	row := h.db.QueryRow(t.Context(), "SELECT status,processing_attempts FROM webhook_events")
	if err := row.Scan(&status, &attempts); err != nil || status != "FAILED" || attempts != 5 {
		t.Fatalf(
			"retry exhaustion status=%q attempts=%d error=%v",
			status,
			attempts,
			err,
		)
	}
	if processed, err := h.webhooks.ProcessNext(t.Context()); processed || err != nil {
		t.Fatalf("exhausted event was claimed again: processed=%t error=%v", processed, err)
	}
}
