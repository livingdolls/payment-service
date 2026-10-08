package xendit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/livingdolls/payment-service/internal/provider"
)

var _ provider.TransactionReferenceLookup = (*Client)(nil)

type transactionListResponse struct {
	HasMore bool `json:"has_more"`

	Data []struct {
		Type        string      `json:"type"`
		ReferenceID string      `json:"reference_id"`
		Currency    string      `json:"currency"`
		Amount      json.Number `json:"amount"`

		ProductData struct {
			PaymentRequestID string `json:"payment_request_id"`
		} `json:"data"`
	}
}

// FindPaymentRequestIDByReference implements [provider.TransactionReferenceLookup].
func (c *Client) FindPaymentRequestIDByReference(ctx context.Context, referenceID string, currency string, amount int64) (string, bool, error) {
	if referenceID == "" {
		return "", false, fmt.Errorf("reference id is required")
	}

	endpoint := c.baseURL + "/transactions"

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint,
		nil,
	)

	if err != nil {
		return "", false, err
	}

	params := url.Values{}
	params.Set("types", "PAYMENT")
	params.Set("reference_id", referenceID)
	params.Set("limit", "50")

	req.URL.RawQuery = params.Encode()
	req.SetBasicAuth(c.secretKey, "")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	const maxResponseSize = 1 << 20

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))

	if err != nil {
		return "", false, err
	}

	if len(body) > maxResponseSize {
		return "", false, fmt.Errorf("xendit transaction response too large")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false, mapHTTPError(
			resp.StatusCode,
			body,
		)
	}

	var response transactionListResponse

	if err := json.Unmarshal(body, &response); err != nil {
		return "", false, fmt.Errorf("decode transactions: %w", err)
	}

	// Jangan menyimpulkan hasil unik bila masih
	// ada halaman transaksi yang belum diperiksa.
	if response.HasMore {
		return "", false, fmt.Errorf("transaction lookup requires pagination")
	}

	candidates := make(map[string]struct{})
	incompleteMatch := false

	for _, tx := range response.Data {
		if tx.Type != "PAYMENT" || tx.ReferenceID != referenceID || tx.Currency != currency {
			continue
		}

		txAmount, err := parseWholeAmount(tx.Amount)
		if err != nil {
			return "", false, err
		}

		if txAmount != amount {
			continue
		}

		requestID := tx.ProductData.PaymentRequestID

		if requestID == "" {
			incompleteMatch = true
			continue
		}

		candidates[requestID] = struct{}{}
	}

	if incompleteMatch || len(candidates) > 1 {
		return "", false, fmt.Errorf("ambiguous or incomplete payment transaction match")
	}

	for requestID := range candidates {
		return requestID, true, nil
	}

	return "", false, nil
}
