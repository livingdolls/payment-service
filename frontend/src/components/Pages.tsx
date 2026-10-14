import { useEffect, useState } from "react";
import { ArrowRightIcon } from "@phosphor-icons/react/dist/csr/ArrowRight";
import { ArrowsClockwiseIcon } from "@phosphor-icons/react/dist/csr/ArrowsClockwise";
import { ArrowCounterClockwiseIcon } from "@phosphor-icons/react/dist/csr/ArrowCounterClockwise";
import { CheckCircleIcon } from "@phosphor-icons/react/dist/csr/CheckCircle";
import { DownloadSimpleIcon } from "@phosphor-icons/react/dist/csr/DownloadSimple";
import { FunnelSimpleIcon } from "@phosphor-icons/react/dist/csr/FunnelSimple";
import { MagnifyingGlassIcon } from "@phosphor-icons/react/dist/csr/MagnifyingGlass";
import { ReceiptIcon } from "@phosphor-icons/react/dist/csr/Receipt";
import { ShieldCheckIcon } from "@phosphor-icons/react/dist/csr/ShieldCheck";
import { SquaresFourIcon } from "@phosphor-icons/react/dist/csr/SquaresFour";
import { PlugsConnectedIcon } from "@phosphor-icons/react/dist/csr/PlugsConnected";
import { WarningCircleIcon } from "@phosphor-icons/react/dist/csr/WarningCircle";
import { api, errorMessage } from "../lib/api";
import {
  STATUS_LABELS,
  count,
  downloadCsv,
  filterPayments,
  money,
} from "../lib/payments";
import { channels } from "../data/channels";
import { demoReviews } from "../data/demo";
import type {
  ConsoleConfig,
  PaymentRecord,
  ReviewItem,
  ToastMessage,
} from "../types";
import {
  Button,
  ChannelMark,
  EmptyState,
  InlineError,
  Loader,
  Modal,
  StatusBadge,
} from "./ui";
import { PaymentTable } from "./PaymentTable";

export function PaymentsPage({
  payments,
  onSelect,
  onFind,
  notify,
}: {
  payments: PaymentRecord[];
  onSelect: (payment: PaymentRecord) => void;
  onFind: () => void;
  notify: (toast: ToastMessage) => void;
}) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("all");
  const [page, setPage] = useState(1);
  const filtered = filterPayments(payments, query, status, null);
  const pageSize = 10;
  const pages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const currentPage = Math.min(page, pages);
  const visible = filtered.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );
  function exportCsv() {
    downloadCsv(filtered);
    notify({
      message: `${count(filtered.length)} pembayaran diekspor ke CSV.`,
      kind: "success",
    });
  }
  return (
    <section className="panel payments-panel">
      <div className="payments-toolbar">
        <div className="search-input">
          <MagnifyingGlassIcon size={18} />
          <input
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setPage(1);
            }}
            placeholder="Cari pesanan, referensi, atau metode…"
            aria-label="Cari riwayat pembayaran"
          />
        </div>
        <div className="table-controls">
          <div className="select-with-icon">
            <FunnelSimpleIcon size={17} />
            <select
              value={status}
              onChange={(event) => {
                setStatus(event.target.value);
                setPage(1);
              }}
              aria-label="Filter status"
            >
              <option value="all">Semua status</option>
              <option value="pending">Menunggu pembayaran</option>
              <option value="CAPTURED">Berhasil</option>
              <option value="failed">Gagal / dibatalkan</option>
              <option value="REFUNDED">Dikembalikan</option>
            </select>
          </div>
          <Button
            variant="secondary"
            icon={DownloadSimpleIcon}
            onClick={exportCsv}
            disabled={!filtered.length}
          >
            Ekspor CSV
          </Button>
        </div>
      </div>
      <div className="status-tabs" aria-label="Status pembayaran">
        {[
          ["all", "Semua"],
          ["CAPTURED", "Berhasil"],
          ["pending", "Menunggu"],
          ["failed", "Gagal"],
        ].map(([value, label]) => (
          <button
            className={status === value ? "active" : ""}
            aria-pressed={status === value}
            key={value}
            onClick={() => {
              setStatus(value);
              setPage(1);
            }}
          >
            {label}
            <span>
              {count(filterPayments(payments, "", value, null).length)}
            </span>
          </button>
        ))}
      </div>
      {visible.length ? (
        <PaymentTable payments={visible} onSelect={onSelect} />
      ) : (
        <EmptyState
          icon={ReceiptIcon}
          title={
            payments.length
              ? "Tidak ada pembayaran yang cocok"
              : "Riwayat pembayaran masih kosong"
          }
          description={
            payments.length
              ? "Coba kata kunci atau filter status yang berbeda."
              : "Pembayaran yang dibuat atau dicari akan muncul di sini."
          }
        >
          {payments.length ? (
            <Button
              variant="secondary"
              onClick={() => {
                setQuery("");
                setStatus("all");
              }}
            >
              Hapus filter
            </Button>
          ) : (
            <Button
              variant="secondary"
              icon={MagnifyingGlassIcon}
              onClick={onFind}
            >
              Cari pembayaran di API
            </Button>
          )}
        </EmptyState>
      )}
      <div className="pagination">
        <span>
          Menampilkan {filtered.length ? (currentPage - 1) * pageSize + 1 : 0}–
          {Math.min(currentPage * pageSize, filtered.length)} dari{" "}
          {count(filtered.length)} pembayaran
        </span>
        <div>
          <Button
            variant="secondary"
            onClick={() => setPage(currentPage - 1)}
            disabled={currentPage === 1}
          >
            Sebelumnya
          </Button>
          <span>
            {currentPage} / {pages}
          </span>
          <Button
            variant="secondary"
            onClick={() => setPage(currentPage + 1)}
            disabled={currentPage === pages}
          >
            Berikutnya
          </Button>
        </div>
      </div>
      <div className="history-note">
        <ShieldCheckIcon size={17} />
        <span>
          Riwayat browser ini, maksimal 500 pembayaran. Cari ID untuk mengambil
          pembayaran dari API.
        </span>
      </div>
    </section>
  );
}

