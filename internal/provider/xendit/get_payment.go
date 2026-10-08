package xendit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"

	"github.com/livingdolls/payment-service/internal/provider"
)

var _ provider.PaymentStatusReader = (*Client)(nil)

type getPaymentRequestResponse struct {
	PaymentRequestID string `json:"payment_request_id"`
	ReferenceID      string `json:"reference_id"`

	RequestAmount json.Number `json:"request_amount"`
	Currency      string      `json:"currency"`

	LatestPaymentID *string `json:"latest_payment_id"`
	Status          string  `json:"status"`

	Actions []provider.Action `json:"actions"`
}

func (c *Client) GetPaymentRequest(ctx context.Context, paymentRequestID string) (*provider.PaymentRequestSnapshot, error) {
	if paymentRequestID == "" {
		return nil, fmt.Errorf("payment request id is required")
	}

	endpoint := c.baseURL + "/v3/payment_requests/" + url.PathEscape(paymentRequestID)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint,
		nil,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"build xendit GET request: %w",
			err,
		)
	}

	req.SetBasicAuth(c.secretKey, "")
	req.Header.Set("api-version", apiVersion)

	resp, err := c.httpClient.Do(req)

	if err != nil {
		return nil, &provider.Error{
			Kind: provider.ErrorKindUnknownOutcome,
			Err:  err,
		}
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if err != nil {
		return nil, fmt.Errorf("read xendit response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapHTTPError(resp.StatusCode, body)
	}

	var response getPaymentRequestResponse

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode payment request: %w", err)
	}

	if response.PaymentRequestID != paymentRequestID {
		return nil, fmt.Errorf("xendit returned unexpected payment request id")
	}

	if err != nil {
		return nil, err
	}

	amount, err := parseWholeAmount(response.RequestAmount)

	if err != nil {
		return nil, err
	}

	status := mapPaymentStatus(response.Status)

	if status == provider.PaymentStatusUnknown {
		return nil, fmt.Errorf(
			"unrecognized xendit status: %s",
			response.Status,
		)
	}

	return &provider.PaymentRequestSnapshot{
		PaymentRequestID: response.PaymentRequestID,
		ReferenceID:      response.ReferenceID,

		Amount:   amount,
		Currency: response.Currency,

		PaymentID: response.LatestPaymentID,
		Status:    status,

		Actions:     response.Actions,
		RawResponse: body,
	}, nil
}

func parseWholeAmount(value json.Number) (int64, error) {
	number, ok := new(big.Rat).SetString(value.String())

	if !ok || !number.IsInt() {
		return 0, fmt.Errorf(
			"provider amount is not a whole number: %s",
			value.String(),
		)
	}

	if !number.Num().IsInt64() {
		return 0, fmt.Errorf(
			"provider amount exceeds int64 range",
		)
	}

	return number.Num().Int64(), nil
}
