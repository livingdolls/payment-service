import * as Dialog from "@radix-ui/react-dialog";
import { IconContext } from "@phosphor-icons/react/dist/lib/context";
import type { Icon } from "@phosphor-icons/react";
import { XIcon } from "@phosphor-icons/react/dist/csr/X";
import { CheckCircleIcon } from "@phosphor-icons/react/dist/csr/CheckCircle";
import { WarningCircleIcon } from "@phosphor-icons/react/dist/csr/WarningCircle";
import { InfoIcon } from "@phosphor-icons/react/dist/csr/Info";
import type { ReactNode } from "react";
import { STATUS_LABELS } from "../lib/payments";
import type { PaymentStatus, ToastMessage } from "../types";
import type { Channel } from "../data/channels";

export function IconsProvider({ children }: { children: ReactNode }) {
  return (
    <IconContext.Provider value={{ size: 20, weight: "regular" }}>
      {children}
    </IconContext.Provider>
  );
}

export function Button({
  children,
  icon: Glyph,
  variant = "primary",
  className = "",
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  icon?: Icon;
  variant?: "primary" | "secondary" | "ghost" | "danger";
}) {
  return (
    <button className={`button button-${variant} ${className}`} {...props}>
      {Glyph ? <Glyph size={18} aria-hidden="true" /> : null}
      {children}
    </button>
  );
}

export function Modal({
  open,
  onOpenChange,
  title,
  description,
  children,
  wide = false,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  children: ReactNode;
  wide?: boolean;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="modal-overlay" />
        <Dialog.Content className={`modal-content ${wide ? "modal-wide" : ""}`}>
          <div className="modal-heading">
            <div>
              <Dialog.Title>{title}</Dialog.Title>
              <Dialog.Description>{description}</Dialog.Description>
            </div>
            <Dialog.Close className="icon-button" aria-label="Tutup dialog">
              <XIcon />
            </Dialog.Close>
          </div>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export function StatusBadge({ status }: { status: PaymentStatus | string }) {
  const variant = ["CAPTURED", "REFUNDED"].includes(status)
    ? "success"
    : ["FAILED", "CANCELED", "EXPIRED"].includes(status)
      ? "error"
      : status === "CREATED"
        ? "neutral"
        : "pending";
  return (
    <span className={`status-badge status-${variant}`}>
      <span className="status-dot" />
      {STATUS_LABELS[status as PaymentStatus] || status}
    </span>
  );
}

export function ChannelMark({
  channel,
  small = false,
}: {
  channel?: Channel;
  small?: boolean;
}) {
  return (
    <span
      aria-hidden="true"
      className={`channel-mark ${small ? "channel-mark-small" : ""}`}
      style={
        {
          "--channel-color": channel?.color || "#70817a",
        } as React.CSSProperties
      }
    >
      {channel?.initials || "·"}
    </span>
  );
}

export function InlineError({ message }: { message: string }) {
  return (
    <div role="alert" className="inline-error">
      <WarningCircleIcon size={19} />
      <span>{message}</span>
    </div>
  );
}

export function EmptyState({
  icon: Glyph,
  title,
  description,
  children,
  compact = false,
}: {
  icon: Icon;
  title: string;
  description: string;
  children?: ReactNode;
  compact?: boolean;
}) {
  return (
    <div className={`empty-state ${compact ? "empty-compact" : ""}`}>
      <span className="empty-icon">
        <Glyph size={27} />
      </span>
      <h3>{title}</h3>
      <p>{description}</p>
      {children}
    </div>
  );
}

export function Toast({
  toast,
  dismiss,
}: {
  toast: ToastMessage | null;
  dismiss: () => void;
}) {
  if (!toast) return null;
  const Glyph =
    toast.kind === "success"
      ? CheckCircleIcon
      : toast.kind === "error"
        ? WarningCircleIcon
        : InfoIcon;
  return (
    <div
      role={toast.kind === "error" ? "alert" : "status"}
      className={`toast toast-${toast.kind}`}
    >
      <Glyph size={23} />
      <span>{toast.message}</span>
      <button
        className="icon-button"
        onClick={dismiss}
        aria-label="Tutup notifikasi"
      >
        <XIcon size={17} />
      </button>
    </div>
  );
}

export function Loader({ label = "Memuat…" }: { label?: string }) {
  return (
    <span className="loader-wrap" role="status">
      <span className="spinner" aria-hidden="true" />
      {label}
    </span>
  );
}
