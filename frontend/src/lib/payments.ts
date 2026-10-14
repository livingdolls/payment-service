import type {
  CreatePaymentResponse,
  PaymentRecord,
  PaymentSnapshot,
  PaymentStatus,
  ProcessResponse,
} from "../types";

export const HISTORY_KEY = "payment-console:history:v1";
const MAX_HISTORY = 500;
export const STATUS_LABELS: Record<PaymentStatus, string> = {
  CREATED: "Baru",
  PROCESSING: "Diproses",
  REQUIRES_ACTION: "Menunggu bayar",
  AUTHORIZED: "Diotorisasi",
  CAPTURED: "Berhasil",
  PARTIALLY_CAPTURED: "Diterima sebagian",
  REFUNDED: "Dikembalikan",
  FAILED: "Gagal",
  CANCELED: "Dibatalkan",
  EXPIRED: "Kedaluwarsa",
};
export const ACTIVE_STATUSES = new Set([
  "PROCESSING",
  "REQUIRES_ACTION",
  "AUTHORIZED",
  "CREATED",
]);
export const FAILED_STATUSES = new Set(["FAILED", "CANCELED", "EXPIRED"]);

const rupiahFormatter = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  maximumFractionDigits: 0,
});
const numberFormatter = new Intl.NumberFormat("id-ID");
const dateFormatter = new Intl.DateTimeFormat("id-ID", {
  day: "2-digit",
  month: "short",
  hour: "2-digit",
  minute: "2-digit",
});

export const money = (amount: number) => rupiahFormatter.format(amount);
export const count = (value: number) => numberFormatter.format(value);
export const formatDate = (value: string) =>
  Number.isNaN(Date.parse(value))
    ? "Belum tersedia"
    : dateFormatter.format(new Date(value));
export const shortId = (value: string) =>
  value.length > 18 ? `${value.slice(0, 8)}…${value.slice(-5)}` : value;

export function fromCreated(payload: CreatePaymentResponse): PaymentRecord {
  return {
    ...payload,
    created_at_known: true,
    attempt_status: "CREATED",
    actions: [],
    captured_amount: payload.captured_amount || 0,
    refunded_amount: payload.refunded_amount || 0,
  };
}

export function fromSnapshot(
  snapshot: PaymentSnapshot,
  previous?: PaymentRecord,
): PaymentRecord {
  return {
    ...previous,
    id: snapshot.payment_intent_id,
    attempt_id: snapshot.attempt_id,
    reference_id: snapshot.reference_id,
    order_id: snapshot.order_id,
    amount: snapshot.amount,
    currency: snapshot.currency,
    status: snapshot.payment_status,
    attempt_status: snapshot.attempt_status,
    captured_amount: snapshot.captured_amount,
    refunded_amount: previous?.refunded_amount || 0,
    created_at: previous?.created_at || new Date().toISOString(),
    created_at_known: previous?.created_at_known ?? false,
    updated_at: new Date().toISOString(),
    actions: snapshot.actions || [],
  };
}

export function fromProcessed(
  previous: PaymentRecord,
  response: ProcessResponse,
  channel: string,
): PaymentRecord {
  const updated_at = new Date().toISOString();
  if ("code" in response) {
    return {
      ...previous,
      status: "PROCESSING",
      attempt_status: "UNKNOWN",
      channel_code: channel,
      actions: [],
      updated_at,
    };
  }
  return {
    ...previous,
    status: response.payment.status,
    attempt_id: response.attempt.id,
    attempt_status: response.attempt.status,
    channel_code: channel,
    actions: response.actions || [],
    updated_at,
  };
}

export function upsertPayment(
  items: PaymentRecord[],
  payment: PaymentRecord,
): PaymentRecord[] {
  const index = items.findIndex((item) => item.id === payment.id);
  if (index < 0) return [payment, ...items].slice(0, MAX_HISTORY);
  return items.map((item) => (item.id === payment.id ? payment : item));
}

