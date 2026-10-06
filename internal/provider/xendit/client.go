package xendit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/livingdolls/payment-service/internal/provider"
)

const apiVersion = "2024-11-11"

type Client struct {
	secretKey  string
	baseURL    string
	httpClient *http.Client
}

var _ provider.PaymentProvider = (*Client)(nil)

func NewClient(secretKey string, baseURL string) *Client {
	return &Client{
		secretKey: secretKey,
		baseURL:   strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// CreatePayment implements [provider.PaymentProvider].
func (c *Client) CreatePayment(ctx context.Context, input provider.CreatePaymentInput) (*provider.CreatePaymentResult, error) {
	payload := createPaymentRequest{
		ReferenceID:       input.ReferenceID,
		Type:              "PAY",
		Country:           input.Country,
		Currency:          input.Currency,
		RequestAmount:     input.Amount,
		CaptureMethod:     input.CaptureMethod,
		ChannelCode:       input.ChannelCode,
		ChannelProperties: input.ChannelProperties,
		Description:       input.Description,
		Metadata:          input.Metadata,
	}

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf(
			"marshal xendit payment request: %w",
			err,
		)
	}

	url := c.baseURL + "/v3/payment_requests"

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(requestBody),
	)

	if err != nil {
		return nil, fmt.Errorf(
			"create xendit http request: %w",
			err,
		)
	}

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	req.Header.Set(
		"api-version",
		apiVersion,
	)

	req.SetBasicAuth(
		c.secretKey,
		"",
	)

	resp, err := c.httpClient.Do(req)

	if err != nil {
		return nil, &provider.Error{
			Kind: provider.ErrorKindUnknownOutcome,
			Err:  err,
		}
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, &provider.Error{
			Kind:       provider.ErrorKindUnknownOutcome,
			StatusCode: resp.StatusCode,
			Err:        err,
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapHTTPError(resp.StatusCode, body)
	}

	var response createPaymentResponse

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, &provider.Error{
			Kind:       provider.ErrorKindUnknownOutcome,
			StatusCode: resp.StatusCode,
			Err:        fmt.Errorf("decode xendit response: %w", err),
		}
	}

	result := &provider.CreatePaymentResult{
		PaymentRequestID: response.PaymentRequestID,
		PaymentID:        response.LatestPaymentID,
		Status:           mapPaymentStatus(response.Status),
		RawResponse:      body,
	}

	for _, action := range response.Actions {
		result.Actions = append(
			result.Actions,
			provider.Action{
				Type:       action.Type,
				Descriptor: action.Descriptor,
				Value:      action.Value,
			},
		)
	}

	return result, nil
}

func mapHTTPError(statusCode int, body []byte) error {
	var response errorResponse

	_ = json.Unmarshal(body, &response)

	kind := provider.ErrorKindRejected

	if statusCode == http.StatusRequestTimeout || statusCode == http.StatusTooManyRequests || statusCode >= 500 {
		kind = provider.ErrorKindUnknownOutcome
	}

	return &provider.Error{
		Kind:       kind,
		StatusCode: statusCode,
		Code:       response.ErrorCode,
		Message:    response.Message,
	}
}

func mapPaymentStatus(status string) provider.PaymentStatus {
	switch status {
	case "PENDING":
		return provider.PaymentStatusPending
	case "REQUIRES_ACTION":
		return provider.PaymentStatusRequiresAction
	case "AWAITING_CAPTURE":
		return provider.PaymentStatusAuthorized
	case "SUCCEEDED":
		return provider.PaymentStatusSucceeded
	case "FAILED":
		return provider.PaymentStatusFail
	case "EXPIRED":
		return provider.PaymentStatusExpired
	case "CANCELED", "VOIDED":
		return provider.PaymentStatusCanceled
	default:
		return provider.PaymentStatusUnknown
	}
}
