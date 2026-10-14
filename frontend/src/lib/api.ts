import type {
  ConsoleConfig,
  CreatePaymentResponse,
  PaymentSnapshot,
  ProcessResponse,
  ReviewItem,
} from "../types";

export class ApiError extends Error {
  constructor(
    message: string,
    public status = 0,
    public code = "NETWORK_ERROR",
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), 20000);
  const headers = new Headers(options.headers);
  headers.set("Accept", "application/json");
  if (options.body) headers.set("Content-Type", "application/json");

  try {
    const response = await fetch(`/api${path}`, {
      ...options,
      headers,
      cache: "no-store",
      signal: options.signal || controller.signal,
    });
    let payload: unknown;
    try {
      payload = await response.json();
    } catch {
      throw new ApiError(
        "Respons API tidak dapat dibaca. Periksa koneksi dan konfigurasi proxy.",
        response.status,
        "INVALID_RESPONSE",
      );
    }

    if (!response.ok) {
      const body = payload as {
        error?: string | { message?: string; code?: string };
      };
      const error = body?.error;
      const message = typeof error === "string" ? error : error?.message;
      const code = typeof error === "object" ? error?.code : undefined;
      throw new ApiError(
        message || `Permintaan gagal (HTTP ${response.status}).`,
        response.status,
        code || "API_ERROR",
      );
    }
    return payload as T;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    if (error instanceof Error && error.name === "AbortError") {
      throw new ApiError(
        "API belum merespons. Periksa status sebelum mengulangi pembayaran.",
        0,
        "TIMEOUT",
      );
    }
    throw new ApiError(
      "Tidak dapat terhubung ke API. Pastikan payment-service berjalan.",
    );
  } finally {
    window.clearTimeout(timeout);
  }
}

export const api = {
  config: () => request<ConsoleConfig>("/console-config"),
  health: () => request<{ status: string; database: string }>("/health/ready"),
  create: (
    body: { order_id: string; amount: number; currency: string },
    key: string,
  ) =>
    request<CreatePaymentResponse>("/payments", {
      method: "POST",
      headers: { "Idempotency-Key": key },
      body: JSON.stringify(body),
    }),
  status: (id: string) =>
    request<PaymentSnapshot>(`/payments/${encodeURIComponent(id)}/status`),
  process: (
    attemptId: string,
    body: {
      country: string;
      channel_code: string;
      channel_properties: Record<string, unknown>;
      description: string;
    },
  ) =>
    request<ProcessResponse>(
      `/payment-attempts/${encodeURIComponent(attemptId)}/process`,
      { method: "POST", body: JSON.stringify(body) },
    ),
  reviews: (limit = 50) =>
    request<{ data: ReviewItem[] | null }>(
      `/admin/reconciliation-reviews?limit=${limit}`,
    ),
  requeue: (attemptId: string, reason: string) =>
    request<{ status: string }>(
      `/admin/payment-attempts/${encodeURIComponent(attemptId)}/requeue-reconciliation`,
      { method: "POST", body: JSON.stringify({ reason }) },
    ),
};

export function errorMessage(error: unknown): string {
  return error instanceof Error
    ? error.message
    : "Terjadi kesalahan. Silakan coba lagi.";
}
