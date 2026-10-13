package orderhttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/livingdolls/payment-service/internal/modules/outbox"
)

func TestPublishAcknowledgesSuccessfulPOST(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent} {
		t.Run(fmt.Sprintf("HTTP_%d", status), func(t *testing.T) {
			t.Parallel()

			type receivedRequest struct {
				method  string
				path    string
				headers http.Header
				body    []byte
				err     error
			}
			requests := make(chan receivedRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				requests <- receivedRequest{
					method:  r.Method,
					path:    r.URL.RequestURI(),
					headers: r.Header.Clone(),
					body:    body,
					err:     err,
				}
				w.WriteHeader(status)
			}))
			t.Cleanup(server.Close)

			const token = "publisher-test-service-token"
			publisher, err := NewPublisher(server.URL, token, nil)
			if err != nil {
				t.Fatal(err)
			}
			if publisher.client.Timeout != 10*time.Second {
				t.Fatalf("default timeout = %v, want 10s", publisher.client.Timeout)
			}
			event := capturedEvent(t)
			if err := publisher.Publish(t.Context(), event); err != nil {
				t.Fatalf("publish successful response: %v", err)
			}

			var request receivedRequest
			select {
			case request = <-requests:
			default:
				t.Fatal("publish returned without delivering a request")
			}
			if request.err != nil {
				t.Fatal(request.err)
			}
			if request.method != http.MethodPost || request.path != "/internal/events/payments" {
				t.Fatalf("request = %s %s, want POST /internal/events/payments", request.method, request.path)
			}
			if !bytes.Equal(request.body, event.Payload) {
				t.Fatalf("request body = %s, want %s", request.body, event.Payload)
			}
			for name, want := range map[string]string{
				"Authorization":   "Bearer " + token,
				"Content-Type":    "application/json",
				"Idempotency-Key": event.ID,
				"X-Event-ID":      event.ID,
				"X-Event-Type":    event.EventType,
			} {
				if got := request.headers.Get(name); got != want {
					t.Errorf(
						"header %s = %q, want %q",
						name,
						got,
						want,
					)
				}
			}
		})
	}
}

func TestPublishRejectsRedirectWithoutFollowing(t *testing.T) {
	t.Parallel()

	for _, mode := range []struct {
		name     string
		supplied bool
	}{
		{name: "default_client"},
		{name: "supplied_client", supplied: true},
	} {
		for _, status := range []int{
			http.StatusMovedPermanently,
			http.StatusFound,
			http.StatusSeeOther,
			http.StatusTemporaryRedirect,
			http.StatusPermanentRedirect,
		} {
			t.Run(fmt.Sprintf("%s/HTTP_%d", mode.name, status), func(t *testing.T) {
				t.Parallel()

				var targetRequests atomic.Int32
				target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					targetRequests.Add(1)
					w.WriteHeader(http.StatusOK)
				}))
				t.Cleanup(target.Close)

				var originRequests atomic.Int32
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					originRequests.Add(1)
					http.Redirect(
						w,
						r,
						target.URL+"/acknowledge",
						status,
					)
				}))
				t.Cleanup(origin.Close)

				var policyCalls atomic.Int32
				var client *http.Client
				if mode.supplied {
					client = &http.Client{
						Timeout: 3 * time.Second,
						CheckRedirect: func(*http.Request, []*http.Request) error {
							policyCalls.Add(1)
							return nil
						},
					}
				}
				publisher, err := NewPublisher(origin.URL, "publisher-test-service-token", client)
				if err != nil {
					t.Fatal(err)
				}
				err = publisher.Publish(t.Context(), capturedEvent(t))
				if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", status)) {
					t.Fatalf("redirect must fail with its HTTP status, got %v", err)
				}
				if got := originRequests.Load(); got != 1 {
					t.Errorf("origin received %d requests, want 1", got)
				}
				if got := targetRequests.Load(); got != 0 {
					t.Errorf("redirect target received %d requests, want 0", got)
				}
				if got := policyCalls.Load(); got != 0 {
					t.Errorf("publisher called caller redirect policy %d times, want 0", got)
				}
			})
		}
	}
}

func TestNewPublisherPreservesCallerClient(t *testing.T) {
	t.Parallel()

	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(target.Close)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(
			w,
			r,
			target.URL,
			http.StatusFound,
		)
	}))
	t.Cleanup(origin.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{}
	t.Cleanup(transport.CloseIdleConnections)
	var policyCalls atomic.Int32
	client := &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			policyCalls.Add(1)
			return nil
		},
	}
	publisher, err := NewPublisher(origin.URL, "publisher-test-service-token", client)
	if err != nil {
		t.Fatal(err)
	}
	if publisher.client == client {
		t.Fatal("publisher reused the caller client")
	}
	for name, configured := range map[string]*http.Client{"caller": client, "publisher": publisher.client} {
		preservesTransport := configured.Transport == transport
		preservesCookieJar := configured.Jar == jar
		preservesTimeout := configured.Timeout == 3*time.Second
		if !preservesTransport || !preservesCookieJar || !preservesTimeout {
			t.Errorf("%s transport, cookie jar, or timeout changed", name)
		}
	}

	response, err := client.Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("caller response = HTTP %d, want redirected HTTP 204", response.StatusCode)
	}
	if got := policyCalls.Load(); got != 1 {
		t.Errorf("caller redirect policy called %d times, want 1", got)
	}
	if got := targetRequests.Load(); got != 1 {
		t.Errorf("caller redirect target received %d requests, want 1", got)
	}
}

func capturedEvent(t *testing.T) *outbox.Event {
	t.Helper()

	payload := outbox.PaymentCapturedPayload{
		EventID:         "7161b79f-f5ef-41f6-8cae-82e49b74cd97",
		EventType:       outbox.EventPaymentCaptured,
		PaymentIntentID: "9b842b5b-c3de-4efb-b09a-c1af531c99d9",
		ReferenceID:     "PAY-publisher-test",
		OrderID:         "ORDER-publisher-test",
		Amount:          50000,
		Currency:        "IDR",
		CapturedAmount:  50000,
		PaymentVersion:  3,
		OccurredAt: time.Date(
			2026,
			time.October,
			13,
			12,
			0,
			0,
			0,
			time.UTC,
		),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return &outbox.Event{
		ID:            payload.EventID,
		AggregateType: "PAYMENT_INTENT",
		AggregateID:   payload.PaymentIntentID,
		EventType:     payload.EventType,
		Payload:       body,
	}
}
