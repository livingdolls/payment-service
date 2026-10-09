package application

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/livingdolls/payment-service/internal/modules/payment"
	xenditprovider "github.com/livingdolls/payment-service/internal/provider/xendit"
)

func TestCaptureAmounts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		requestAmount int64
		captures      []int64
		amount        int64
		wantError     bool
	}{
		{name: "request amount fallback", requestAmount: 50000, amount: 50000},
		{name: "capture total", captures: []int64{20000, 30000}, amount: 50000},
		{name: "partial automatic capture", captures: []int64{30000}, wantError: true},
		{name: "zero fallback", wantError: true},
		{name: "negative fallback", requestAmount: -1, wantError: true},
		{name: "oversized fallback", requestAmount: 50001, wantError: true},
		{name: "zero capture", captures: []int64{0}, wantError: true},
		{name: "negative capture", captures: []int64{-1}, wantError: true},
		{name: "oversized total", captures: []int64{30000, 30000}, wantError: true},
		{name: "overflow", captures: []int64{math.MaxInt64, math.MaxInt64}, wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			attempt := &payment.PaymentAttempt{Status: payment.AttemptStatusPending}
			intent := &payment.PaymentIntent{
				Status: payment.StatusProcessing, Amount: 50000, CaptureMethod: payment.CaptureMethodAutomatic,
			}
			event := xenditprovider.PaymentWebhook{
				Event: "payment.capture",
				Data: xenditprovider.PaymentWebhookData{
					RequestAmount: tc.requestAmount,
					Captures:      []xenditprovider.PaymentWebhookCapture{},
				},
			}
			for _, amount := range tc.captures {
				event.Data.Captures = append(event.Data.Captures, xenditprovider.PaymentWebhookCapture{
					CaptureAmount: amount,
				})
			}

			err := applyXenditPaymentEvent(attempt, intent, event)
			if tc.wantError {
				if err == nil {
					t.Fatal("invalid capture was accepted")
				}
				if intent.Status != payment.StatusProcessing || attempt.Status != payment.AttemptStatusPending {
					t.Fatal("invalid capture changed payment state")
				}
				return
			}
			if err != nil {
				t.Fatalf("capture: %v", err)
			}
			if intent.CapturedAmount != tc.amount || intent.Status != payment.StatusCaptured {
				t.Fatalf("unexpected captured payment: %+v", intent)
			}
			if attempt.Status != payment.AttemptStatusCaptured {
				t.Fatalf("attempt status = %s", attempt.Status)
			}
		})
	}
}

func TestCapturedPaymentDoesNotRegress(t *testing.T) {
	t.Parallel()
	for _, eventType := range []string{"payment.capture", "payment.authorization", "payment.failure"} {
		t.Run(eventType, func(t *testing.T) {
			t.Parallel()
			attempt := &payment.PaymentAttempt{Status: payment.AttemptStatusCaptured}
			intent := &payment.PaymentIntent{
				Status: payment.StatusCaptured, Amount: 50000, CapturedAmount: 50000,
			}
			event := xenditprovider.PaymentWebhook{
				Event: eventType,
				Data: xenditprovider.PaymentWebhookData{
					RequestAmount: 50000,
					Captures: []xenditprovider.PaymentWebhookCapture{
						{CaptureAmount: 50000},
					},
				},
			}
			if err := applyXenditPaymentEvent(attempt, intent, event); err != nil {
				t.Fatalf("late event: %v", err)
			}
			if intent.CapturedAmount != 50000 || intent.Status != payment.StatusCaptured {
				t.Fatalf("late event regressed payment: %+v", intent)
			}
			if attempt.Status != payment.AttemptStatusCaptured {
				t.Fatal("late event regressed attempt")
			}
		})
	}
}

func TestWebhookIdentityValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*xenditprovider.PaymentWebhookData)
	}{
		{name: "reference", change: func(d *xenditprovider.PaymentWebhookData) { d.ReferenceID = "wrong" }},
		{name: "currency", change: func(d *xenditprovider.PaymentWebhookData) { d.Currency = "USD" }},
		{name: "amount", change: func(d *xenditprovider.PaymentWebhookData) { d.RequestAmount = 50001 }},
		{name: "request id", change: func(d *xenditprovider.PaymentWebhookData) { d.PaymentRequestID = "wrong" }},
		{name: "payment id", change: func(d *xenditprovider.PaymentWebhookData) { d.PaymentID = "wrong" }},
		{name: "contradictory status", change: func(d *xenditprovider.PaymentWebhookData) { d.Status = "FAILED" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			requestID, paymentID := "pr-test", "py-test"
			attempt := &payment.PaymentAttempt{
				ProviderPaymentRequestID: &requestID, ProviderPaymentID: &paymentID,
			}
			intent := &payment.PaymentIntent{ReferenceID: "PAY-test", Currency: "IDR", Amount: 50000}
			event := xenditprovider.PaymentWebhook{Event: "payment.capture", Data: xenditprovider.PaymentWebhookData{
				PaymentRequestID: requestID, PaymentID: paymentID, ReferenceID: intent.ReferenceID,
				Currency: intent.Currency, RequestAmount: intent.Amount,
			}}
			tc.change(&event.Data)
			if err := validateXenditPaymentEvent(attempt, intent, event); err == nil {
				t.Fatal("mismatched event was accepted")
			}
		})
	}
}

func TestWebhookClaimErrorIsReturned(t *testing.T) {
	t.Parallel()
	want := errors.New("database unavailable")
	processor := NewWebhookProcessor(failingWebhookDB{err: want}, nil)
	processed, err := processor.ProcessNext(t.Context())
	if processed || !errors.Is(err, want) {
		t.Fatalf("ProcessNext = (%v, %v), want (false, database error)", processed, err)
	}
}

type failingWebhookDB struct{ err error }

func (db failingWebhookDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, db.err
}

func (db failingWebhookDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, db.err
}

func (db failingWebhookDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return failingWebhookRow{err: db.err}
}

type failingWebhookRow struct{ err error }

func (row failingWebhookRow) Scan(...any) error { return row.err }