export function readHistory(): PaymentRecord[] {
  try {
    const stored = JSON.parse(localStorage.getItem(HISTORY_KEY) || "null");
    if (stored?.version !== 1 || !Array.isArray(stored.items)) return [];
    return stored.items
      .filter(
        (item: Partial<PaymentRecord>) =>
          typeof item.id === "string" &&
          typeof item.attempt_id === "string" &&
          typeof item.order_id === "string" &&
          typeof item.reference_id === "string" &&
          typeof item.amount === "number" &&
          Number.isSafeInteger(item.amount) &&
          item.amount > 0 &&
          typeof item.currency === "string" &&
          typeof item.attempt_status === "string" &&
          typeof item.captured_amount === "number" &&
          Number.isSafeInteger(item.captured_amount) &&
          typeof item.refunded_amount === "number" &&
          Number.isSafeInteger(item.refunded_amount) &&
          typeof item.updated_at === "string" &&
          typeof item.status === "string" &&
          Object.hasOwn(STATUS_LABELS, item.status) &&
          typeof item.created_at === "string" &&
          !Number.isNaN(Date.parse(item.created_at)),
      )
      .slice(0, MAX_HISTORY)
      .map((item: PaymentRecord) => ({ ...item, actions: [] }));
  } catch {
    return [];
  }
}

export function persistHistory(items: PaymentRecord[]): boolean {
  try {
    // Provider action URLs may contain credentials. Keep actions in memory only.
    const safeItems = items
      .slice(0, MAX_HISTORY)
      .map(({ actions: _actions, ...item }) => item);
    localStorage.setItem(
      HISTORY_KEY,
      JSON.stringify({ version: 1, items: safeItems }),
    );
    return true;
  } catch {
    return false;
  }
}

export function filterPayments(
  items: PaymentRecord[],
  query: string,
  status: string,
  days: number | null,
  now = new Date(),
): PaymentRecord[] {
  const search = query.toLocaleLowerCase("id-ID").trim();
  const minimumDate = days === null ? 0 : now.getTime() - days * 86400000;
  return items.filter((item) => {
    const matchesQuery =
      !search ||
      [item.id, item.order_id, item.reference_id, item.channel_code || ""].some(
        (value) => value.toLocaleLowerCase("id-ID").includes(search),
      );
    const matchesStatus =
      status === "all" ||
      (status === "pending"
        ? ACTIVE_STATUSES.has(item.status)
        : status === "failed"
          ? FAILED_STATUSES.has(item.status)
          : item.status === status);
    return (
      matchesQuery &&
      matchesStatus &&
      Date.parse(item.created_at) >= minimumDate
    );
  });
}

export function dailyVolume(
  items: PaymentRecord[],
  days: number,
  now = new Date(),
) {
  return Array.from({ length: days }, (_, index) => {
    const date = new Date(
      now.getFullYear(),
      now.getMonth(),
      now.getDate() - days + index + 1,
    );
    const dateKey = date.toLocaleDateString("en-CA");
    const relevant = items.filter(
      (item) =>
        new Date(item.created_at).toLocaleDateString("en-CA") === dateKey,
    );
    return {
      date,
      label: date.toLocaleDateString("id-ID", {
        day: "numeric",
        month: "short",
      }),
      amount: relevant.reduce((sum, item) => sum + item.amount, 0),
      total: relevant.length,
    };
  });
}

export function safeActionUrl(value: string): string | null {
  try {
    const url = new URL(value);
    return ["https:", "http:"].includes(url.protocol) &&
      !url.username &&
      !url.password
      ? url.href
      : null;
  } catch {
    return null;
  }
}

function csvCell(value: string | number): string {
  const text = String(value);
  // Spreadsheet apps interpret unescaped leading symbols as formulas.
  const safe = /^[=+@\-\t\r]/.test(text) ? `'${text}` : text;
  return `"${safe.replaceAll('"', '""')}"`;
}

export function paymentsCsv(items: PaymentRecord[]): string {
  const columns = [
    "ID pembayaran",
    "ID pesanan",
    "Referensi",
    "Nominal",
    "Mata uang",
    "Status",
    "Metode",
    "Dibuat",
  ];
  const rows = items.map((item) => [
    item.id,
    item.order_id,
    item.reference_id,
    item.amount,
    item.currency,
    item.status,
    item.channel_code || "",
    item.created_at,
  ]);
  return (
    "\uFEFF" +
    [columns, ...rows].map((row) => row.map(csvCell).join(",")).join("\r\n")
  );
}

export function downloadCsv(items: PaymentRecord[]) {
  const url = URL.createObjectURL(
    new Blob([paymentsCsv(items)], { type: "text/csv;charset=utf-8;" }),
  );
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `pembayaran-${new Date().toISOString().slice(0, 10)}.csv`;
  anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
