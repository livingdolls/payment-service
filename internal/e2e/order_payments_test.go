//go:build integration

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livingdolls/payment-service/internal/modules/order"
	orderhandler "github.com/livingdolls/payment-service/internal/modules/order/http"
	orderpostgres "github.com/livingdolls/payment-service/internal/modules/order/postgres"
	"github.com/livingdolls/payment-service/internal/modules/outbox"
	orderpublisher "github.com/livingdolls/payment-service/internal/modules/outbox/orderhttp"
	outboxpostgres "github.com/livingdolls/payment-service/internal/modules/outbox/postgres"
	"github.com/livingdolls/payment-service/internal/worker"
)

const orderDeliveryToken = "integration-order-service-token-at-least-32-characters"

const (
	orderConsumerAvailable int32 = iota
	orderConsumerUnavailable
	orderConsumerDropsAcknowledgement
)

func TestOrderPaymentCaptureAndDuplicateDelivery(t *testing.T) {
	h := newHarness(t, "PENDING")
	orders := newOrderDeliveryHarness(t)
	orderID, created := orders.createBoundPayment(h)
	expectHTTP(t, h.process(created, "QRIS", map[string]any{}), http.StatusOK)
	capture := h.event(created, "payment.capture")
	expectHTTP(t, h.receive(capture), http.StatusOK)
	processed, err := h.webhooks.ProcessNext(t.Context())
	if err != nil || !processed {
		t.Fatalf("process capture webhook: processed=%t error=%v", processed, err)
	}
	h.expectState(
		created,
		"CAPTURED",
		"CAPTURED",
		amount,
	)
	orders.expectPending(orderID, created.ID)
	event := orderOutboxEvent(t, h.db, created.ID)
	if event.Status != outbox.StatusPending || event.AttemptCount != 0 {
		t.Fatalf("capture did not enqueue a pending event: status=%s attempts=%d", event.Status, event.AttemptCount)
	}

	delivery := orders.deliveryWorker(h.db)
	processed, err = delivery.ProcessNext(t.Context())
	if err != nil || !processed {
		t.Fatalf("publish capture to order service: processed=%t error=%v", processed, err)
	}
	paidAt := orders.expectPaid(orderID, created.ID, event.ID)
	published := orderOutboxEvent(t, h.db, created.ID)
	expectPublishedOrderEvent(t, published, 1)
	intent, attempt := h.state(created)

	response := orders.receive(event)
	expectHTTP(t, response, http.StatusOK)
	var acknowledgement struct {
		Received  bool   `json:"received"`
		Duplicate bool   `json:"duplicate"`
		OrderID   string `json:"order_id"`
	}
	if err := json.Unmarshal(response.Body, &acknowledgement); err != nil {
		t.Fatal(err)
	}
	if !acknowledgement.Received || !acknowledgement.Duplicate || acknowledgement.OrderID != orderID {
		t.Fatalf("duplicate delivery was not acknowledged: %s", response.Body)
	}
	if after := orders.expectPaid(orderID, created.ID, event.ID); !after.Equal(paidAt) {
		t.Fatal("duplicate delivery changed the paid timestamp")
	}

	response = h.receive(capture)
	expectHTTP(t, response, http.StatusOK)
	processed, err = h.webhooks.ProcessNext(t.Context())
	if err != nil || processed {
		t.Fatalf("duplicate webhook created new work: processed=%t error=%v", processed, err)
	}
	afterIntent, afterAttempt := h.state(created)
	if afterIntent.Version != intent.Version || afterAttempt.Version != attempt.Version {
		t.Fatal("duplicate webhook changed payment versions")
	}
	h.expectState(
		created,
		"CAPTURED",
		"CAPTURED",
		amount,
	)
	afterPublished := orderOutboxEvent(t, h.db, created.ID)
	expectPublishedOrderEvent(t, afterPublished, 1)
	if !afterPublished.PublishedAt.Equal(*published.PublishedAt) {
		t.Fatal("duplicate webhook changed the published timestamp")
	}
	processed, err = delivery.ProcessNext(t.Context())
	if err != nil || processed {
		t.Fatalf("published event was claimed again: processed=%t error=%v", processed, err)
	}
}

