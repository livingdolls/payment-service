import http from "node:http";
import { randomUUID } from "node:crypto";

const internalToken = "frontend-test-internal-token-at-least-32-characters";
const adminToken = "frontend-test-admin-token-at-least-32-characters";
let payments = new Map();
let requests = [];
let keys = new Map();
let mode = "normal";
const review = {
  attempt_id: "b6000000-0000-4000-8000-000000000001",
  order_id: "ORDER-REVIEW-01",
  reference_id: "REF-REVIEW",
  amount: 175000,
  currency: "IDR",
  payment_status: "PROCESSING",
  attempt_status: "UNKNOWN",
  provider_payment_request_id: null,
  reconcile_state: "NEEDS_REVIEW",
  reconcile_attempts: 9,
  reconcile_failures: 4,
  last_reconcile_error: "Provider belum merespons.",
  next_reconcile_at: null,
};
let reviews = [review];

http
  .createServer(async (req, res) => {
    const path = new URL(req.url, "http://127.0.0.1").pathname;
    const send = (status, body) => {
      res.writeHead(status, { "Content-Type": "application/json" });
      res.end(JSON.stringify(body));
    };
    let raw = "";
    for await (const part of req) raw += part;
    const body = raw ? JSON.parse(raw) : null;
    if (path === "/health/ready")
      return send(200, { status: "ok", database: "healthy" });
    if (path === "/__test/reset") {
      payments = new Map();
      keys = new Map();
      requests = [];
      reviews = [review];
      mode = "normal";
      return send(200, { ok: true });
    }
    if (path === "/__test/requests") return send(200, requests);
    if (path === "/__test/mode") {
      mode = body.mode;
      return send(200, { ok: true });
    }
    requests.push({
      method: req.method,
      path,
      body,
      authorization: req.headers.authorization,
      key: req.headers["idempotency-key"],
    });
    if (
      path.startsWith("/internal/") &&
      req.headers.authorization !== `Bearer ${internalToken}`
    )
      return send(401, {
        error: { code: "UNAUTHORIZED", message: "unauthorized" },
      });
    if (
      path.startsWith("/admin/") &&
      req.headers.authorization !== `Bearer ${adminToken}`
    )
      return send(401, {
        error: { code: "UNAUTHORIZED", message: "unauthorized" },
      });
    if (path === "/internal/payments" && req.method === "POST") {
      if (!req.headers["idempotency-key"])
        return send(400, {
          error: {
            code: "IDEMPOTENCY_KEY_REQUIRED",
            message: "Idempotency-Key is required",
          },
        });
      if (mode === "create-fail") {
        mode = "normal";
        return send(503, {
          error: {
            code: "UNAVAILABLE",
            message: "Layanan sementara tidak tersedia.",
          },
        });
      }
      if (keys.has(req.headers["idempotency-key"]))
        return send(201, keys.get(req.headers["idempotency-key"]));
      const item = {
        id: randomUUID(),
        attempt_id: randomUUID(),
        reference_id: `REF-${payments.size + 1}`,
        ...body,
        status: "CREATED",
        attempt_status: "CREATED",
        captured_amount: 0,
        refunded_amount: 0,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        actions: [],
      };
      payments.set(item.id, item);
      keys.set(req.headers["idempotency-key"], item);
      return send(201, item);
    }
    if (
      /^\/internal\/payment-attempts\/[^/]+\/process$/.test(path) &&
      req.method === "POST"
    ) {
      const id = path.split("/")[3];
      const payment = [...payments.values()].find(
        (item) => item.attempt_id === id,
      );
      if (!payment)
        return send(404, {
          error: { code: "NOT_FOUND", message: "attempt not found" },
        });
      if (mode === "unknown-outcome") {
        payment.status = "PROCESSING";
        payment.attempt_status = "UNKNOWN";
        return send(202, {
          attempt_id: id,
          status: "PROCESSING",
          code: "PAYMENT_OUTCOME_UNKNOWN",
          message: "payment outcome is being reconciled",
        });
      }
      payment.status = "REQUIRES_ACTION";
      payment.attempt_status = "REQUIRES_ACTION";
      payment.actions =
        body.channel_code === "QRIS"
          ? [
              {
                type: "PRESENT_TO_CUSTOMER",
                descriptor: "QR_STRING",
                value: "MOCK-QRIS:payment-test",
              },
            ]
          : [
              {
                type: "PRESENT_TO_CUSTOMER",
                descriptor: "VIRTUAL_ACCOUNT_NUMBER",
                value: "8808123456789",
              },
            ];
      return send(200, {
        payment: {
          id: payment.id,
          reference_id: payment.reference_id,
          status: payment.status,
        },
        attempt: {
          id: payment.attempt_id,
          status: payment.attempt_status,
          provider: "XENDIT",
        },
        actions: payment.actions,
      });
    }
    if (/^\/internal\/payments\/[^/]+\/status$/.test(path)) {
      const id = path.split("/")[3];
      const payment = payments.get(id);
      if (!payment) return send(404, { error: "payment not found" });
      return send(200, {
        ...payment,
        payment_intent_id: payment.id,
        payment_status: payment.status,
      });
    }
    if (path === "/admin/reconciliation-reviews")
      return send(200, { data: reviews });
    if (
      /^\/admin\/payment-attempts\/[^/]+\/requeue-reconciliation$/.test(path)
    ) {
      if (!body.reason.trim())
        return send(400, {
          error: { code: "INVALID_REQUEST", message: "reason is required" },
        });
      reviews = reviews.filter(
        (item) => item.attempt_id !== path.split("/")[3],
      );
      return send(202, { status: "QUEUED" });
    }
    return send(404, { error: "not found" });
  })
  .listen(18991, "127.0.0.1");
