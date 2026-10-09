package main

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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livingdolls/payment-service/internal/testkit"
)

type sandboxRunner struct {
	opts   options
	db     *pgxpool.Pool
	client *http.Client
}

type createResponse struct {
	ID          string `json:"id"`
	AttemptID   string `json:"attempt_id"`
	ReferenceID string `json:"reference_id"`
	Status      string `json:"status"`
}

type processResponse struct {
	Attempt struct {
		ProviderPaymentRequestID string `json:"provider_payment_request_id"`
	} `json:"attempt"`
	Actions []struct {
		Type       string `json:"type"`
		Descriptor string `json:"descriptor"`
		Value      string `json:"value"`
	} `json:"actions"`
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

type observedState struct {
	paymentStatus string
	attemptStatus string
	requestID     string
	webhookStatus string
	errorCode     string
	amount        int64
	captured      int64
	reconcileRuns int
}

func (r *sandboxRunner) ready(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		r.opts.apiURL+"/health/ready",
		nil,
	)
	if err != nil {
		return false
	}
	response, err := r.client.Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func (r *sandboxRunner) testChannel(ctx context.Context, profile testkit.ChannelProfile) testkit.ChannelResult {
	result := testkit.ChannelResult{
		ChannelCode: profile.ChannelCode,
		Status:      "FAIL",
		Amount:      profile.Amount,
		SourceURLs:  profile.SourceURLs,
	}
	orderID := fmt.Sprintf(
		"SANDBOX-%s-%d",
		strings.TrimSuffix(profile.ChannelCode, "_VIRTUAL_ACCOUNT"),
		time.Now().UnixNano(),
	)
	var created createResponse
	status, err := r.post(
		ctx,
		r.opts.apiURL+"/v1/payments",
		map[string]any{"order_id": orderID, "amount": profile.Amount, "currency": "IDR"},
		orderID,
		false,
		&created,
	)
	createAccepted := err == nil && status == http.StatusCreated
	validCreatedState := created.ID != "" && created.AttemptID != "" && created.Status == "CREATED"
	if !createAccepted || !validCreatedState {
		result.Reason = fmt.Sprintf("Create payment failed (HTTP %d); no successful full flow verified.", status)
		return result
	}
	result.PaymentID, result.AttemptID = created.ID, created.AttemptID
	initial, err := r.observe(ctx, created.ID, created.AttemptID)
	initialCreated := initial.paymentStatus == "CREATED" && initial.attemptStatus == "CREATED"
	if err != nil || !initialCreated || initial.amount != profile.Amount {
		result.Status = "BLOCKED"
		result.Reason = "Created IDs or initial state do not match the dedicated sandbox database; " +
			"process request was not sent."
		return result
	}
	var processed processResponse
	status, err = r.post(
		ctx,
		r.opts.apiURL+"/v1/payment-attempts/"+url.PathEscape(created.AttemptID)+"/process",
		map[string]any{
			"country":            "ID",
			"channel_code":       profile.ChannelCode,
			"channel_properties": profile.Properties,
			"description":        "Opt-in sandbox payment verification",
			"metadata":           map[string]any{"test_run": orderID},
		},
		"",
		false,
		&processed,
	)
	result.ProviderPaymentRequestID = processed.Attempt.ProviderPaymentRequestID
	if err != nil || (status != http.StatusOK && status != http.StatusAccepted) {
		state, stateErr := r.observe(ctx, created.ID, created.AttemptID)
		if stateErr == nil {
			populateState(&result, state)
			if isCredentialError(state.errorCode) {
				result.Status = "BLOCKED"
				result.Reason = "BLOCKED_API_CREDENTIALS: the provider rejected the sandbox API credential " +
					"or its permissions."
				return result
			}
			if isActivationError(state.errorCode) {
				result.Status = "BLOCKED"
				result.Reason = "Channel is not activated or merchant configuration does not permit " +
					"this sandbox request."
				return result
			}
		}
		result.Reason = fmt.Sprintf(
			"Process payment failed (HTTP %d); provider/application rejected the request.",
			status,
		)
		if err != nil {
			result.Reason = fmt.Sprintf(
				"Process HTTP outcome is unconfirmed (HTTP %d); "+
					"a later webhook/reconciliation outcome may still exist.",
				status,
			)
		}
		return result
	}
	if profile.CompletionMode == "simulate" && result.ProviderPaymentRequestID != "" {
		var simulation struct {
			Status    string `json:"status"`
			ErrorCode string `json:"error_code"`
		}
		status, err := r.post(
			ctx,
			"https://api.xendit.co/v3/payment_requests/"+url.PathEscape(result.ProviderPaymentRequestID)+"/simulate",
			map[string]int64{"amount": profile.Amount},
			"",
			true,
			&simulation,
		)
		if err != nil || status != http.StatusOK || simulation.Status != "PENDING" {
			unauthorized := status == http.StatusUnauthorized || status == http.StatusForbidden
			if unauthorized || isCredentialError(simulation.ErrorCode) {
				result.Status = "BLOCKED"
				result.Reason = "BLOCKED_API_CREDENTIALS: sandbox simulation credential or permission was rejected."
				return result
			}
			if isUnsupportedSimulation(simulation.ErrorCode) {
				result.Status = "BLOCKED"
				result.Reason = "Provider does not support the documented simulation for this channel/account; " +
					"no fabricated callback was sent."
				return result
			}
			result.Reason = fmt.Sprintf(
				"Documented sandbox simulation failed (HTTP %d); no fabricated callback was sent.",
				status,
			)
			return result
		}
	}
	if profile.CompletionMode == "actions" {
		fmt.Printf(
			"%s customer action: payment_request_id=%s. "+
				"Complete the TEST-mode action/3DS screen; waiting up to %s.\n",
			profile.ChannelCode,
			result.ProviderPaymentRequestID,
			r.opts.timeout,
		)
		for _, action := range processed.Actions {
			if action.Type == "REDIRECT_CUSTOMER" {
				fmt.Printf("  %s: %s\n", action.Descriptor, sanitizedActionURL(action.Value))
			}
		}
		if len(processed.Actions) == 0 {
			fmt.Println("  Check the Xendit TEST dashboard for this request; " +
				"push-notification channels may complete automatically in sandbox.")
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, r.opts.timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		state, err := r.observe(waitCtx, created.ID, created.AttemptID)
		if err != nil && waitCtx.Err() == nil {
			result.Reason = "Cannot verify final state in the sandbox database."
			return result
		}
		if err == nil {
			populateState(&result, state)
			if state.paymentStatus == "CAPTURED" && state.attemptStatus == "CAPTURED" {
				if state.amount != profile.Amount || state.captured != profile.Amount {
					result.Reason = "Final captured amount does not match the requested amount."
					return result
				}
				if state.webhookStatus == "PROCESSED" {
					result.Status, result.CompletionPath = "PASS", "webhook"
					result.Reason = "Captured with the exact amount and a processed capture webhook " +
						"matching all provider identifiers and reference."
					return result
				}
				if state.reconcileRuns > 0 {
					result.CompletionPath = "reconciled_without_processed_webhook"
				} else {
					result.CompletionPath = "provider_response_without_processed_webhook"
				}
			}
			if terminalFailure(state.paymentStatus) || terminalFailure(state.attemptStatus) {
				result.Reason = "Payment ended in FAILED, EXPIRED, or CANCELED."
				return result
			}
		}
		select {
		case <-waitCtx.Done():
			if result.CompletionPath != "" {
				result.Reason = "Captured outcome observed, but no matching capture webhook reached PROCESSED; " +
					"webhook delivery is unverified."
			} else if profile.CompletionMode == "actions" && ctx.Err() == nil {
				result.Status = "BLOCKED"
				result.Reason = "Customer sandbox action was not completed within the wait period; " +
					"payment remains unverified."
			} else {
				result.Reason = "Completion wait expired before captured intent, captured attempt, " +
					"and processed matching webhook were observed."
			}
			return result
		case <-ticker.C:
		}
	}
}

func (r *sandboxRunner) observe(ctx context.Context, paymentID, attemptID string) (observedState, error) {
	const query = `
		SELECT p.status, a.status, COALESCE(a.provider_payment_request_id, ''),
			p.amount, p.captured_amount, a.reconcile_attempts, COALESCE(a.error_code, ''),
			COALESCE((SELECT w.status FROM webhook_events w
				WHERE w.provider = 'XENDIT' AND w.event_type = 'payment.capture'
					AND w.reference_id = p.reference_id
					AND w.provider_payment_request_id = a.provider_payment_request_id
					AND w.provider_payment_id = a.provider_payment_id
				ORDER BY (w.status = 'PROCESSED') DESC, w.received_at DESC LIMIT 1), 'MISSING')
		FROM payment_intents p JOIN payment_attempts a ON a.payment_intent_id = p.id
		WHERE p.id = $1 AND a.id = $2`
	var state observedState
	err := r.db.QueryRow(
		ctx,
		query,
		paymentID,
		attemptID,
	).Scan(
		&state.paymentStatus,
		&state.attemptStatus,
		&state.requestID,
		&state.amount,
		&state.captured,
		&state.reconcileRuns,
		&state.errorCode,
		&state.webhookStatus,
	)
	return state, err
}

func populateState(result *testkit.ChannelResult, state observedState) {
	result.PaymentStatus, result.AttemptStatus = state.paymentStatus, state.attemptStatus
	result.ProviderPaymentRequestID, result.WebhookStatus = state.requestID, state.webhookStatus
	result.CapturedAmount = state.captured
}

func terminalFailure(status string) bool {
	return status == "FAILED" || status == "EXPIRED" || status == "CANCELED"
}

func isActivationError(code string) bool {
	switch code {
	case "CHANNEL_NOT_ACTIVATED",
		"CHANNEL_UNAVAILABLE",
		"ACCOUNT_NOT_ACTIVATED",
		"INVALID_MERCHANT_SETTINGS",
		"INVALID_MERCHANT_CREDENTIALS",
		"PAYMENT_METHOD_NOT_ACTIVATED":
		return true
	default:
		return false
	}
}

func isCredentialError(code string) bool {
	switch code {
	case "INVALID_API_KEY",
		"API_KEY_INVALID",
		"UNAUTHORIZED",
		"AUTHENTICATION_ERROR",
		"API_KEY_EXPIRED",
		"INSUFFICIENT_PERMISSIONS":
		return true
	default:
		return false
	}
}

func isUnsupportedSimulation(code string) bool {
	switch code {
	case "SIMULATION_NOT_SUPPORTED",
		"PAYMENT_SIMULATION_NOT_SUPPORTED",
		"CHANNEL_NOT_SUPPORTED",
		"PAYMENT_METHOD_NOT_SUPPORTED",
		"FEATURE_NOT_SUPPORTED",
		"UNSUPPORTED_CHANNEL":
		return true
	default:
		return false
	}
}

func (r *sandboxRunner) post(
	ctx context.Context,
	endpoint string,
	body any,
	idempotencyKey string,
	xendit bool,
	result any,
) (int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("encode request")
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(data),
	)
	if err != nil {
		return 0, fmt.Errorf("build request")
	}
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if xendit {
		request.Header.Set("api-version", "2024-11-11")
		request.SetBasicAuth(r.opts.secretKey, "")
	}
	response, err := r.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("HTTP request failed")
	}
	defer response.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(result); err != nil {
		return response.StatusCode, fmt.Errorf("unexpected HTTP response")
	}
	return response.StatusCode, nil
}

func sanitizedActionURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "Open the customer action from the Xendit TEST dashboard."
	}
	publicHTTPS := parsed.Scheme == "https" && parsed.Host != ""
	if !publicHTTPS || parsed.User != nil {
		return "Open the customer action from the Xendit TEST dashboard."
	}
	query := parsed.Query()
	redacted := false
	for key := range query {
		lower := strings.ToLower(key)
		keyOrToken := strings.Contains(lower, "key") || strings.Contains(lower, "token")
		secretOrAuth := strings.Contains(lower, "secret") || strings.Contains(lower, "auth")
		if keyOrToken || secretOrAuth {
			query.Del(key)
			redacted = true
		}
	}
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	if redacted {
		return parsed.String() + " [credential query omitted; use the full action in the TEST dashboard]"
	}
	return parsed.String()
}
