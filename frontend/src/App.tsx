import { useCallback, useEffect, useRef, useState } from "react";
import { SquaresFourIcon } from "@phosphor-icons/react/dist/csr/SquaresFour";
import { ReceiptIcon } from "@phosphor-icons/react/dist/csr/Receipt";
import { CreditCardIcon } from "@phosphor-icons/react/dist/csr/CreditCard";
import { ArrowsClockwiseIcon } from "@phosphor-icons/react/dist/csr/ArrowsClockwise";
import { GearSixIcon } from "@phosphor-icons/react/dist/csr/GearSix";
import { BookOpenIcon } from "@phosphor-icons/react/dist/csr/BookOpen";
import { ArrowUpRightIcon } from "@phosphor-icons/react/dist/csr/ArrowUpRight";
import { PlusIcon } from "@phosphor-icons/react/dist/csr/Plus";
import { MagnifyingGlassIcon } from "@phosphor-icons/react/dist/csr/MagnifyingGlass";
import { ListIcon } from "@phosphor-icons/react/dist/csr/List";
import { XIcon } from "@phosphor-icons/react/dist/csr/X";
import { SunIcon } from "@phosphor-icons/react/dist/csr/Sun";
import { MoonIcon } from "@phosphor-icons/react/dist/csr/Moon";
import { CalendarBlankIcon } from "@phosphor-icons/react/dist/csr/CalendarBlank";
import { FlaskIcon } from "@phosphor-icons/react/dist/csr/Flask";
import { ArrowRightIcon } from "@phosphor-icons/react/dist/csr/ArrowRight";
import { WarningCircleIcon } from "@phosphor-icons/react/dist/csr/WarningCircle";
import { PlugsConnectedIcon } from "@phosphor-icons/react/dist/csr/PlugsConnected";
import { api } from "./lib/api";
import {
  filterPayments,
  fromSnapshot,
  persistHistory,
  readHistory,
  upsertPayment,
} from "./lib/payments";
import { demoPayments } from "./data/demo";
import { Button, IconsProvider, Toast } from "./components/ui";
import { Overview } from "./components/Dashboard";
import {
  ChannelsPage,
  GuidePage,
  PaymentsPage,
  ReviewsPage,
  SettingsPage,
} from "./components/Pages";
import {
  CreatePaymentModal,
  FindPaymentModal,
  PaymentDetailModal,
} from "./components/PaymentForms";
import type { ConsoleConfig, Page, PaymentRecord, ToastMessage } from "./types";

const NAVIGATION = [
  { id: "overview" as const, label: "Ringkasan", icon: SquaresFourIcon },
  { id: "payments" as const, label: "Pembayaran", icon: ReceiptIcon },
  { id: "channels" as const, label: "Metode pembayaran", icon: CreditCardIcon },
  { id: "reviews" as const, label: "Rekonsiliasi", icon: ArrowsClockwiseIcon },
];
const TITLES: Record<Page, { title: string; description: string }> = {
  overview: {
    title: "Ringkasan pembayaran",
    description: "Gambaran pembayaran Anda, dalam satu tempat.",
  },
  payments: {
    title: "Semua pembayaran",
    description: "Kelola dan pantau riwayat pembayaran di browser ini.",
  },
  channels: {
    title: "Metode pembayaran",
    description: "Berikan pelanggan lebih banyak cara untuk membayar.",
  },
  reviews: {
    title: "Rekonsiliasi",
    description: "Pastikan setiap pembayaran memiliki status yang jelas.",
  },
  settings: {
    title: "Pengaturan",
    description: "Periksa koneksi layanan dan preferensi dashboard.",
  },
  guide: {
    title: "Panduan penggunaan",
    description: "Kenali alur pembayaran dan mulai dengan percaya diri.",
  },
};

function initialPage(): Page {
  const hash = window.location.hash.slice(1);
  return hash in TITLES ? (hash as Page) : "overview";
}

