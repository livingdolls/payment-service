//go:build integration

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livingdolls/payment-service/internal/application"
	"github.com/livingdolls/payment-service/internal/database"
	router "github.com/livingdolls/payment-service/internal/http"
	"github.com/livingdolls/payment-service/internal/http/handler"
	"github.com/livingdolls/payment-service/internal/modules/payment"
	paymentpostgres "github.com/livingdolls/payment-service/internal/modules/payment/postgres"
	reviewpostgres "github.com/livingdolls/payment-service/internal/modules/reconciliation/postgres"
	"github.com/livingdolls/payment-service/internal/modules/webhook"
	webhookpostgres "github.com/livingdolls/payment-service/internal/modules/webhook/postgres"
	"github.com/livingdolls/payment-service/internal/provider/xendit"
)

const (
	webhookToken = "integration-webhook-token"
	adminToken   = "integration-admin-token-at-least-32-characters"
	adminActor   = "integration-test-operator"
	amount       = int64(50000)
)

type harness struct {
	t         *testing.T
	db        *pgxpool.Pool
	api       *httptest.Server
	xendit    *fakeXendit
	webhooks  *application.WebhookProcessor
	reconcile *application.ReconciliationProcessor
}

type createdPayment struct {
	ID          string `json:"id"`
	AttemptID   string `json:"attempt_id"`
	ReferenceID string `json:"reference_id"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
}

type httpResult struct {
	Status int
	Body   []byte
}

func repositoryRoot() string {
	_, source, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
}

func isolatedDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("integration tests require TEST_DATABASE_URL pointing to a dedicated test database")
	}
	goose, err := exec.LookPath("goose")
	if err != nil {
		t.Fatal("integration tests require the goose migration executable in PATH")
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
		t.Fatal("cannot connect to TEST_DATABASE_URL; start the isolated test PostgreSQL server")
	}
	schema := "payment_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		admin.Close()
		t.Fatalf("create isolated test schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("remove isolated test schema: %v", err)
		}
		admin.Close()
	})
	// Both goose and every pooled connection use only this random schema.
	// Never migrate, truncate, or query the shared public schema.
	runGoose(
		t,
		ctx,
		goose,
		dsn,
		schema,
		"up",
	)
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal("open isolated test pool")
	}
	t.Cleanup(pool.Close)
	var currentSchema string
	if err := pool.QueryRow(ctx, "SELECT current_schema()").Scan(&currentSchema); err != nil || currentSchema != schema {
		t.Fatalf("test schema isolation failed: schema=%q error=%v", currentSchema, err)
	}
	return pool
}

func runGoose(t *testing.T, ctx context.Context, goose, dsn, schema string, action ...string) {
	t.Helper()
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
	arguments := append([]string{"-env=", "-dir", filepath.Join(repositoryRoot(), "migrations")}, action...)
	command := exec.CommandContext(ctx, goose, arguments...)
	command.Env = append(os.Environ(), "GOOSE_DRIVER=postgres", "GOOSE_DBSTRING="+migrationDSN)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.ReplaceAll(string(output), migrationDSN, "[test database]")
		message = strings.ReplaceAll(message, dsn, "[test database]")
		t.Fatalf(
			"migrate isolated schema (%s): %v\n%s",
			strings.Join(action, " "),
			err,
			message,
		)
	}
}

func newHarness(t *testing.T, status string) *harness {
	t.Helper()
	db := isolatedDatabase(t)
	fake := newFakeXendit(t, status)
	client := xendit.NewClient("integration-provider-key", fake.server.URL)
	tx := database.NewTransactor(db)
	create := application.NewCreatePaymentUseCase(tx)
	process := application.NewProcessPaymentUseCase(tx, client)
	webhookRepo := webhookpostgres.NewRepository(db)
	reviewRepo := reviewpostgres.NewReviewRepository(db, tx)
	api := httptest.NewServer(router.NewRouter(
		db,
		handler.NewPaymentHandler(create, process),
		handler.NewWebhookHandler(webhook.NewService(webhookRepo, webhookToken)),
		handler.NewReconciliationReviewHandler(application.NewReconciliationReviewUseCase(reviewRepo)),
		adminToken,
		adminActor,
	))
	t.Cleanup(api.Close)
	return &harness{
		t: t, db: db, api: api, xendit: fake,
		webhooks: application.NewWebhookProcessor(db, tx),
		reconcile: application.NewReconciliationProcessor(
			db,
			client,
			client,
			process,
		),
	}
}

func (h *harness) request(method, path string, body any, headers map[string]string) httpResult {
	h.t.Helper()
	var data []byte
	var err error
	if raw, ok := body.([]byte); ok {
		data = raw
	} else if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal request: %v", err)
		}
	}
	request, err := http.NewRequestWithContext(
		h.t.Context(),
		method,
		h.api.URL+path,
		bytes.NewReader(data),
	)
	if err != nil {
		h.t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := h.api.Client().Do(request)
	if err != nil {
		h.t.Fatalf("API request: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		h.t.Fatalf("read API response: %v", err)
	}
	return httpResult{Status: response.StatusCode, Body: responseBody}
}

func expectHTTP(t *testing.T, response httpResult, status int) {
	t.Helper()
	if response.Status != status {
		t.Fatalf(
			"HTTP status=%d want=%d response=%s",
			response.Status,
			status,
			response.Body,
		)
	}
}

func (h *harness) create() createdPayment {
	h.t.Helper()
	response := h.request(
		http.MethodPost,
		"/v1/payments",
		map[string]any{
			"order_id": "e2e-" + uuid.NewString(), "amount": amount, "currency": "IDR",
		},
		map[string]string{"Idempotency-Key": uuid.NewString()},
	)
	expectHTTP(h.t, response, http.StatusCreated)
	var created createdPayment
	if err := json.Unmarshal(response.Body, &created); err != nil {
		h.t.Fatalf("decode created payment: %v", err)
	}
	if created.ID == "" || created.AttemptID == "" || created.ReferenceID == "" || created.Status != "CREATED" {
		h.t.Fatalf("invalid create response: %s", response.Body)
	}
	return created
}

func (h *harness) process(created createdPayment, code string, properties map[string]any) httpResult {
	h.t.Helper()
	return h.request(
		http.MethodPost,
		"/v1/payment-attempts/"+created.AttemptID+"/process",
		map[string]any{
			"country": "ID", "channel_code": code, "channel_properties": properties,
			"metadata": map[string]any{"attempt_id": created.AttemptID},
		},
		nil,
	)
}

func (h *harness) state(created createdPayment) (*payment.PaymentIntent, *payment.PaymentAttempt) {
	h.t.Helper()
	intent, err := paymentpostgres.NewRepository(h.db).GetByID(h.t.Context(), created.ID)
	if err != nil {
		h.t.Fatalf("read persisted payment: %v", err)
	}
	attempt, err := paymentpostgres.NewAttemptRepository(h.db).GetAttemptByID(h.t.Context(), created.AttemptID)
	if err != nil {
		h.t.Fatalf("read persisted attempt: %v", err)
	}
	return intent, attempt
}

func (h *harness) expectState(created createdPayment, intentStatus, attemptStatus string, captured int64) {
	h.t.Helper()
	intent, attempt := h.state(created)
	intentMatches := string(intent.Status) == intentStatus
	attemptMatches := string(attempt.Status) == attemptStatus
	amountMatches := intent.CapturedAmount == captured
	if !intentMatches || !attemptMatches || !amountMatches {
		h.t.Fatalf(
			"persisted state payment=%s attempt=%s captured=%d; want %s/%s/%d",
			intent.Status,
			attempt.Status,
			intent.CapturedAmount,
			intentStatus,
			attemptStatus,
			captured,
		)
	}
}

func (h *harness) event(created createdPayment, kind string) map[string]any {
	h.t.Helper()
	status := "SUCCEEDED"
	if kind == "payment.authorization" {
		status = "AUTHORIZED"
	} else if kind == "payment.failure" {
		status = "FAILED"
	}
	return map[string]any{
		"event": kind, "business_id": "integration-business", "created": time.Now().UTC().Format(time.RFC3339Nano),
		"data": map[string]any{
			"payment_id":         "py-" + created.AttemptID,
			"payment_request_id": "pr-" + created.AttemptID,
			"reference_id":       created.ReferenceID,
			"status":             status, "currency": "IDR", "request_amount": created.Amount,
			"captures": []map[string]any{{"capture_id": "cap-" + created.AttemptID, "capture_amount": created.Amount}},
		},
	}
}

func (h *harness) receive(event map[string]any) httpResult {
	h.t.Helper()
	return h.request(
		http.MethodPost,
		"/webhooks/xendit/payments",
		event,
		map[string]string{"x-callback-token": webhookToken},
	)
}

func (h *harness) finish(created createdPayment) {
	h.t.Helper()
	expectHTTP(h.t, h.receive(h.event(created, "payment.capture")), http.StatusOK)
	processed, err := h.webhooks.ProcessNext(h.t.Context())
	if err != nil || !processed {
		h.t.Fatalf("process capture webhook: processed=%t error=%v", processed, err)
	}
	h.expectState(
		created,
		"CAPTURED",
		"CAPTURED",
		created.Amount,
	)
	var status string
	query := h.db.QueryRow(h.t.Context(), "SELECT status FROM webhook_events")
	if err := query.Scan(&status); err != nil || status != "PROCESSED" {
		h.t.Fatalf("persisted webhook status=%q error=%v", status, err)
	}
}

func (h *harness) due(attemptID string) {
	h.t.Helper()
	const query = `
		UPDATE payment_attempts
		SET updated_at=NOW()-INTERVAL '3 minutes', next_reconcile_at=NULL
		WHERE id=$1
	`
	if _, err := h.db.Exec(h.t.Context(), query, attemptID); err != nil {
		h.t.Fatalf("make isolated reconciliation fixture due: %v", err)
	}
}

type fakeXendit struct {
	server       *httptest.Server
	status       string
	createCode   int
	getCode      int
	createCalls  atomic.Int64
	getCalls     atomic.Int64
	lookupCalls  atomic.Int64
	mu           sync.Mutex
	input        map[string]any
	getOverrides map[string]any
	onCreate     func(map[string]any)
}

func newFakeXendit(t *testing.T, status string) *fakeXendit {
	t.Helper()
	fake := &fakeXendit{status: status}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		key, password, ok := request.BasicAuth()
		if !ok || key != "integration-provider-key" || password != "" {
			t.Error("provider request is missing expected test-only Basic Auth")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v3/payment_requests":
			fake.createCalls.Add(1)
			if request.Header.Get("api-version") != "2024-11-11" {
				t.Error("provider API version is missing")
			}
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Errorf("decode provider input: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			fake.mu.Lock()
			fake.input = input
			fake.mu.Unlock()
			if fake.onCreate != nil {
				fake.onCreate(input)
			}
			if fake.createCode != 0 {
				w.WriteHeader(fake.createCode)
				_, _ = io.WriteString(w, `{"error_code":"INTEGRATION_PROVIDER_ERROR","message":"synthetic provider failure"}`)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(fake.snapshot(input))
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v3/payment_requests/"):
			fake.getCalls.Add(1)
			if fake.getCode != 0 {
				w.WriteHeader(fake.getCode)
				_, _ = io.WriteString(w, `{"error_code":"INTEGRATION_READ_ERROR","message":"synthetic status read failure"}`)
				return
			}
			fake.mu.Lock()
			input := fake.input
			overrides := fake.getOverrides
			fake.mu.Unlock()
			snapshot := fake.snapshot(input)
			for key, value := range overrides {
				snapshot[key] = value
			}
			_ = json.NewEncoder(w).Encode(snapshot)
		case request.Method == http.MethodGet && request.URL.Path == "/transactions":
			fake.lookupCalls.Add(1)
			fake.mu.Lock()
			input := fake.input
			fake.mu.Unlock()
			inputAvailable := input != nil
			referenceMatches := request.URL.Query().Get("reference_id") == input["reference_id"]
			paymentType := request.URL.Query().Get("types") == "PAYMENT"
			if !inputAvailable || !referenceMatches || !paymentType {
				t.Error("transaction recovery query did not use the persisted reference")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"has_more": false, "data": []map[string]any{{
				"type": "PAYMENT", "reference_id": input["reference_id"], "currency": "IDR", "amount": input["request_amount"],
				"data": map[string]any{"payment_request_id": fake.requestID(input)},
			}}})
		default:
			t.Errorf("unexpected provider call: %s %s", request.Method, request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeXendit) requestID(input map[string]any) string {
	return "pr-" + fmt.Sprint(input["metadata"].(map[string]any)["attempt_id"])
}

func (f *fakeXendit) snapshot(input map[string]any) map[string]any {
	id := f.requestID(input)
	f.mu.Lock()
	status := f.status
	f.mu.Unlock()
	return map[string]any{
		"payment_request_id": id, "latest_payment_id": "py-" + strings.TrimPrefix(id, "pr-"),
		"reference_id":   input["reference_id"],
		"request_amount": input["request_amount"],
		"currency":       input["currency"],
		"status":         status,
		"actions": []map[string]any{{
			"type": "PRESENT_TO_CUSTOMER", "descriptor": "QR_STRING", "value": "synthetic-payment-code",
		}},
	}
}

func (f *fakeXendit) setStatus(status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
}

func (f *fakeXendit) setGetOverrides(overrides map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getOverrides = overrides
}
