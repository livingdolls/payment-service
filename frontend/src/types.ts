export type PaymentStatus =
  | "CREATED"
  | "PROCESSING"
  | "REQUIRES_ACTION"
  | "AUTHORIZED"
  | "CAPTURED"
  | "PARTIALLY_CAPTURED"
  | "REFUNDED"
  | "FAILED"
  | "CANCELED"
  | "EXPIRED";

export interface PaymentAction {
  type: string;
  descriptor: string;
  value: string;
}

export interface PaymentRecord {
  id: string;
  attempt_id: string;
  reference_id: string;
  order_id: string;
  amount: number;
  currency: string;
  status: PaymentStatus;
  attempt_status: string;
  captured_amount: number;
  refunded_amount: number;
  created_at: string;
  created_at_known?: boolean;
  updated_at: string;
  channel_code?: string;
  description?: string;
  actions: PaymentAction[];
}

export interface CreatePaymentResponse {
  id: string;
  attempt_id: string;
  reference_id: string;
  order_id: string;
  amount: number;
  currency: string;
  status: PaymentStatus;
  captured_amount: number;
  refunded_amount: number;
  created_at: string;
  updated_at: string;
}

export interface PaymentSnapshot {
  payment_intent_id: string;
  attempt_id: string;
  reference_id: string;
  order_id: string;
  amount: number;
  currency: string;
  captured_amount: number;
  payment_status: PaymentStatus;
  attempt_status: string;
  actions: PaymentAction[] | null;
}

export type ProcessResponse =
  | {
      payment: { id: string; reference_id: string; status: PaymentStatus };
      attempt: { id: string; status: string; provider: string };
      actions: PaymentAction[] | null;
    }
  | {
      attempt_id: string;
      status: "PROCESSING";
      code: "PAYMENT_OUTCOME_UNKNOWN";
      message: string;
    };

export interface ReviewItem {
  attempt_id: string;
  order_id: string;
  reference_id: string;
  amount: number;
  currency: string;
  payment_status: PaymentStatus;
  attempt_status: string;
  provider_payment_request_id: string | null;
  reconcile_state: string;
  reconcile_attempts: number;
  reconcile_failures: number;
  last_reconcile_error: string | null;
  next_reconcile_at: string | null;
}

export interface ConsoleConfig {
  apiUrl: string;
  environment: string;
  internalConfigured: boolean;
  adminConfigured: boolean;
}

export type Page =
  "overview" | "payments" | "channels" | "reviews" | "settings" | "guide";
export type ToastMessage = {
  message: string;
  kind: "success" | "error" | "info";
};