func TestOrderPaymentDeliveryRetries(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		mode        int32
		paidOnFirst bool
	}{
		{name: "consumer_unavailable", mode: orderConsumerUnavailable},
		{name: "acknowledgement_lost", mode: orderConsumerDropsAcknowledgement, paidOnFirst: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			h := newHarness(t, "PENDING")
			orders := newOrderDeliveryHarness(t)
			orderID, created := orders.createBoundPayment(h)
			expectHTTP(t, h.process(created, "QRIS", map[string]any{}), http.StatusOK)
			h.finish(created)
			event := orderOutboxEvent(t, h.db, created.ID)
			orders.mode.Store(scenario.mode)
			delivery := orders.deliveryWorker(h.db)
			processed, err := delivery.ProcessNext(t.Context())
			if err != nil || !processed {
				t.Fatalf("record failed delivery: processed=%t error=%v", processed, err)
			}
			retry := orderOutboxEvent(t, h.db, created.ID)
			pendingRetry := retry.Status == outbox.StatusPending && retry.AttemptCount == 1
			errorRecorded := retry.LastError != nil && *retry.LastError != ""
			claimReleased := retry.ClaimToken == nil && retry.PublishedAt == nil
			if !pendingRetry || !errorRecorded || !claimReleased {
				t.Fatalf("failed delivery was not saved for retry: %+v", retry)
			}
			var databaseNow time.Time
			if err := h.db.QueryRow(t.Context(), "SELECT NOW()").Scan(&databaseNow); err != nil {
				t.Fatal(err)
			}
			if !retry.AvailableAt.After(databaseNow) {
				t.Fatal("failed delivery did not schedule a future retry")
			}
			processed, err = delivery.ProcessNext(t.Context())
			if err != nil || processed {
				t.Fatalf("retry ignored its cooldown: processed=%t error=%v", processed, err)
			}

			var firstPaidAt time.Time
			if scenario.paidOnFirst {
				firstPaidAt = orders.expectPaid(orderID, created.ID, event.ID)
				if orders.lostAcknowledgements.Load() != 1 {
					t.Fatal("test did not drop the acknowledgement after committing the order")
				}
			} else {
				orders.expectPending(orderID, created.ID)
			}
			orders.mode.Store(orderConsumerAvailable)
			const makeDue = `
				UPDATE outbox_events
				SET available_at=NOW()-INTERVAL '1 second'
				WHERE id=$1 AND status='PENDING'
			`
			result, err := h.db.Exec(t.Context(), makeDue, event.ID)
			if err != nil || result.RowsAffected() != 1 {
				t.Fatalf("make isolated retry fixture due: affected=%d error=%v", result.RowsAffected(), err)
			}
			processed, err = delivery.ProcessNext(t.Context())
			if err != nil || !processed {
				t.Fatalf("retry delivery to recovered consumer: processed=%t error=%v", processed, err)
			}
			paidAt := orders.expectPaid(orderID, created.ID, event.ID)
			if scenario.paidOnFirst && !paidAt.Equal(firstPaidAt) {
				t.Fatal("retry after a lost acknowledgement changed the paid timestamp")
			}
			expectPublishedOrderEvent(t, orderOutboxEvent(t, h.db, created.ID), 2)
			h.expectState(
				created,
				"CAPTURED",
				"CAPTURED",
				amount,
			)
			if got := orders.requests.Load(); got != 2 {
				t.Fatalf("HTTP delivery attempts=%d, want 2", got)
			}
		})
	}
}

type orderDeliveryHarness struct {
	t                    *testing.T
	db                   *pgxpool.Pool
	server               *httptest.Server
	client               *http.Client
	mode                 atomic.Int32
	requests             atomic.Int32
	lostAcknowledgements atomic.Int32
}

func newOrderDeliveryHarness(t *testing.T) *orderDeliveryHarness {
	t.Helper()
	h := &orderDeliveryHarness{t: t, db: isolatedOrderDatabase(t)}
	consumer := order.NewConsumer(orderpostgres.NewRepository(h.db))
	handler := orderhandler.NewHandler(consumer, orderDeliveryToken)
	router := gin.New()
	router.Use(gin.Recovery())
	router.POST("/internal/events/payments", handler.ConsumePaymentEvent)
	h.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.requests.Add(1)
		switch h.mode.Load() {
		case orderConsumerUnavailable:
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		case orderConsumerDropsAcknowledgement:
			response := httptest.NewRecorder()
			router.ServeHTTP(response, r)
			if response.Code != http.StatusOK {
				w.WriteHeader(response.Code)
				_, _ = w.Write(response.Body.Bytes())
				return
			}
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("test HTTP server does not support dropping the acknowledgement")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("drop test acknowledgement: %v", err)
				return
			}
			h.lostAcknowledgements.Add(1)
			_ = connection.Close()
			return
		}
		router.ServeHTTP(w, r)
	}))
	t.Cleanup(h.server.Close)
	// Fresh connections keep the lost-acknowledgement test at the outbox retry
	// boundary rather than allowing the HTTP transport to retry internally.
	transport := &http.Transport{DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	h.client = &http.Client{Transport: transport, Timeout: 5 * time.Second}
	return h
}

