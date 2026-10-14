import { useEffect, useRef, useState } from "react";
import { ArrowRightIcon } from "@phosphor-icons/react/dist/csr/ArrowRight";
import { ArrowSquareOutIcon } from "@phosphor-icons/react/dist/csr/ArrowSquareOut";
import { ArrowsClockwiseIcon } from "@phosphor-icons/react/dist/csr/ArrowsClockwise";
import { CopyIcon } from "@phosphor-icons/react/dist/csr/Copy";
import { CheckCircleIcon } from "@phosphor-icons/react/dist/csr/CheckCircle";
import { MagnifyingGlassIcon } from "@phosphor-icons/react/dist/csr/MagnifyingGlass";
import { QrCodeIcon } from "@phosphor-icons/react/dist/csr/QrCode";
import { ReceiptIcon } from "@phosphor-icons/react/dist/csr/Receipt";
import { api, errorMessage } from "../lib/api";
import {
  ACTIVE_STATUSES,
  formatDate,
  fromCreated,
  fromProcessed,
  money,
  safeActionUrl,
} from "../lib/payments";
import {
  buildProperties,
  channelByCode,
  channels,
  type Channel,
} from "../data/channels";
import type { PaymentAction, PaymentRecord, ToastMessage } from "../types";
import {
  Button,
  ChannelMark,
  InlineError,
  Loader,
  Modal,
  StatusBadge,
} from "./ui";

type Notify = (toast: ToastMessage) => void;

function idempotencyKey(body: {
  order_id: string;
  amount: number;
  currency: string;
}): string {
  const fingerprint = JSON.stringify(body);
  try {
    const previous = JSON.parse(
      sessionStorage.getItem("payment-console:create-draft:v1") || "null",
    );
    if (
      previous?.fingerprint === fingerprint &&
      typeof previous.key === "string"
    )
      return previous.key;
  } catch {
    /* An unavailable browser store still permits a request. */
  }
  const key = crypto.randomUUID();
  try {
    sessionStorage.setItem(
      "payment-console:create-draft:v1",
      JSON.stringify({ fingerprint, key }),
    );
  } catch {
    /* Fall back to the current form's request key. */
  }
  return key;
}

