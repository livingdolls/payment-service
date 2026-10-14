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
var _ order.PaymentFlowGateway = (*Client)(nil)

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

func (c *Client) GetStatus(ctx context.Context, paymentID string) (*order.PaymentSnapshot, error) {
	endpoint := c.baseURL + "/internal/payments/" + url.PathEscape(paymentID) + "/status"

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint,
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set(
		"Authorization",
		"Bearer "+c.token,
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", order.ErrPaymentServiceUnavailable, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"%w: status endpoint returned %d",
			order.ErrPaymentServiceUnavailable,
			resp.StatusCode,
		)
	}

	var snapshot order.PaymentSnapshot

	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("%w: invalid status response: %v", order.ErrPaymentServiceUnavailable, err)
	}

	return &snapshot, nil
}

func (c *Client) Process(ctx context.Context, attemptID string, channelCode string) error {
	endpoint := c.baseURL + "/internal/payment-attempts/" + url.PathEscape(attemptID) + "/process"

	payload, err := json.Marshal(map[string]any{
		"country":      "ID",
		"channel_code": channelCode,
	})

	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(payload),
	)

	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf(
			"%w: %v",
			order.ErrPaymentProcessingUncertain,
			err,
		)
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		return nil
	case http.StatusConflict:
		return nil
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return order.ErrPaymentProcessingRejected
	default:
		return fmt.Errorf("%w: HTTP %d", order.ErrPaymentProcessingUncertain, resp.StatusCode)
	}
}
