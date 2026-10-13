package paymenthttp

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

	"github.com/google/uuid"
	"github.com/livingdolls/payment-service/internal/modules/order"
)

type Client struct {
	baseURL string
	token   string

	httpClient *http.Client
}

var _ order.PaymentCreator = (*Client)(nil)

func NewClient(baseURL string, token string) (*Client, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	isLocalHTTP := parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1")

	if parsed.Host == "" || (parsed.Scheme != "https" && !isLocalHTTP) || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, fmt.Errorf("invalid payment service URL")
	}

	if len(token) < 32 {
		return nil, fmt.Errorf("payment internal token is invalid")
	}

	return &Client{
		baseURL: baseURL,
		token:   token,

		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// CreatePayment implements [order.PaymentCreator].
func (c *Client) CreatePayment(ctx context.Context, checkoutOrder order.CheckoutOrder, idempotencyKey string) (*order.CheckoutPayment, error) {
	requestBody, err := json.Marshal(struct {
		OrderID  string `json:"order_id"`
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}{
		OrderID:  checkoutOrder.ID,
		Amount:   checkoutOrder.Amount,
		Currency: checkoutOrder.Currency,
	})

	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/internal/payments",
		bytes.NewReader(requestBody),
	)

	if err != nil {
		return nil, err
	}

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	req.Header.Set(
		"Authorization",
		"Bearer "+c.token,
	)

	req.Header.Set(
		"Idempotency-Key",
		idempotencyKey,
	)

	response, err := c.httpClient.Do(req)

	if err != nil {
		return nil, fmt.Errorf(
			"%w: %v",
			order.ErrPaymentServiceUnavailable,
			err,
		)
	}

	defer response.Body.Close()

	const maxResponseSize = 1 << 20

	body, err := io.ReadAll(
		io.LimitReader(
			response.Body,
			maxResponseSize+1,
		),
	)

	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", order.ErrPaymentServiceUnavailable, err)
	}

	if len(body) > maxResponseSize {
		return nil, fmt.Errorf("%w: response too large", order.ErrPaymentServiceUnavailable)
	}

	if response.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("%w: HTTP %d", order.ErrPaymentServiceUnavailable, response.StatusCode)
	}

	var payload struct {
		ID          string `json:"id"`
		AttemptID   string `json:"attempt_id"`
		ReferenceID string `json:"reference_id"`

		OrderID string `json:"order_id"`

		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`

		Status string `json:"status"`
	}

	if err := json.Unmarshal(
		body,
		&payload,
	); err != nil {
		return nil, fmt.Errorf("%w: invalid payment service response: %v", order.ErrPaymentServiceUnavailable, err)
	}

	// validasi response sebelum diterima oleh checkout service
	if _, err := uuid.Parse(payload.ID); err != nil {
		return nil, fmt.Errorf("%w: invalid payment intent id", order.ErrPaymentServiceUnavailable)
	}

	if _, err := uuid.Parse(payload.AttemptID); err != nil {
		return nil, fmt.Errorf("%w: invalid payment attempt id", order.ErrPaymentServiceUnavailable)
	}

	if payload.ReferenceID == "" || payload.OrderID != checkoutOrder.ID || payload.Amount != checkoutOrder.Amount || payload.Currency != checkoutOrder.Currency {
		return nil, order.ErrCheckoutPaymentMismatch
	}

	return &order.CheckoutPayment{
		PaymentIntentID: payload.ID,
		AttemptID:       payload.AttemptID,

		ReferenceID: payload.ReferenceID,
		OrderID:     payload.OrderID,

		Amount:   payload.Amount,
		Currency: payload.Currency,

		Status: payload.Status,
	}, nil
}