export function CreatePaymentModal({
  open,
  onOpenChange,
  onCreated,
  demo,
  notify,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (payment: PaymentRecord) => void;
  demo: boolean;
  notify: Notify;
}) {
  const [orderId, setOrderId] = useState("");
  const [amount, setAmount] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const pending = useRef<{ fingerprint: string; key: string } | null>(null);

  async function submit(event: React.SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setError("");
    const value = Number(amount);
    if (!orderId.trim()) return setError("Isi ID pesanan terlebih dahulu.");
    if (!Number.isSafeInteger(value) || value <= 0)
      return setError(
        "Nominal harus berupa bilangan bulat Rupiah lebih dari nol.",
      );
    const body = { order_id: orderId.trim(), amount: value, currency: "IDR" };
    const fingerprint = JSON.stringify(body);
    if (pending.current?.fingerprint !== fingerprint)
      pending.current = {
        fingerprint,
        key: demo ? crypto.randomUUID() : idempotencyKey(body),
      };
    setBusy(true);
    try {
      const payment = demo
        ? {
            id: crypto.randomUUID(),
            attempt_id: crypto.randomUUID(),
            reference_id: `DEMO-${Date.now().toString().slice(-7)}`,
            order_id: body.order_id,
            amount: body.amount,
            currency: "IDR",
            status: "CREATED" as const,
            attempt_status: "CREATED",
            captured_amount: 0,
            refunded_amount: 0,
            created_at: new Date().toISOString(),
            created_at_known: true,
            updated_at: new Date().toISOString(),
            actions: [],
          }
        : fromCreated(await api.create(body, pending.current!.key));
      onCreated({ ...payment, description: description.trim() });
      setOrderId("");
      setAmount("");
      setDescription("");
      pending.current = null;
      if (!demo)
        try {
          sessionStorage.removeItem("payment-console:create-draft:v1");
        } catch {
          /* No persistent draft to clear. */
        }
      notify({
        message: demo
          ? "Pembayaran demo dibuat."
          : "Pembayaran berhasil dibuat. Pilih metode untuk melanjutkan.",
        kind: "success",
      });
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      onOpenChange={(value) => {
        if (!busy) {
          setError("");
          onOpenChange(value);
        }
      }}
      title="Buat pembayaran"
      description="Mulai dari pesanan, lalu pilih metode pembayaran."
    >
      <form onSubmit={submit} className="form-body">
        {demo ? (
          <div className="form-notice">
            Mode demo: formulir ini tidak mengirim permintaan ke API.
          </div>
        ) : null}
        <label className="field-label" htmlFor="new-order">
          ID pesanan<span>*</span>
        </label>
        <input
          id="new-order"
          name="order_id"
          value={orderId}
          onChange={(event) => setOrderId(event.target.value)}
          placeholder="Contoh: ORD-20261014-001"
          maxLength={255}
          required
          autoComplete="off"
          disabled={busy}
        />
        <p className="field-hint">
          Gunakan ID pesanan yang unik dari sistem Anda.
        </p>
        <label className="field-label" htmlFor="new-amount">
          Nominal pembayaran<span>*</span>
        </label>
        <div className="amount-input">
          <span>Rp</span>
          <input
            id="new-amount"
            name="amount"
            type="number"
            inputMode="numeric"
            min="1"
            step="1"
            max={Number.MAX_SAFE_INTEGER}
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
            placeholder="0"
            required
            disabled={busy}
          />
          <span>IDR</span>
        </div>
        <label className="field-label" htmlFor="new-description">
          Catatan <small>opsional</small>
        </label>
        <textarea
          id="new-description"
          rows={2}
          value={description}
          onChange={(event) => setDescription(event.target.value)}
          placeholder="Tambahkan keterangan pembayaran"
          maxLength={512}
          disabled={busy}
        />
        <p className="field-hint">
          Catatan akan dikirim saat metode pembayaran diproses.
        </p>
        <div className="payment-total">
          <span>Total pembayaran</span>
          <strong>{money(Number(amount) || 0)}</strong>
        </div>
        {error ? <InlineError message={error} /> : null}
        <div className="modal-actions">
          <Button
            type="button"
            variant="secondary"
            onClick={() => onOpenChange(false)}
            disabled={busy}
          >
            Batal
          </Button>
          <Button
            type="submit"
            icon={busy ? undefined : ArrowRightIcon}
            disabled={busy}
          >
            {busy ? <Loader label="Membuat pembayaran…" /> : "Buat pembayaran"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

export function FindPaymentModal({
  open,
  onOpenChange,
  onFind,
  demo,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onFind: (id: string) => Promise<void>;
  demo: boolean;
}) {
  const [id, setId] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(event: React.SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const value = id.trim();
    if (
      !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
        value,
      )
    )
      return setError("ID pembayaran harus berupa UUID yang valid.");
    setError("");
    setBusy(true);
    try {
      await onFind(value);
      setId("");
      onOpenChange(false);
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal
      open={open}
      onOpenChange={(value) => {
        if (!busy) {
          setError("");
          onOpenChange(value);
        }
      }}
      title="Cari pembayaran"
      description="Ambil pembayaran yang sudah ada menggunakan ID pembayaran."
    >
      <form className="form-body" onSubmit={submit}>
        {demo ? (
          <div className="form-notice">
            Pencarian menggunakan riwayat contoh selama mode demo.
          </div>
        ) : null}
        <label className="field-label" htmlFor="find-id">
          ID pembayaran
        </label>
        <input
          id="find-id"
          value={id}
          onChange={(event) => setId(event.target.value)}
          placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
          required
          autoComplete="off"
          disabled={busy}
        />
        <p className="field-hint">
          Gunakan ID pembayaran, bukan ID pesanan atau ID attempt.
        </p>
        {error ? <InlineError message={error} /> : null}
        <div className="modal-actions">
          <Button type="submit" icon={MagnifyingGlassIcon} disabled={busy}>
            {busy ? <Loader label="Mencari…" /> : "Cari pembayaran"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function QrPayment({ value }: { value: string }) {
  const [dataUrl, setDataUrl] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    let cancelled = false;
    setDataUrl("");
    setError("");
    import("qrcode")
      .then((module) =>
        module.toDataURL(value, {
          width: 280,
          margin: 2,
          color: { dark: "#142b28", light: "#ffffff" },
        }),
      )
      .then((result) => {
        if (!cancelled) setDataUrl(result);
      })
      .catch(() => {
        if (!cancelled)
          setError(
            "QR tidak dapat ditampilkan. Salin kode QR dari instruksi di bawah.",
          );
      });
    return () => {
      cancelled = true;
    };
  }, [value]);
  return (
    <div className="qr-payment">
      {dataUrl ? (
        <img
          src={dataUrl}
          width={240}
          height={240}
          alt="Kode QRIS pembayaran"
        />
      ) : error ? (
        <InlineError message={error} />
      ) : (
        <Loader label="Menyiapkan QRIS…" />
      )}
      <strong>Scan untuk membayar</strong>
      <p>Buka aplikasi bank atau e-wallet, lalu scan QR ini.</p>
    </div>
  );
}

function ActionInstructions({
  actions,
  notify,
}: {
  actions: PaymentAction[];
  notify: Notify;
}) {
  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      notify({ message: "Instruksi pembayaran disalin.", kind: "success" });
    } catch {
      notify({
        message:
          "Tidak dapat menyalin. Pilih dan salin instruksi secara manual.",
        kind: "error",
      });
    }
  }
  return (
    <div className="payment-instructions">
      {actions.map((action, index) => {
        if (!action.value) return null;
        const url =
          action.descriptor === "WEB_URL" ? safeActionUrl(action.value) : null;
        return (
          <div className="instruction" key={`${action.descriptor}-${index}`}>
            {action.descriptor === "QR_STRING" ? (
              <QrPayment value={action.value} />
            ) : null}
            {url ? (
              <Button
                icon={ArrowSquareOutIcon}
                variant="secondary"
                type="button"
                onClick={() =>
                  window.open(url, "_blank", "noopener,noreferrer")
                }
              >
                Lanjutkan ke halaman pembayaran
              </Button>
            ) : (
              <div className="instruction-value">
                <div>
                  <small>
                    {action.descriptor === "VIRTUAL_ACCOUNT_NUMBER"
                      ? "Nomor virtual account"
                      : action.descriptor === "PAYMENT_CODE"
                        ? "Kode pembayaran"
                        : action.descriptor === "QR_STRING"
                          ? "Kode QR"
                          : action.descriptor
                              .replaceAll("_", " ")
                              .toLowerCase()}
                  </small>
                  <code>{action.value}</code>
                </div>
                <button
                  className="icon-button"
                  onClick={() => copy(action.value)}
                  aria-label={`Salin ${action.descriptor}`}
                >
                  <CopyIcon size={19} />
                </button>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

function ChannelForm({
  payment,
  onUpdated,
  onRefresh,
  notify,
  demo,
}: {
  payment: PaymentRecord;
  onUpdated: (payment: PaymentRecord) => void;
  onRefresh: (id: string) => Promise<void>;
  notify: Notify;
  demo: boolean;
}) {
  const [code, setCode] = useState("QRIS");
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [cashtag, setCashtag] = useState("");
  const [returnUrl, setReturnUrl] = useState("");
  const [advancedEnabled, setAdvancedEnabled] = useState(false);
  const [advanced, setAdvanced] = useState("{}");
  const [description, setDescription] = useState(payment.description || "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const channel = channelByCode(code)!;
  const amountError =
    channel.minimumAmount !== undefined &&
    payment.amount < channel.minimumAmount
      ? `${channel.name} Virtual Account membutuhkan nominal minimal ${money(channel.minimumAmount)}. Nominal pembayaran ini ${money(payment.amount)}. Buat pembayaran baru dengan nominal yang memenuhi batas tersebut.`
      : "";

  function changeChannel(next: Channel) {
    setCode(next.code);
    setAdvanced("{}");
    setAdvancedEnabled(false);
    setError("");
  }
  async function submit(event: React.SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    if (amountError) return;
    setError("");
    let properties: Record<string, unknown>;
    try {
      properties = buildProperties(channel, {
        name,
        phone,
        cashtag,
        returnUrl,
        advanced,
        advancedEnabled,
      });
    } catch (error) {
      return setError(errorMessage(error));
    }
    setBusy(true);
    try {
      if (demo) {
        onUpdated({
          ...payment,
          status: "REQUIRES_ACTION",
          attempt_status: "REQUIRES_ACTION",
          channel_code: code,
          updated_at: new Date().toISOString(),
        });
        notify({
          message:
            "Metode demo dipilih. Tidak ada pembayaran yang dikirim ke provider.",
          kind: "info",
        });
      } else {
        const response = await api.process(payment.attempt_id, {
          country: "ID",
          channel_code: code,
          channel_properties: properties,
          description: description.trim(),
        });
        onUpdated({
          ...fromProcessed(payment, response, code),
          description: description.trim(),
        });
        notify({
          message:
            "code" in response
              ? "Status belum final. Sistem sedang melakukan rekonsiliasi; periksa status sebelum mengulangi."
              : "Metode berhasil diproses. Ikuti instruksi pembayaran.",
          kind: "info",
        });
        if (!("code" in response) && response.payment.status === "CAPTURED") {
          try {
            await onRefresh(payment.id);
          } catch {
            notify({
              message:
                "Metode sudah diproses. Perbarui status untuk mengambil nominal diterima dari API.",
              kind: "info",
            });
          }
        }
      }
    } catch (error) {
      setError(
        `${errorMessage(error)} Periksa status sebelum mengulangi permintaan.`,
      );
      if (!demo) {
        notify({ message: errorMessage(error), kind: "error" });
        try {
          await onRefresh(payment.id);
        } catch {
          /* Keep the error and last known state until a status request succeeds. */
        }
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="channel-form" onSubmit={submit}>
      <div className="section-label">
        <h3>Pilih metode pembayaran</h3>
        <span>Indonesia · IDR</span>
      </div>
      <div className="popular-channels">
        {["QRIS", "BCA_VIRTUAL_ACCOUNT", "DANA", "GOPAY"].map((value) => {
          const option = channelByCode(value)!;
          return (
            <button
              key={value}
              type="button"
              className={`channel-option ${code === value ? "channel-selected" : ""}`}
              onClick={() => changeChannel(option)}
              disabled={busy}
              aria-pressed={code === value}
            >
              <ChannelMark channel={option} />
              <span>{option.name}</span>
              {code === value ? (
                <CheckCircleIcon size={15} weight="fill" />
              ) : null}
            </button>
          );
        })}
      </div>
      <label className="field-label" htmlFor="process-channel">
        Semua metode pembayaran
      </label>
      <select
        id="process-channel"
        value={code}
        onChange={(event) => changeChannel(channelByCode(event.target.value)!)}
        disabled={busy}
      >
        {channels
          .filter((item) => item.available)
          .map((item) => (
            <option value={item.code} key={item.code}>
              {item.name} · {item.category}
            </option>
          ))}
      </select>
      {!advancedEnabled && channel.properties === "name" ? (
        <>
          <label className="field-label" htmlFor="payer-name">
            Nama pembayar
          </label>
          <input
            id="payer-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            maxLength={100}
            placeholder="Nama sesuai rekening / identitas"
            disabled={busy}
          />
        </>
      ) : null}
      {!advancedEnabled && channel.properties === "phone" ? (
        <>
          <label className="field-label" htmlFor="payer-phone">
            Nomor ponsel OVO
          </label>
          <input
            id="payer-phone"
            type="tel"
            value={phone}
            onChange={(event) => setPhone(event.target.value)}
            pattern="\+62[0-9]{8,13}"
            required
            placeholder="+628123456789"
            disabled={busy}
          />
        </>
      ) : null}
      {!advancedEnabled && channel.properties === "cashtag" ? (
        <>
          <label className="field-label" htmlFor="payer-cashtag">
            Cashtag Jenius
          </label>
          <input
            id="payer-cashtag"
            value={cashtag}
            onChange={(event) => setCashtag(event.target.value)}
            pattern="\$[A-Za-z0-9_]+"
            required
            placeholder="$cashtag"
            disabled={busy}
          />
        </>
      ) : null}
      {!advancedEnabled && channel.properties === "return_url" ? (
        <>
          <label className="field-label" htmlFor="payer-return">
            URL kembali setelah pembayaran
          </label>
          <input
            id="payer-return"
            type="url"
            value={returnUrl}
            onChange={(event) => setReturnUrl(event.target.value)}
            required
            placeholder="https://toko-anda.id/pembayaran"
            disabled={busy}
          />
          <p className="field-hint">
            Halaman HTTPS toko Anda yang dibuka setelah checkout provider.
          </p>
        </>
      ) : null}
      <label className="field-label" htmlFor="process-description">
        Keterangan <small>opsional</small>
      </label>
      <input
        id="process-description"
        value={description}
        onChange={(event) => setDescription(event.target.value)}
        maxLength={512}
        placeholder="Keterangan untuk provider pembayaran"
        disabled={busy}
      />
      {channel.properties !== "card" ? (
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={advancedEnabled}
            onChange={(event) => setAdvancedEnabled(event.target.checked)}
            disabled={busy}
          />
          Isi properti channel secara manual
        </label>
      ) : null}
      {advancedEnabled || channel.properties === "card" ? (
        <>
          <label className="field-label" htmlFor="process-properties">
            Properti channel (JSON)
          </label>
          <textarea
            id="process-properties"
            className="code-input"
            rows={5}
            value={advanced}
            onChange={(event) => setAdvanced(event.target.value)}
            spellCheck={false}
            disabled={busy}
          />
          <p className="field-hint">
            Gunakan properti sesuai profil Xendit merchant. Properti tidak
            disimpan ke riwayat browser.
          </p>
        </>
      ) : null}
      {error ? <InlineError message={error} /> : null}
      {amountError ? <InlineError message={amountError} /> : null}
      <Button
        type="submit"
        className="full-width"
        disabled={busy || !!amountError}
        icon={QrCodeIcon}
      >
        {busy ? (
          <Loader label="Memproses metode…" />
        ) : (
          `Lanjutkan dengan ${channel.name}`
        )}
      </Button>
    </form>
  );
}

export function PaymentDetailModal({
  payment,
  onClose,
  onUpdated,
  onRefresh,
  notify,
  demo,
}: {
  payment: PaymentRecord | null;
  onClose: () => void;
  onUpdated: (payment: PaymentRecord) => void;
  onRefresh: (id: string) => Promise<void>;
  notify: Notify;
  demo: boolean;
}) {
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const pollBusy = useRef(false);
  const id = payment?.id;
  const hasActions = !!payment?.actions.length;
  const shouldPoll =
    !!payment &&
    !demo &&
    ACTIVE_STATUSES.has(payment.status) &&
    payment.status !== "CREATED";
  useEffect(() => {
    setError("");
  }, [id]);
  useEffect(() => {
    if (!shouldPoll || !id) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      if (cancelled) return;
      if (document.visibilityState === "visible" && !pollBusy.current) {
        pollBusy.current = true;
        try {
          await onRefresh(id!);
          if (!cancelled) setError("");
        } catch (error) {
          if (!cancelled) setError(errorMessage(error));
        } finally {
          pollBusy.current = false;
        }
      }
      if (!cancelled) timer = setTimeout(poll, 8000);
    }
    timer = setTimeout(poll, hasActions ? 8000 : 0);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [id, shouldPoll, hasActions, onRefresh]);

  async function refresh() {
    if (!payment || pollBusy.current) return;
    setRefreshing(true);
    setError("");
    pollBusy.current = true;
    try {
      await onRefresh(payment.id);
      notify({
        message: demo
          ? "Status contoh ditampilkan."
          : "Status pembayaran diperbarui.",
        kind: "success",
      });
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setRefreshing(false);
      pollBusy.current = false;
    }
  }
  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      notify({ message: "ID pembayaran disalin.", kind: "success" });
    } catch {
      notify({ message: "Tidak dapat menyalin ID pembayaran.", kind: "error" });
    }
  }

  return (
    <Modal
      open={!!payment}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title="Detail pembayaran"
      description="Informasi pembayaran dan instruksi untuk menyelesaikannya."
      wide
    >
      {payment ? (
        <div className="payment-detail">
          <div className="detail-summary">
            <span className="detail-receipt">
              <ReceiptIcon size={26} />
            </span>
            <StatusBadge status={payment.status} />
            <strong>{money(payment.amount)}</strong>
            <span>{payment.order_id}</span>
          </div>
          <dl className="detail-data">
            <div>
              <dt>ID pembayaran</dt>
              <dd className="copyable">
                <code>{payment.id}</code>
                <button
                  className="icon-button"
                  onClick={() => copy(payment.id)}
                  aria-label="Salin ID pembayaran"
                >
                  <CopyIcon size={16} />
                </button>
              </dd>
            </div>
            <div>
              <dt>Referensi</dt>
              <dd>{payment.reference_id}</dd>
            </div>
            <div>
              <dt>Attempt</dt>
              <dd>
                <code>{payment.attempt_id}</code>
              </dd>
            </div>
            <div>
              <dt>Status attempt</dt>
              <dd>{payment.attempt_status}</dd>
            </div>
            <div>
              <dt>
                {payment.created_at_known === false
                  ? "Ditambahkan di browser"
                  : "Dibuat"}
              </dt>
              <dd>{formatDate(payment.created_at)}</dd>
            </div>
            <div>
              <dt>Diperbarui</dt>
              <dd>{formatDate(payment.updated_at)}</dd>
            </div>
            {payment.channel_code ? (
              <div>
                <dt>Metode</dt>
                <dd>
                  {channelByCode(payment.channel_code)?.name ||
                    payment.channel_code}
                </dd>
              </div>
            ) : null}
            <div>
              <dt>Nominal diterima</dt>
              <dd>{money(payment.captured_amount)}</dd>
            </div>
          </dl>
          {error ? <InlineError message={error} /> : null}
          <div className="detail-refresh">
            <span>
              <i />
              {shouldPoll
                ? "Status diperbarui otomatis setiap 8 detik"
                : "Periksa status terbaru dari API"}
            </span>
            <Button
              onClick={refresh}
              variant="secondary"
              disabled={refreshing}
              icon={ArrowsClockwiseIcon}
            >
              {refreshing ? "Memperbarui…" : "Perbarui status"}
            </Button>
          </div>
          {payment.attempt_status === "CREATED" ? (
            <ChannelForm
              key={payment.attempt_id}
              payment={payment}
              onUpdated={onUpdated}
              onRefresh={onRefresh}
              notify={notify}
              demo={demo}
            />
          ) : payment.actions.length ? (
            <ActionInstructions actions={payment.actions} notify={notify} />
          ) : (
            <div
              className={`payment-state-notice ${payment.status === "CAPTURED" ? "payment-state-success" : ""}`}
            >
              {payment.status === "CAPTURED" ? (
                <CheckCircleIcon size={24} />
              ) : (
                <ReceiptIcon size={24} />
              )}
              <div>
                <strong>
                  {payment.status === "CAPTURED"
                    ? "Pembayaran berhasil diterima"
                    : payment.status === "REQUIRES_ACTION"
                      ? "Menunggu pembayaran pelanggan"
                      : payment.status === "PROCESSING"
                        ? "Pembayaran sedang diproses"
                        : "Status pembayaran tercatat"}
                </strong>
                <p>
                  {demo
                    ? "Ini contoh tampilan. Instruksi provider tersedia pada pembayaran melalui API."
                    : payment.status === "CAPTURED"
                      ? "Nominal telah dikonfirmasi oleh provider pembayaran."
                      : "Gunakan tombol perbarui status untuk mengambil perkembangan terakhir."}
                </p>
              </div>
            </div>
          )}
        </div>
      ) : null}
    </Modal>
  );
}
