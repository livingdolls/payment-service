//go:build integration

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestConcurrentProcessingMakesOneProviderRequest(t *testing.T) {
	h := newHarness(t, "PENDING")
	created := h.create()
	entered := make(chan struct{})
	release := make(chan struct{})
	h.xendit.onCreate = func(map[string]any) {
		close(entered)
		select {
		case <-release:
		case <-t.Context().Done():
		}
	}
	// Closing release before the HTTP-server cleanup prevents a failed assertion
	// from leaving the deliberately blocked provider handler behind.
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	body, err := json.Marshal(map[string]any{
		"country": "ID", "channel_code": "QRIS", "metadata": map[string]any{"attempt_id": created.AttemptID},
	})
	if err != nil {
		t.Fatal(err)
	}
	completed := make(chan httpResult, 1)
	requestErrors := make(chan error, 1)
	go func() {
		request, err := http.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			h.api.URL+"/v1/payment-attempts/"+created.AttemptID+"/process",
			bytes.NewReader(body),
		)
		if err != nil {
			requestErrors <- err
			return
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := h.api.Client().Do(request)
		if err != nil {
			requestErrors <- err
			return
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			requestErrors <- err
			return
		}
		completed <- httpResult{Status: response.StatusCode, Body: data}
	}()
	select {
	case <-entered:
	case err := <-requestErrors:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("first processing request never reached the provider")
	}
	expectHTTP(t, h.process(created, "QRIS", nil), http.StatusConflict)
	h.expectState(
		created,
		"PROCESSING",
		"REQUESTING_PROVIDER",
		0,
	)
	close(release)
	select {
	case result := <-completed:
		expectHTTP(t, result, http.StatusOK)
	case err := <-requestErrors:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("provider processing request did not finish after release")
	}
	if h.xendit.createCalls.Load() != 1 {
		t.Fatal("concurrent processing made more than one provider create call")
	}
	h.finish(created)
}

func TestWebhookWinsAgainstOlderProviderResponse(t *testing.T) {
	cases := []struct {
		name       string
		createCode int
	}{
		{name: "older_requires_action_response"},
		{name: "older_provider_error", createCode: http.StatusServiceUnavailable},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t, "REQUIRES_ACTION")
			h.xendit.createCode = test.createCode
			created := h.create()
			callbackResult := make(chan error, 1)
			h.xendit.onCreate = func(map[string]any) {
				response := h.receive(h.event(created, "payment.capture"))
				if response.Status != http.StatusOK {
					callbackResult <- fmt.Errorf("capture receipt returned HTTP %d", response.Status)
					return
				}
				processed, err := h.webhooks.ProcessNext(t.Context())
				if !processed || err != nil {
					callbackResult <- fmt.Errorf("capture processing: processed=%t error=%v", processed, err)
					return
				}
				callbackResult <- nil
			}
			response := h.process(created, "QRIS", nil)
			expectHTTP(t, response, http.StatusOK)
			select {
			case err := <-callbackResult:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("provider race did not execute the early capture callback")
			}
			h.expectState(
				created,
				"CAPTURED",
				"CAPTURED",
				amount,
			)
			var result struct {
				Actions []json.RawMessage `json:"actions"`
			}
			if err := json.Unmarshal(response.Body, &result); err != nil || len(result.Actions) != 0 {
				t.Fatalf("older provider response exposed obsolete payment actions: %s error=%v", response.Body, err)
			}
			if h.xendit.createCalls.Load() != 1 {
				t.Fatal("webhook/provider race made a second provider payment")
			}
		})
	}
}