export function ChannelsPage() {
  const [category, setCategory] = useState("Semua");
  const [includeUnavailable, setIncludeUnavailable] = useState(false);
  const categories = [
    "Semua",
    "QR code",
    "Virtual account",
    "E-wallet",
    "Retail",
    "Kartu",
    "Lainnya",
  ];
  const filtered = channels.filter(
    (channel) =>
      (category === "Semua" || channel.category === category) &&
      (includeUnavailable || channel.available),
  );
  return (
    <>
      <div className="channels-intro">
        <div className="intro-icon">
          <SquaresFourIcon size={25} />
        </div>
        <div>
          <strong>
            {channels.filter((channel) => channel.available).length} profil
            metode pembayaran
          </strong>
          <p>
            Katalog IDR proyek. Ketersediaan transaksi mengikuti aktivasi
            merchant Xendit Anda.
          </p>
        </div>
      </div>
      <div className="channels-filters">
        <div className="category-tabs" aria-label="Kategori metode pembayaran">
          {categories.map((value) => (
            <button
              key={value}
              className={category === value ? "active" : ""}
              aria-pressed={category === value}
              onClick={() => setCategory(value)}
            >
              {value}
            </button>
          ))}
        </div>
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={includeUnavailable}
            onChange={(event) => setIncludeUnavailable(event.target.checked)}
          />
          Tampilkan profil belum tersedia
        </label>
      </div>
      <div className="channels-grid">
        {filtered.map((channel) => (
          <article
            className={`channel-card ${!channel.available ? "channel-unavailable" : ""}`}
            key={channel.code}
          >
            <div className="channel-card-heading">
              <ChannelMark channel={channel} />
              <span
                className={`profile-badge ${channel.available ? "" : "profile-unavailable"}`}
              >
                {channel.available ? "Profil tersedia" : "Belum tersedia"}
              </span>
            </div>
            <h2>{channel.name}</h2>
            <p>{channel.category}</p>
            <code>{channel.code}</code>
            {channel.note ? (
              <p className="channel-note">{channel.note}</p>
            ) : (
              <p className="channel-note">
                {channel.properties === "name"
                  ? "Menggunakan nama pembayar."
                  : channel.properties === "phone"
                    ? "Menggunakan nomor ponsel OVO."
                    : channel.properties === "cashtag"
                      ? "Menggunakan cashtag Jenius."
                      : channel.properties === "return_url"
                        ? "Menggunakan URL kembali checkout."
                        : "Instruksi QR dibuat oleh provider."}
              </p>
            )}
          </article>
        ))}
      </div>
      {!filtered.length ? (
        <EmptyState
          icon={SquaresFourIcon}
          title="Profil belum tersedia"
          description="Aktifkan opsi profil belum tersedia untuk melihat katalog kategori ini."
        />
      ) : null}
      <div className="page-source-note">
        Sumber: katalog channel IDR payment-service, ditinjau 9 Oktober 2026.
      </div>
    </>
  );
}

