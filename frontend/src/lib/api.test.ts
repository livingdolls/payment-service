import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, request } from "./api";

beforeEach(() => {
  vi.stubGlobal("window", { setTimeout, clearTimeout });
});
afterEach(() => {
  vi.unstubAllGlobals();
});

describe("API response handling", () => {
  it("accepts the backend 202 reconciliation response", async () => {
    const body = {
      attempt_id: "attempt",
      status: "PROCESSING",
      code: "PAYMENT_OUTCOME_UNKNOWN",
    };
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(Response.json(body, { status: 202 })),
    );
    expect(await request("/payment-attempts/attempt/process")).toEqual(body);
  });

  it("supports both string and nested error formats used by the API", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(
          Response.json({ error: "payment not found" }, { status: 404 }),
        )
        .mockResolvedValueOnce(
          Response.json(
            { error: { code: "UNAUTHORIZED", message: "unauthorized" } },
            { status: 401 },
          ),
        ),
    );
    await expect(request("/payments/missing/status")).rejects.toMatchObject({
      message: "payment not found",
      status: 404,
    });
    await expect(
      request("/admin/reconciliation-reviews"),
    ).rejects.toMatchObject({ code: "UNAUTHORIZED", status: 401 });
  });

  it("reports non-JSON and network failures without fabricating a successful payment", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(new Response("<html>unrelated service</html>"))
        .mockRejectedValueOnce(new TypeError("failed to fetch")),
    );
    await expect(request("/health/ready")).rejects.toMatchObject({
      code: "INVALID_RESPONSE",
    });
    await expect(request("/health/ready")).rejects.toMatchObject({
      code: "NETWORK_ERROR",
    });
  });

  it("sends an idempotency key and safe JSON request without a browser authorization token", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(Response.json({ id: "payment" }, { status: 201 }));
    vi.stubGlobal("fetch", fetch);
    await api.create(
      { order_id: "ORDER-01", amount: 50000, currency: "IDR" },
      "stable-key",
    );
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe("/api/payments");
    expect(options.headers.get("Idempotency-Key")).toBe("stable-key");
    expect(options.headers.get("Authorization")).toBeNull();
    expect(JSON.parse(options.body)).toEqual({
      order_id: "ORDER-01",
      amount: 50000,
      currency: "IDR",
    });
  });
});