func (h *orderDeliveryHarness) createBoundPayment(payments *harness) (string, createdPayment) {
	h.t.Helper()
	orderID := "order-e2e-" + uuid.NewString()
	const createOrder = `
		INSERT INTO orders (id, amount, currency, status)
		VALUES ($1, $2, 'IDR', 'PENDING_PAYMENT')
	`
	if _, err := h.db.Exec(
		h.t.Context(),
		createOrder,
		orderID,
		amount,
	); err != nil {
		h.t.Fatalf("create isolated pending order fixture: %v", err)
	}
	response := payments.request(
		http.MethodPost,
		"/v1/payments",
		map[string]any{"order_id": orderID, "amount": amount, "currency": "IDR"},
		map[string]string{"Idempotency-Key": uuid.NewString()},
	)
	expectHTTP(h.t, response, http.StatusCreated)
	var created createdPayment
	if err := json.Unmarshal(response.Body, &created); err != nil {
		h.t.Fatalf("decode order payment: %v", err)
	}
	const bindPayment = `
		UPDATE orders SET payment_intent_id=$2::uuid
		WHERE id=$1 AND payment_intent_id IS NULL AND status='PENDING_PAYMENT'
	`
	result, err := h.db.Exec(
		h.t.Context(),
		bindPayment,
		orderID,
		created.ID,
	)
	if err != nil || result.RowsAffected() != 1 {
		h.t.Fatalf("bind payment before processing: affected=%d error=%v", result.RowsAffected(), err)
	}
	h.expectPending(orderID, created.ID)
	return orderID, created
}

func (h *orderDeliveryHarness) deliveryWorker(paymentDB *pgxpool.Pool) *worker.OutboxWorker {
	h.t.Helper()
	publisher, err := orderpublisher.NewPublisher(h.server.URL, orderDeliveryToken, h.client)
	if err != nil {
		h.t.Fatal(err)
	}
	return worker.NewOutboxWorker(outboxpostgres.NewRepository(paymentDB), publisher)
}

