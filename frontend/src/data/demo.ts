import type { PaymentRecord, PaymentStatus, ReviewItem } from "../types";

export function demoPayments(): PaymentRecord[] {
  const codes = [
    "QRIS",
    "BCA_VIRTUAL_ACCOUNT",
    "DANA",
    "MANDIRI_VIRTUAL_ACCOUNT",
    "QRIS",
    "GOPAY",
    "BRI_VIRTUAL_ACCOUNT",
    "SHOPEEPAY",
  ];
  const amounts = [
    185000, 75000, 325000, 249000, 125000, 489000, 95000, 175000, 650000,
    215000, 89000, 379000,
  ];
  const statuses: PaymentStatus[] = [
    "CAPTURED",
    "CAPTURED",
    "REQUIRES_ACTION",
    "CAPTURED",
    "CAPTURED",
    "FAILED",
    "CAPTURED",
    "CREATED",
    "CAPTURED",
    "PROCESSING",
    "CAPTURED",
    "EXPIRED",
  ];
  const now = new Date();
  const result: PaymentRecord[] = [];
  let sequence = 0;
  for (let day = 0; day < 30; day++) {
    const total = 2 + ((day * 7 + 3) % 6);
    for (let index = 0; index < total; index++) {
      sequence++;
      const date = new Date(now.getTime() - day * 86400000 - index * 2730000);
      const status = statuses[(sequence + day) % statuses.length];
      const amount = amounts[(sequence * 7 + day) % amounts.length];
      result.push({
        id: `de000000-0000-4000-8000-${String(sequence).padStart(12, "0")}`,
        attempt_id: `da000000-0000-4000-8000-${String(sequence).padStart(12, "0")}`,
        order_id: `ORD-${date.toISOString().slice(2, 10).replaceAll("-", "")}-${String(sequence + 1034).padStart(4, "0")}`,
        reference_id: `PAY-${String(sequence + 7802).padStart(6, "0")}`,
        amount,
        currency: "IDR",
        status,
        attempt_status: status === "PROCESSING" ? "PENDING" : status,
        captured_amount: status === "CAPTURED" ? amount : 0,
        refunded_amount: 0,
        channel_code:
          status === "CREATED"
            ? undefined
            : codes[(sequence + day) % codes.length],
        created_at: date.toISOString(),
        updated_at: date.toISOString(),
        actions: [],
      });
    }
  }
  return result;
}

export function demoReviews(payments: PaymentRecord[]): ReviewItem[] {
  return payments
    .filter((payment) => payment.status === "PROCESSING")
    .slice(0, 3)
    .map((payment, index) => ({
      attempt_id: payment.attempt_id,
      order_id: payment.order_id,
      reference_id: payment.reference_id,
      amount: payment.amount,
      currency: "IDR",
      payment_status: payment.status,
      attempt_status: "UNKNOWN",
      provider_payment_request_id: null,
      reconcile_state: "NEEDS_REVIEW",
      reconcile_attempts: 8 + index * 2,
      reconcile_failures: 4 + index,
      last_reconcile_error: "Provider belum memberikan status final.",
      next_reconcile_at: null,
    }));
}
