import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  filterPayments,
  fromProcessed,
  fromSnapshot,
  paymentsCsv,
  persistHistory,
  readHistory,
  safeActionUrl,
  upsertPayment,
} from "./payments";
import type { PaymentRecord, PaymentSnapshot } from "../types";

const payment: PaymentRecord = {
  id: "f5000000-0000-4000-8000-000000000001",
  attempt_id: "f6000000-0000-4000-8000-000000000001",
  order_id: "ORDER-TEST",
  reference_id: "REF-001",
  amount: 75000,
  currency: "IDR",
  status: "CREATED",
  attempt_status: "CREATED",
  captured_amount: 0,
  refunded_amount: 0,
  created_at: "2026-10-14T05:00:00Z",
  updated_at: "2026-10-14T05:00:00Z",
  actions: [
    {
      type: "REDIRECT_CUSTOMER",
      descriptor: "WEB_URL",
      value: "https://checkout.example/pay?token=private",
    },
  ],
};

beforeEach(() => {
  const storage = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => storage.get(key) || null,
    setItem: (key: string, value: string) => storage.set(key, value),
  });
});

describe("payment lifecycle", () => {
  it("keeps a 202 unknown provider outcome pending instead of treating it as a capture or retry", () => {
    const result = fromProcessed(
      payment,
      {
        attempt_id: payment.attempt_id,
        status: "PROCESSING",
        code: "PAYMENT_OUTCOME_UNKNOWN",
        message: "pending",
      },
      "QRIS",
    );
    expect(result.status).toBe("PROCESSING");
    expect(result.attempt_status).toBe("UNKNOWN");
    expect(result.actions).toEqual([]);
    expect(result.captured_amount).toBe(0);
  });

  it("merges a status snapshot without losing channel information and clears provider actions", () => {
    const snapshot: PaymentSnapshot = {
      payment_intent_id: payment.id,
      attempt_id: payment.attempt_id,
      reference_id: payment.reference_id,
      order_id: payment.order_id,
      amount: 75000,
      currency: "IDR",
      payment_status: "CAPTURED",
      attempt_status: "CAPTURED",
      captured_amount: 75000,
      actions: null,
    };
    const result = fromSnapshot(snapshot, {
      ...payment,
      channel_code: "QRIS",
      created_at_known: true,
    });
    expect(result.status).toBe("CAPTURED");
    expect(result.captured_amount).toBe(75000);
    expect(result.channel_code).toBe("QRIS");
    expect(result.created_at).toBe(payment.created_at);
    expect(result.actions).toEqual([]);
    expect(fromSnapshot(snapshot).created_at_known).toBe(false);
  });

  it("updates an existing payment by ID without creating another history record", () => {
    const result = upsertPayment([payment], {
      ...payment,
      status: "PROCESSING",
    });
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("PROCESSING");
  });
});

describe("local history and exports", () => {
  it("never persists action URLs or provider credentials", () => {
    expect(persistHistory([payment])).toBe(true);
    const raw = localStorage.getItem("payment-console:history:v1")!;
    expect(raw).not.toContain("private");
    expect(raw).not.toContain("actions");
    expect(readHistory()[0].actions).toEqual([]);
    expect(readHistory()[0].id).toBe(payment.id);
  });

  it("recovers safely from corrupted, old, or invalid browser data", () => {
    localStorage.setItem("payment-console:history:v1", "{invalid");
    expect(readHistory()).toEqual([]);
    localStorage.setItem(
      "payment-console:history:v1",
      JSON.stringify({
        version: 1,
        items: [
          { ...payment, amount: "75000" },
          { ...payment, captured_amount: null },
          { ...payment, status: "toString" },
        ],
      }),
    );
    expect(readHistory()).toEqual([]);
    vi.stubGlobal("localStorage", {
      getItem: () => {
        throw new Error("disabled");
      },
      setItem: () => {
        throw new Error("disabled");
      },
    });
    expect(readHistory()).toEqual([]);
    expect(persistHistory([payment])).toBe(false);
  });

  it("filters by query, terminal/pending status, and selected period together", () => {
    const items = [
      payment,
      {
        ...payment,
        id: "old",
        order_id: "OTHER",
        status: "FAILED" as const,
        created_at: "2026-09-01T00:00:00Z",
      },
    ];
    expect(
      filterPayments(
        items,
        "order",
        "pending",
        7,
        new Date("2026-10-14T06:00:00Z"),
      ),
    ).toEqual([payment]);
    expect(filterPayments(items, "", "failed", null)).toHaveLength(1);
    expect(filterPayments(items, "missing", "all", null)).toHaveLength(0);
  });

  it("quotes CSV content and neutralizes formulas in user-provided IDs", () => {
    const csv = paymentsCsv([{ ...payment, order_id: '=HYPERLINK("evil")' }]);
    expect(csv).toContain('"\'=HYPERLINK(""evil"")"');
    expect(csv).toContain('"75000"');
    expect(csv).not.toContain("checkout.example");
  });

  it("renders only safe HTTP(S) action links", () => {
    expect(safeActionUrl("https://checkout.example/pay")).toBe(
      "https://checkout.example/pay",
    );
    expect(safeActionUrl("javascript:alert(1)")).toBeNull();
    expect(safeActionUrl("https://user:password@checkout.example")).toBeNull();
    expect(safeActionUrl("data:text/html,unsafe")).toBeNull();
  });
});