func (h *orderDeliveryHarness) receive(event *outbox.Event) httpResult {
	h.t.Helper()
	request, err := http.NewRequestWithContext(
		h.t.Context(),
		http.MethodPost,
		h.server.URL+"/internal/events/payments",
		bytes.NewReader(event.Payload),
	)
	if err != nil {
		h.t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+orderDeliveryToken)
	request.Header.Set("Idempotency-Key", event.ID)
	request.Header.Set("X-Event-ID", event.ID)
	request.Header.Set("X-Event-Type", event.EventType)
	response, err := h.client.Do(request)
	if err != nil {
		h.t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	return httpResult{Status: response.StatusCode, Body: body}
}

type persistedOrder struct {
	Status        string
	Amount        int64
	Currency      string
	PaymentID     *string
	PaidAt        *time.Time
	ConsumedCount int
}

func (h *orderDeliveryHarness) state(orderID string) persistedOrder {
	h.t.Helper()
	const query = `
		SELECT o.status, o.amount, o.currency, o.payment_intent_id::text, o.paid_at,
			(SELECT count(*) FROM consumed_payment_events WHERE order_id=o.id)
		FROM orders o WHERE o.id=$1
	`
	var state persistedOrder
	if err := h.db.QueryRow(h.t.Context(), query, orderID).Scan(
		&state.Status,
		&state.Amount,
		&state.Currency,
		&state.PaymentID,
		&state.PaidAt,
		&state.ConsumedCount,
	); err != nil {
		h.t.Fatalf("read isolated order: %v", err)
	}
	return state
}

func (h *orderDeliveryHarness) expectPending(orderID, paymentID string) {
	h.t.Helper()
	state := h.state(orderID)
	unpaid := state.Status == "PENDING_PAYMENT" && state.PaidAt == nil
	noReceipt := state.ConsumedCount == 0
	bound := state.PaymentID != nil && *state.PaymentID == paymentID
	if !unpaid || !noReceipt || !bound {
		h.t.Fatalf("expected a bound unpaid order with no receipt: %+v", state)
	}
}

func (h *orderDeliveryHarness) expectPaid(orderID, paymentID, eventID string) time.Time {
	h.t.Helper()
	state := h.state(orderID)
	paid := state.Status == "PAID" && state.PaidAt != nil
	correctAmount := state.Amount == amount && state.Currency == "IDR"
	bound := state.PaymentID != nil && *state.PaymentID == paymentID
	if !paid || !correctAmount || !bound || state.ConsumedCount != 1 {
		h.t.Fatalf("expected exactly one captured-payment receipt and a paid order: %+v", state)
	}
	var receivedID, receivedType, receivedPaymentID string
	const receipt = `
		SELECT event_id::text, event_type, payment_intent_id::text
		FROM consumed_payment_events WHERE order_id=$1
	`
	if err := h.db.QueryRow(h.t.Context(), receipt, orderID).Scan(
		&receivedID,
		&receivedType,
		&receivedPaymentID,
	); err != nil {
		h.t.Fatal(err)
	}
	identityMatches := receivedID == eventID && receivedPaymentID == paymentID
	if !identityMatches || receivedType != outbox.EventPaymentCaptured {
		h.t.Fatal("consumed receipt does not match the captured payment outbox event")
	}
	return *state.PaidAt
}

func orderOutboxEvent(t *testing.T, db *pgxpool.Pool, paymentID string) *outbox.Event {
	t.Helper()
	var count int
	if err := db.QueryRow(
		t.Context(),
		"SELECT count(*) FROM outbox_events WHERE aggregate_id=$1",
		paymentID,
	).Scan(&count); err != nil || count != 1 {
		t.Fatalf("captured payment outbox count=%d, want 1: %v", count, err)
	}
	const query = `
		SELECT id, event_type, aggregate_id, payload, status, attempt_count,
			published_at, available_at, last_error, claim_token
		FROM outbox_events WHERE aggregate_id=$1
	`
	event := &outbox.Event{}
	if err := db.QueryRow(t.Context(), query, paymentID).Scan(
		&event.ID,
		&event.EventType,
		&event.AggregateID,
		&event.Payload,
		&event.Status,
		&event.AttemptCount,
		&event.PublishedAt,
		&event.AvailableAt,
		&event.LastError,
		&event.ClaimToken,
	); err != nil {
		t.Fatalf("read isolated payment outbox event: %v", err)
	}
	return event
}

func expectPublishedOrderEvent(t *testing.T, event *outbox.Event, attempts int) {
	t.Helper()
	published := event.Status == outbox.StatusPublished && event.PublishedAt != nil
	claimReleased := event.ClaimToken == nil
	noError := event.LastError == nil
	if !published || !claimReleased || !noError || event.AttemptCount != attempts {
		t.Fatalf("expected an acknowledged published event after %d attempts: %+v", attempts, event)
	}
}

func isolatedOrderDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("order integration tests require TEST_DATABASE_URL pointing to a dedicated test database")
	}
	goose, err := exec.LookPath("goose")
	if err != nil {
		t.Fatal("order integration tests require the goose migration executable in PATH")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("TEST_DATABASE_URL is not a valid PostgreSQL connection string")
	}
	config.MaxConns = 4
	config.MinConns = 0
	admin, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal("could not open TEST_DATABASE_URL")
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatal("cannot connect to the isolated integration test PostgreSQL database")
	}
	schema := "order_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		admin.Close()
		t.Fatalf("create isolated order schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("remove isolated order schema: %v", err)
		}
		admin.Close()
	})
	migrationDSN := dsn + " search_path=" + schema
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatal("TEST_DATABASE_URL could not be parsed")
		}
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		migrationDSN = parsed.String()
	}
	command := exec.CommandContext(
		ctx,
		goose,
		"-env=",
		"-dir",
		filepath.Join(repositoryRoot(), "order-migrations"),
		"up",
	)
	command.Env = append(os.Environ(), "GOOSE_DRIVER=postgres", "GOOSE_DBSTRING="+migrationDSN)
	if output, err := command.CombinedOutput(); err != nil {
		message := strings.ReplaceAll(string(output), migrationDSN, "[test database]")
		message = strings.ReplaceAll(message, dsn, "[test database]")
		t.Fatalf("migrate isolated order schema: %v\n%s", err, message)
	}
	orderConfig := config.Copy()
	orderConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, orderConfig)
	if err != nil {
		t.Fatal("open isolated order test pool")
	}
	t.Cleanup(pool.Close)
	var currentSchema string
	if err := pool.QueryRow(ctx, "SELECT current_schema()").Scan(&currentSchema); err != nil || currentSchema != schema {
		t.Fatalf("order schema isolation failed: schema=%q error=%v", currentSchema, err)
	}
	return pool
}
