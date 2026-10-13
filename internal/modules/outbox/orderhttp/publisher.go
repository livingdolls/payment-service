package orderhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/livingdolls/payment-service/internal/modules/outbox"
)

type Publisher struct {
	endpoint string
	token    string
	client   *http.Client
}

var _ outbox.Publisher = (*Publisher)(nil)

func NewPublisher(baseURL string, token string, client *http.Client) (*Publisher, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")

	if token == "" {
		return nil, fmt.Errorf("order service token is required")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid order service URL: %w", err)
	}

	host := parsed.Hostname()

	localHost := host == "localhost" || host == "127.0.0.1" || host == "::1"
	localHTTP := parsed.Scheme == "http" && localHost

	unsupportedScheme := parsed.Scheme != "https" && !localHTTP
	containsCredentials := parsed.User != nil
	containsQueryOrFragment := parsed.RawQuery != "" || parsed.Fragment != ""
	if parsed.Host == "" || unsupportedScheme || containsCredentials || containsQueryOrFragment {
		return nil, fmt.Errorf("order service requires HTTPS or localhost HTTP")
	}

	publisherClient := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		publisherClient = *client
	}
	publisherClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &Publisher{
		endpoint: baseURL + "/internal/events/payments",
		token:    token,
		client:   &publisherClient,
	}, nil
}

// Publish implements [outbox.Publisher].
func (p *Publisher) Publish(ctx context.Context, event *outbox.Event) error {
	if event == nil {
		return fmt.Errorf(
			"%w: nil event",
			outbox.ErrInvalidPayload,
		)
	}

	if event.EventType != outbox.EventPaymentCaptured {
		return fmt.Errorf("%w: unsupported event type %s", outbox.ErrInvalidPayload, event.EventType)
	}

	var payload outbox.PaymentCapturedPayload

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("%w : %v", outbox.ErrInvalidPayload, err)
	}

	// Event yang dikirim harus sesuai dengan
	// metadata event yang tersimpan di database.
	identityMismatch := payload.EventID != event.ID || payload.EventType != event.EventType ||
		payload.PaymentIntentID != event.AggregateID
	invalidPayment := payload.OrderID == "" || payload.Amount <= 0 || payload.Currency == ""
	incompleteCapture := payload.CapturedAmount != payload.Amount
	if identityMismatch || invalidPayment || incompleteCapture {
		return fmt.Errorf(
			"%w: payment captured payload missmatch", outbox.ErrInvalidPayload,
		)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.endpoint,
		bytes.NewReader(event.Payload),
	)

	if err != nil {
		return fmt.Errorf("build order service request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Idempotency-Key", event.ID)
	req.Header.Set("X-Event-ID", event.ID)
	req.Header.Set("X-Event-Type", event.EventType)

	resp, err := p.client.Do(req)

	if err != nil {
		return fmt.Errorf("send event to order service: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("order service did not acknowledge event: HTTP %d", resp.StatusCode)
	}

	// Batasi pembacaan response karena kita tidak
	// membutuhkan data response untuk business logic.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	return nil
}