export function ReviewsPage({
  demo,
  payments,
  notify,
}: {
  demo: boolean;
  payments: PaymentRecord[];
  notify: (toast: ToastMessage) => void;
}) {
  const [reviews, setReviews] = useState<ReviewItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<ReviewItem | null>(null);
  const [reason, setReason] = useState("");
  const [requeueError, setRequeueError] = useState("");
  const [busy, setBusy] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    const pending = demo
      ? Promise.resolve({ data: demoReviews(payments) })
      : api.reviews();
    pending
      .then((response) => {
        if (!cancelled) setReviews(response.data || []);
      })
      .catch((error) => {
        if (!cancelled) setError(errorMessage(error));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [demo, payments, reloadKey]);

  async function requeue(event: React.SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selected || busy) return;
    if (
      Array.from(reason.trim()).length < 10 ||
      Array.from(reason.trim()).length > 500
    )
      return setRequeueError("Alasan peninjauan harus berisi 10–500 karakter.");
    setBusy(true);
    setRequeueError("");
    try {
      if (!demo) await api.requeue(selected.attempt_id, reason.trim());
      setReviews((items) =>
        items.filter((item) => item.attempt_id !== selected.attempt_id),
      );
      setSelected(null);
      setReason("");
      notify({
        message: demo
          ? "Antrean contoh diperbarui."
          : "Pembayaran dikembalikan ke antrean rekonsiliasi.",
        kind: "success",
      });
    } catch (error) {
      setRequeueError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="review-info">
        <span className="intro-icon">
          <ShieldCheckIcon size={25} />
        </span>
        <div>
          <strong>Periksa pembayaran yang membutuhkan perhatian</strong>
          <p>
            Rekonsiliasi memeriksa kembali status provider. Setiap tindakan
            dicatat bersama alasan dan identitas admin.
          </p>
        </div>
      </div>
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>
              Antrean peninjauan{" "}
              <span className="count-badge">
                {loading ? "…" : reviews.length}
              </span>
            </h2>
            <p>Maksimal 50 pembayaran yang membutuhkan review</p>
          </div>
          <Button
            variant="secondary"
            icon={ArrowsClockwiseIcon}
            onClick={() => setReloadKey((value) => value + 1)}
            disabled={loading}
          >
            Perbarui
          </Button>
        </div>
        {loading ? (
          <div className="page-loader">
            <Loader label="Memuat antrean rekonsiliasi…" />
          </div>
        ) : error ? (
          <div className="review-error">
            <InlineError message={error} />
            <p className="muted">
              Pastikan ADMIN_API_TOKEN di konfigurasi server telah diisi.
            </p>
          </div>
        ) : reviews.length ? (
          <div className="review-list">
            {reviews.map((review) => (
              <article className="review-item" key={review.attempt_id}>
                <div className="review-item-top">
                  <div>
                    <strong>{review.order_id}</strong>
                    <small>{review.reference_id}</small>
                  </div>
                  <StatusBadge status={review.payment_status} />
                </div>
                <div className="review-item-body">
                  <div>
                    <span>Nominal</span>
                    <strong>{money(review.amount)}</strong>
                  </div>
                  <div>
                    <span>Percobaan rekonsiliasi</span>
                    <strong>{review.reconcile_attempts} kali</strong>
                  </div>
                  <div>
                    <span>Kegagalan</span>
                    <strong>{review.reconcile_failures} kali</strong>
                  </div>
                  <div>
                    <span>Status attempt</span>
                    <strong>{review.attempt_status}</strong>
                  </div>
                </div>
                <div className="review-item-bottom">
                  <p>
                    <WarningCircleIcon size={17} />
                    {review.last_reconcile_error ||
                      "Status pembayaran perlu diperiksa kembali."}
                  </p>
                  <Button
                    variant="secondary"
                    icon={ArrowCounterClockwiseIcon}
                    onClick={() => {
                      setSelected(review);
                      setReason("");
                      setRequeueError("");
                    }}
                  >
                    Antrekan ulang
                  </Button>
                </div>
              </article>
            ))}
          </div>
        ) : (
          <EmptyState
            icon={CheckCircleIcon}
            title="Tidak ada pembayaran yang perlu ditinjau"
            description="Pembayaran yang membutuhkan pemeriksaan manual akan muncul di halaman ini."
          />
        )}
      </section>
      <Modal
        open={!!selected}
        onOpenChange={(open) => {
          if (!busy && !open) setSelected(null);
        }}
        title="Antrekan ulang rekonsiliasi"
        description={`Periksa kembali status provider untuk pesanan ${selected?.order_id || ""}.`}
      >
        <form onSubmit={requeue} className="form-body">
          <label className="field-label" htmlFor="requeue-reason">
            Alasan peninjauan ulang
          </label>
          <textarea
            id="requeue-reason"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder="Jelaskan alasan pembayaran perlu diperiksa lagi"
            rows={4}
            minLength={10}
            maxLength={500}
            required
            disabled={busy}
          />
          <p className="field-hint">
            Gunakan 10–500 karakter. Alasan tersimpan dalam audit rekonsiliasi.
          </p>
          {requeueError ? <InlineError message={requeueError} /> : null}
          <div className="modal-actions">
            <Button
              type="button"
              variant="secondary"
              disabled={busy}
              onClick={() => setSelected(null)}
            >
              Batal
            </Button>
            <Button
              type="submit"
              icon={ArrowCounterClockwiseIcon}
              disabled={busy}
            >
              {busy ? "Mengantrekan…" : "Antrekan ulang"}
            </Button>
          </div>
        </form>
      </Modal>
    </>
  );
}

export function SettingsPage({
  config,
  health,
  onReconnect,
  demo,
  onDemo,
  storageAvailable,
}: {
  config: ConsoleConfig | null;
  health: "loading" | "online" | "offline";
  onReconnect: () => void;
  demo: boolean;
  onDemo: () => void;
  storageAvailable: boolean;
}) {
  return (
    <div className="settings-stack">
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>Koneksi layanan</h2>
            <p>Konfigurasi koneksi dari server frontend lokal</p>
          </div>
          <Button
            variant="secondary"
            icon={ArrowsClockwiseIcon}
            onClick={onReconnect}
            disabled={health === "loading"}
          >
            Cek koneksi
          </Button>
        </div>
        <div className="settings-rows">
          <div>
            <span>Payment API</span>
            <code>{config?.apiUrl || "Konfigurasi proxy /api"}</code>
          </div>
          <div>
            <span>Koneksi database</span>
            <span className={`connection-inline ${health}`}>
              <i />
              {health === "online"
                ? "Terhubung"
                : health === "loading"
                  ? "Memeriksa…"
                  : "Belum terhubung"}
            </span>
          </div>
          <div>
            <span>Lingkungan backend</span>
            <strong>{config?.environment || "Belum tersedia"}</strong>
          </div>
          <div>
            <span>Token layanan internal</span>
            <strong
              className={
                config?.internalConfigured ? "text-success" : "text-warning"
              }
            >
              {config?.internalConfigured
                ? "Dikonfigurasi"
                : "Perlu dikonfigurasi"}
            </strong>
          </div>
          <div>
            <span>Token admin</span>
            <strong
              className={
                config?.adminConfigured ? "text-success" : "text-warning"
              }
            >
              {config?.adminConfigured
                ? "Dikonfigurasi"
                : "Perlu dikonfigurasi"}
            </strong>
          </div>
        </div>
        <div className="settings-hint">
          <PlugsConnectedIcon size={21} />
          <p>
            Frontend membaca <code>PAYMENT_SERVICE_URL</code>,{" "}
            <code>PAYMENT_INTERNAL_TOKEN</code>, dan{" "}
            <code>ADMIN_API_TOKEN</code> dari <code>.env</code> proyek. Restart
            frontend setelah mengubah konfigurasi.
          </p>
        </div>
      </section>
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>Riwayat & tampilan demo</h2>
            <p>Data pembayaran untuk browser ini</p>
          </div>
        </div>
        <div className="settings-rows">
          <div>
            <span>Penyimpanan riwayat</span>
            <strong>
              {storageAvailable
                ? "Lokal di browser, maksimal 500 item"
                : "Sesi ini saja; penyimpanan tidak tersedia"}
            </strong>
          </div>
          <div>
            <span>Mode data</span>
            <Button variant="secondary" onClick={onDemo}>
              {demo ? "Kembali ke data API" : "Lihat data demo"}
            </Button>
          </div>
        </div>
        <div className="settings-hint">
          <ShieldCheckIcon size={21} />
          <p>
            Token server, properti channel, dan instruksi pembayaran provider
            tidak disimpan dalam riwayat browser. Mode demo menggunakan data
            contoh terpisah.
          </p>
        </div>
      </section>
    </div>
  );
}

export function GuidePage({
  onNew,
  onFind,
}: {
  onNew: () => void;
  onFind: () => void;
}) {
  const steps = [
    {
      title: "Buat pembayaran",
      description:
        "Masukkan ID pesanan unik dan nominal Rupiah. Dashboard membuat payment intent dan attempt dengan kunci idempotensi.",
      icon: ReceiptIcon,
    },
    {
      title: "Pilih metode pembayaran",
      description:
        "Buka detail pembayaran, pilih QRIS, virtual account, e-wallet, retail, atau kartu. Lengkapi properti yang diperlukan metode tersebut.",
      icon: SquaresFourIcon,
    },
    {
      title: "Ikuti instruksi & pantau status",
      description:
        "Scan QR, transfer ke virtual account, atau buka halaman provider. Status diperbarui otomatis saat detail pembayaran terbuka.",
      icon: CheckCircleIcon,
    },
  ];
  return (
    <div className="guide-stack">
      <section className="panel guide-intro">
        <span className="intro-icon">
          <ReceiptIcon size={28} />
        </span>
        <h2>Dari pesanan sampai pembayaran diterima</h2>
        <p>
          Gunakan dashboard ini untuk menjalankan alur payment-service dan
          meninjau pembayaran yang memerlukan perhatian.
        </p>
        <div className="guide-actions">
          <Button onClick={onNew} icon={ArrowRightIcon}>
            Buat pembayaran
          </Button>
          <Button
            variant="secondary"
            onClick={onFind}
            icon={MagnifyingGlassIcon}
          >
            Cari pembayaran
          </Button>
        </div>
      </section>
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>Alur pembayaran</h2>
            <p>Tiga langkah untuk memulai</p>
          </div>
        </div>
        <div className="guide-steps">
          {steps.map((step, index) => (
            <article key={step.title}>
              <div className="guide-step-icon">
                <step.icon size={24} />
              </div>
              <div>
                <small>Langkah {index + 1}</small>
                <h3>{step.title}</h3>
                <p>{step.description}</p>
              </div>
            </article>
          ))}
        </div>
      </section>
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>Memahami status pembayaran</h2>
            <p>
              Status dikirim oleh API, webhook provider, dan worker rekonsiliasi
            </p>
          </div>
        </div>
        <div className="status-guide">
          {Object.entries(STATUS_LABELS).map(([status, label]) => (
            <div key={status}>
              <StatusBadge status={status} />
              <code>{status}</code>
              <span>{label}</span>
            </div>
          ))}
        </div>
      </section>
      <div className="guide-note">
        <ShieldCheckIcon size={21} />
        <p>
          Webhook diproses oleh backend. Pembayaran dengan hasil belum pasti
          akan diperiksa worker rekonsiliasi sebelum status akhirnya
          dikonfirmasi.
        </p>
      </div>
    </div>
  );
}