function initialTheme(): "light" | "dark" {
  try {
    const value = localStorage.getItem("payment-console:theme:v1");
    if (value === "light" || value === "dark") return value;
  } catch {
    /* Use system preference when local storage is unavailable. */
  }
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

export default function App() {
  const [page, setPage] = useState<Page>(initialPage);
  const [payments, setPayments] = useState<PaymentRecord[]>(readHistory);
  const [demoItems, setDemoItems] = useState<PaymentRecord[]>(demoPayments);
  const [demo, setDemo] = useState(false);
  const [config, setConfig] = useState<ConsoleConfig | null>(null);
  const [health, setHealth] = useState<"loading" | "online" | "offline">(
    "loading",
  );
  const [theme, setTheme] = useState(initialTheme);
  const [mobileMenu, setMobileMenu] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [findOpen, setFindOpen] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [toast, setToast] = useState<ToastMessage | null>(null);
  const [days, setDays] = useState(14);
  const [storageAvailable, setStorageAvailable] = useState(true);
  const connecting = useRef(false);

  const items = demo ? demoItems : payments;
  const selected = items.find((payment) => payment.id === selectedId) || null;
  const displayedItems = filterPayments(items, "", "all", days);
  const title = TITLES[page];

  const notify = useCallback((message: ToastMessage) => setToast(message), []);
  const navigate = useCallback((next: Page) => {
    setPage(next);
    setMobileMenu(false);
    window.location.hash = next;
    window.scrollTo({ top: 0, behavior: "instant" });
  }, []);

  const checkConnection = useCallback(async () => {
    if (connecting.current) return;
    connecting.current = true;
    setHealth("loading");
    const [configuration, readiness] = await Promise.allSettled([
      api.config(),
      api.health(),
    ]);
    if (configuration.status === "fulfilled") setConfig(configuration.value);
    setHealth(
      readiness.status === "fulfilled" && readiness.value.status === "ok"
        ? "online"
        : "offline",
    );
    connecting.current = false;
  }, []);

  useEffect(() => {
    void checkConnection();
    const interval = window.setInterval(() => {
      if (document.visibilityState === "visible") void checkConnection();
    }, 30000);
    return () => window.clearInterval(interval);
  }, [checkConnection]);

  useEffect(() => {
    setStorageAvailable(persistHistory(payments));
  }, [payments]);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem("payment-console:theme:v1", theme);
    } catch {
      /* Theme remains available during this session. */
    }
  }, [theme]);
  useEffect(() => {
    if (!mobileMenu) return;
    const previousFocus = document.activeElement as HTMLElement | null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const sidebar = document.querySelector<HTMLElement>(".sidebar")!;
    sidebar.querySelector<HTMLElement>("a")?.focus({ preventScroll: true });
    function trapFocus(event: KeyboardEvent) {
      if (event.key !== "Tab") return;
      const controls = [
        ...sidebar.querySelectorAll<HTMLElement>("a, button"),
      ].filter((element) => !element.hasAttribute("disabled"));
      const first = controls[0];
      const last = controls[controls.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    }
    sidebar.addEventListener("keydown", trapFocus);
    return () => {
      document.body.style.overflow = previousOverflow;
      sidebar.removeEventListener("keydown", trapFocus);
      previousFocus?.focus({ preventScroll: true });
    };
  }, [mobileMenu]);
  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(
      () => setToast(null),
      toast.kind === "error" ? 8000 : 4500,
    );
    return () => window.clearTimeout(timer);
  }, [toast]);
  useEffect(() => {
    function onHashChange() {
      setPage(initialPage());
      setMobileMenu(false);
    }
    function shortcuts(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setFindOpen(true);
      }
      if (event.key === "Escape") setMobileMenu(false);
    }
    window.addEventListener("hashchange", onHashChange);
    window.addEventListener("keydown", shortcuts);
    return () => {
      window.removeEventListener("hashchange", onHashChange);
      window.removeEventListener("keydown", shortcuts);
    };
  }, []);

  const refreshPayment = useCallback(
    async (id: string) => {
      if (demo) return;
      const snapshot = await api.status(id);
      setPayments((previous) =>
        upsertPayment(
          previous,
          fromSnapshot(
            snapshot,
            previous.find((item) => item.id === id),
          ),
        ),
      );
    },
    [demo],
  );

  function updatePayment(payment: PaymentRecord) {
    if (demo) setDemoItems((previous) => upsertPayment(previous, payment));
    else setPayments((previous) => upsertPayment(previous, payment));
  }

  function createdPayment(payment: PaymentRecord) {
    updatePayment(payment);
    setCreateOpen(false);
    setSelectedId(payment.id);
  }

  async function findPayment(id: string) {
    if (demo) {
      const payment = demoItems.find((item) => item.id === id);
      if (!payment)
        throw new Error("ID pembayaran tidak ditemukan dalam data demo.");
      setSelectedId(payment.id);
    } else {
      const snapshot = await api.status(id);
      setPayments((previous) =>
        upsertPayment(
          previous,
          fromSnapshot(
            snapshot,
            previous.find((item) => item.id === id),
          ),
        ),
      );
      setSelectedId(snapshot.payment_intent_id);
    }
  }

  function toggleDemo() {
    setDemo((current) => !current);
    setSelectedId(null);
    setMobileMenu(false);
    notify({
      message: demo
        ? "Menampilkan riwayat pembayaran dari API."
        : "Mode demo aktif. Data ini hanya contoh.",
      kind: "info",
    });
  }

  return (
    <IconsProvider>
      <div className="app-shell">
        {mobileMenu ? (
          <button
            className="sidebar-backdrop"
            onClick={() => setMobileMenu(false)}
            aria-label="Tutup navigasi"
          />
        ) : null}
        <aside
          className={`sidebar ${mobileMenu ? "sidebar-open" : ""}`}
          aria-label="Navigasi utama"
        >
          <a
            className="brand"
            href="#overview"
            onClick={() => setMobileMenu(false)}
          >
            <span className="brand-symbol">
              p<span />
            </span>
            <span>
              payment<span className="brand-period">.</span>
              <small>PAYMENT CONSOLE</small>
            </span>
          </a>
          <button
            className="mobile-close icon-button"
            onClick={() => setMobileMenu(false)}
            aria-label="Tutup menu"
          >
            <XIcon />
          </button>
          <div className="workspace-selector">
            <span className="workspace-avatar">PS</span>
            <div>
              <strong>Payment Service</strong>
              <span>Workspace lokal</span>
            </div>
          </div>
          <div className="sidebar-label">WORKSPACE</div>
          <nav>
            {NAVIGATION.map((item) => (
              <a
                key={item.id}
                href={`#${item.id}`}
                aria-current={page === item.id ? "page" : undefined}
                className={`nav-item ${page === item.id ? "nav-active" : ""}`}
                onClick={(event) => {
                  event.preventDefault();
                  navigate(item.id);
                }}
              >
                <item.icon
                  size={20}
                  weight={page === item.id ? "fill" : "regular"}
                />
                <span>{item.label}</span>
                {item.id === "payments" && items.length ? (
                  <span className="nav-count">{items.length}</span>
                ) : null}
              </a>
            ))}
          </nav>
          <div className="sidebar-bottom">
            <div className="sidebar-demo">
              <span className="demo-icon">
                <FlaskIcon size={21} />
              </span>
              <h3>Kenali dashboard Anda</h3>
              <p>Jelajahi semua fitur dengan data contoh.</p>
              <button onClick={toggleDemo}>
                {demo ? "Kembali ke data API" : "Lihat mode demo"}
                <ArrowUpRightIcon size={16} />
              </button>
            </div>
            <nav>
              <a
                className={`nav-item ${page === "guide" ? "nav-active" : ""}`}
                href="#guide"
                onClick={(event) => {
                  event.preventDefault();
                  navigate("guide");
                }}
              >
                <BookOpenIcon size={20} />
                <span>Panduan penggunaan</span>
              </a>
              <a
                className={`nav-item ${page === "settings" ? "nav-active" : ""}`}
                href="#settings"
                onClick={(event) => {
                  event.preventDefault();
                  navigate("settings");
                }}
              >
                <GearSixIcon size={20} />
                <span>Pengaturan</span>
              </a>
            </nav>
            <div className="sidebar-footer">
              <span>
                <i />
                {demo
                  ? "Mode demo"
                  : config?.environment === "production"
                    ? "Production"
                    : "Lingkungan lokal"}
              </span>
              <span>Xendit v3</span>
            </div>
          </div>
        </aside>
        <div className="main-shell" inert={mobileMenu}>
          <header className="topbar">
            <div className="breadcrumb">
              <button
                className="mobile-toggle icon-button"
                onClick={() => setMobileMenu(true)}
                aria-label="Buka navigasi"
                aria-expanded={mobileMenu}
              >
                <ListIcon size={23} />
              </button>
              <span>Workspace</span>
              <span className="breadcrumb-separator">/</span>
              <strong>
                {NAVIGATION.find((item) => item.id === page)?.label ||
                  (page === "settings" ? "Pengaturan" : "Panduan")}
              </strong>
            </div>
            <div className="topbar-actions">
              <button
                className="global-search"
                onClick={() => setFindOpen(true)}
              >
                <MagnifyingGlassIcon size={17} />
                <span>Cari pembayaran…</span>
                <kbd>⌘ K</kbd>
              </button>
              <button
                className={`connection-status connection-${demo ? "demo" : health}`}
                onClick={() => navigate("settings")}
              >
                <span className="status-dot" />
                {demo
                  ? "Mode demo"
                  : health === "online"
                    ? "API terhubung"
                    : health === "loading"
                      ? "Memeriksa API"
                      : "API offline"}
              </button>
              <div className="topbar-divider" />
              <button
                className="icon-button theme-button"
                onClick={() =>
                  setTheme((value) => (value === "light" ? "dark" : "light"))
                }
                aria-label={
                  theme === "light"
                    ? "Aktifkan mode gelap"
                    : "Aktifkan mode terang"
                }
              >
                {theme === "light" ? (
                  <MoonIcon size={19} />
                ) : (
                  <SunIcon size={19} />
                )}
              </button>
              <button
                className="profile-avatar"
                onClick={() => navigate("settings")}
                aria-label="Pengaturan workspace"
              >
                PS
              </button>
            </div>
          </header>
          <main className="main-content" id="main-content">
            {demo ? (
              <div className="demo-banner">
                <FlaskIcon size={18} />
                <span>
                  <strong>Mode demo aktif.</strong> Semua angka dan pembayaran
                  adalah data contoh.
                </span>
                <button onClick={toggleDemo}>
                  Kembali ke data API <ArrowRightIcon size={16} />
                </button>
              </div>
            ) : health === "offline" ? (
              <div className="offline-banner">
                <PlugsConnectedIcon size={19} />
                <span>
                  API belum terhubung. Jalankan backend untuk mulai mengelola
                  pembayaran.
                </span>
                <button onClick={() => navigate("settings")}>
                  Periksa koneksi <ArrowRightIcon size={16} />
                </button>
              </div>
            ) : null}
            {!storageAvailable ? (
              <div className="offline-banner">
                <WarningCircleIcon size={19} />
                <span>
                  Penyimpanan browser tidak tersedia. Riwayat hanya bertahan
                  selama sesi ini.
                </span>
              </div>
            ) : null}
            <div className="page-header">
              <div>
                <div className="page-eyebrow">
                  {page === "overview" ? "WORKSPACE ANDA" : "PAYMENT SERVICE"}
                </div>
                <h1>{title.title}</h1>
                <p>{title.description}</p>
              </div>
              <div className="page-header-actions">
                {page === "overview" || page === "payments" ? (
                  <>
                    <div className="period-selector">
                      <CalendarBlankIcon size={17} />
                      <select
                        aria-label="Rentang waktu"
                        value={days}
                        onChange={(event) =>
                          setDays(Number(event.target.value))
                        }
                      >
                        <option value="7">7 hari terakhir</option>
                        <option value="14">14 hari terakhir</option>
                        <option value="30">30 hari terakhir</option>
                        <option value="36500">Semua waktu</option>
                      </select>
                    </div>
                    <Button icon={PlusIcon} onClick={() => setCreateOpen(true)}>
                      Buat pembayaran
                    </Button>
                  </>
                ) : null}
              </div>
            </div>
            {page === "overview" ? (
              <Overview
                payments={displayedItems}
                days={Math.min(days, 30)}
                onSelect={(payment) => setSelectedId(payment.id)}
                onAllPayments={() => navigate("payments")}
                onNew={() => setCreateOpen(true)}
                onChannels={() => navigate("channels")}
              />
            ) : page === "payments" ? (
              <PaymentsPage
                payments={displayedItems}
                onSelect={(payment) => setSelectedId(payment.id)}
                onFind={() => setFindOpen(true)}
                notify={notify}
              />
            ) : page === "channels" ? (
              <ChannelsPage />
            ) : page === "reviews" ? (
              <ReviewsPage
                key={String(demo)}
                demo={demo}
                payments={items}
                notify={notify}
              />
            ) : page === "settings" ? (
              <SettingsPage
                config={config}
                health={health}
                onReconnect={() => void checkConnection()}
                demo={demo}
                onDemo={toggleDemo}
                storageAvailable={storageAvailable}
              />
            ) : (
              <GuidePage
                onNew={() => setCreateOpen(true)}
                onFind={() => setFindOpen(true)}
              />
            )}
            <footer className="main-footer">
              <span>Payment Console</span>
              <span>
                Data {demo ? "contoh" : "browser ini"} · IDR · Indonesia
              </span>
            </footer>
          </main>
        </div>
        <CreatePaymentModal
          open={createOpen}
          onOpenChange={setCreateOpen}
          onCreated={createdPayment}
          demo={demo}
          notify={notify}
        />
        <FindPaymentModal
          open={findOpen}
          onOpenChange={setFindOpen}
          onFind={findPayment}
          demo={demo}
        />
        <PaymentDetailModal
          payment={selected}
          onClose={() => setSelectedId(null)}
          onUpdated={updatePayment}
          onRefresh={refreshPayment}
          notify={notify}
          demo={demo}
        />
        <Toast toast={toast} dismiss={() => setToast(null)} />
      </div>
    </IconsProvider>
  );
}
