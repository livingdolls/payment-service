import { ArrowUpRightIcon } from "@phosphor-icons/react/dist/csr/ArrowUpRight";
import { ArrowRightIcon } from "@phosphor-icons/react/dist/csr/ArrowRight";
import { WalletIcon } from "@phosphor-icons/react/dist/csr/Wallet";
import { CheckCircleIcon } from "@phosphor-icons/react/dist/csr/CheckCircle";
import { ClockIcon } from "@phosphor-icons/react/dist/csr/Clock";
import { ChartLineUpIcon } from "@phosphor-icons/react/dist/csr/ChartLineUp";
import { PlusIcon } from "@phosphor-icons/react/dist/csr/Plus";
import { ReceiptIcon } from "@phosphor-icons/react/dist/csr/Receipt";
import { useId, useState } from "react";
import {
  ACTIVE_STATUSES,
  FAILED_STATUSES,
  count,
  dailyVolume,
  money,
} from "../lib/payments";
import { channelByCode, channelName } from "../data/channels";
import type { PaymentRecord } from "../types";
import { Button, ChannelMark, EmptyState } from "./ui";
import { PaymentTable } from "./PaymentTable";

function Sparkline({
  values,
  variant = "green",
}: {
  values: number[];
  variant?: "green" | "amber";
}) {
  const maximum = Math.max(...values, 1);
  const coordinates = values
    .map(
      (value, index) =>
        `${(index / Math.max(values.length - 1, 1)) * 96},${32 - (value / maximum) * 27}`,
    )
    .join(" ");
  return (
    <svg
      className={`sparkline sparkline-${variant}`}
      viewBox="0 0 96 36"
      aria-hidden="true"
    >
      <polyline
        points={coordinates}
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function Metrics({ payments }: { payments: PaymentRecord[] }) {
  const total = payments.reduce((sum, payment) => sum + payment.amount, 0);
  const succeeded = payments.filter((payment) => payment.status === "CAPTURED");
  const pending = payments.filter((payment) =>
    ACTIVE_STATUSES.has(payment.status),
  );
  const finished =
    succeeded.length +
    payments.filter((payment) => FAILED_STATUSES.has(payment.status)).length;
  const rate = finished
    ? Math.round((succeeded.length / finished) * 1000) / 10
    : null;
  const points = dailyVolume(payments, 14);
  const succeededPoints = dailyVolume(succeeded, 14);
  const pendingPoints = dailyVolume(pending, 14);
  const failedPoints = dailyVolume(
    payments.filter((payment) => FAILED_STATUSES.has(payment.status)),
    14,
  );
  const stats = [
    {
      label: "Total volume pembayaran",
      value: money(total),
      icon: WalletIcon,
      detail: `${count(payments.length)} permintaan pembayaran`,
      values: points.map((point) => point.amount),
    },
    {
      label: "Pembayaran berhasil",
      value: count(succeeded.length),
      icon: CheckCircleIcon,
      detail:
        money(
          succeeded.reduce((sum, payment) => sum + payment.captured_amount, 0),
        ) + " diterima",
      values: succeededPoints.map((point) => point.total),
    },
    {
      label: "Menunggu pembayaran",
      value: count(pending.length),
      icon: ClockIcon,
      detail: "Baru, diproses, atau menunggu aksi",
      values: pendingPoints.map((point) => point.total),
      amber: true,
    },
    {
      label: "Tingkat keberhasilan",
      value: rate === null ? "–" : `${rate.toLocaleString("id-ID")}%`,
      icon: ChartLineUpIcon,
      detail: `${count(finished)} pembayaran dengan status final`,
      values: succeededPoints.map((point, index) => {
        const final = point.total + failedPoints[index].total;
        return final ? (point.total / final) * 100 : 0;
      }),
    },
  ];
  return (
    <div className="metrics-grid">
      {stats.map((stat) => (
        <div className="metric-card" key={stat.label}>
          <div className="metric-label">
            <span>{stat.label}</span>
            <stat.icon size={18} />
          </div>
          <div className="metric-value">{stat.value}</div>
          <div className="metric-footer">
            <span>{stat.detail}</span>
            <Sparkline
              values={stat.values}
              variant={stat.amber ? "amber" : "green"}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

function VolumeChart({
  payments,
  days,
}: {
  payments: PaymentRecord[];
  days: number;
}) {
  const [metric, setMetric] = useState<"amount" | "total">("amount");
  const [hovered, setHovered] = useState<number | null>(null);
  const id = useId().replaceAll(":", "");
  const points = dailyVolume(payments, days);
  const max =
    Math.max(
      ...points.map((point) => point[metric]),
      metric === "amount" ? 100000 : 5,
    ) * 1.15;
  const graph = {
    width: 720,
    height: 230,
    left: 58,
    top: 18,
    bottom: 193,
    right: 706,
  };
  const x = (index: number) =>
    graph.left + (index / Math.max(days - 1, 1)) * (graph.right - graph.left);
  const y = (value: number) =>
    graph.bottom - (value / max) * (graph.bottom - graph.top);
  const coordinates = points.map(
    (point, index) => `${x(index)},${y(point[metric])}`,
  );
  const line = `M ${coordinates.join(" L ")}`;
  const area = `${line} L ${graph.right},${graph.bottom} L ${graph.left},${graph.bottom} Z`;
  const total = points.reduce((sum, point) => sum + point[metric], 0);
  const selected = hovered !== null ? points[hovered] : null;
  return (
    <section className="panel volume-panel">
      <div className="panel-heading">
        <div>
          <h2>Aktivitas pembayaran</h2>
          <p>Pergerakan pembayaran selama {days} hari terakhir</p>
        </div>
        <div className="segmented-control" aria-label="Satuan grafik">
          <button
            aria-pressed={metric === "amount"}
            onClick={() => setMetric("amount")}
            className={metric === "amount" ? "selected" : ""}
          >
            Nominal
          </button>
          <button
            aria-pressed={metric === "total"}
            onClick={() => setMetric("total")}
            className={metric === "total" ? "selected" : ""}
          >
            Transaksi
          </button>
        </div>
      </div>
      <div className="chart-summary">
        <strong>{metric === "amount" ? money(total) : count(total)}</strong>
        <span>
          <i />
          {metric === "amount" ? "Total nominal" : "Jumlah pembayaran"}
        </span>
      </div>
      <div className="chart-container">
        <svg
          className="volume-chart"
          viewBox={`0 0 ${graph.width} ${graph.height}`}
          role="img"
          aria-label={`Grafik ${metric === "amount" ? "nominal" : "jumlah"} pembayaran selama ${days} hari terakhir`}
          onMouseLeave={() => setHovered(null)}
        >
          <defs>
            <linearGradient id={`chart-fill-${id}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--accent)" stopOpacity="0.16" />
              <stop
                offset="100%"
                stopColor="var(--accent)"
                stopOpacity="0.015"
              />
            </linearGradient>
          </defs>
          {[0, 1, 2, 3, 4].map((tick) => {
            const value = (max * tick) / 4;
            const label =
              metric === "total"
                ? Math.round(value).toString()
                : value >= 1000000
                  ? `${(value / 1000000).toLocaleString("id-ID", { maximumFractionDigits: 1 })} jt`
                  : `${Math.round(value / 1000)} rb`;
            return (
              <g key={tick}>
                <line
                  x1={graph.left}
                  x2={graph.right}
                  y1={y(value)}
                  y2={y(value)}
                  className="chart-gridline"
                />
                <text
                  x={graph.left - 12}
                  y={y(value) + 4}
                  textAnchor="end"
                  className="chart-label"
                >
                  {label}
                </text>
              </g>
            );
          })}
          <path d={area} fill={`url(#chart-fill-${id})`} />
          <path
            d={line}
            fill="none"
            stroke="var(--accent)"
            strokeWidth="2.4"
            strokeLinejoin="round"
            strokeLinecap="round"
          />
          {points.map((point, index) => (
            <g key={index}>
              {index % Math.max(1, Math.round(days / 7)) === 0 ||
              index === days - 1 ? (
                <text
                  x={x(index)}
                  y={222}
                  textAnchor="middle"
                  className="chart-label"
                >
                  {point.label}
                </text>
              ) : null}
              <rect
                x={x(index) - (graph.right - graph.left) / days / 2}
                y={graph.top}
                width={(graph.right - graph.left) / days}
                height={graph.bottom - graph.top}
                fill="transparent"
                onMouseEnter={() => setHovered(index)}
              >
                <title>
                  {point.label}:{" "}
                  {metric === "amount"
                    ? money(point.amount)
                    : `${point.total} pembayaran`}
                </title>
              </rect>
            </g>
          ))}
          {hovered !== null ? (
            <g pointerEvents="none">
              <line
                x1={x(hovered)}
                x2={x(hovered)}
                y1={graph.top}
                y2={graph.bottom}
                className="chart-hover-line"
              />
              <circle
                cx={x(hovered)}
                cy={y(points[hovered][metric])}
                r="4.5"
                fill="var(--accent)"
                stroke="var(--surface)"
                strokeWidth="2"
              />
            </g>
          ) : null}
        </svg>
        {selected ? (
          <div className="chart-tooltip">
            <span>{selected.label}</span>
            <strong>
              {metric === "amount"
                ? money(selected.amount)
                : `${selected.total} pembayaran`}
            </strong>
          </div>
        ) : null}
        {payments.length === 0 ? (
          <div className="chart-empty">
            <ChartLineUpIcon size={23} />
            <span>Grafik akan terisi saat ada pembayaran</span>
          </div>
        ) : null}
      </div>
    </section>
  );
}

function MethodsDistribution({
  payments,
  onChannels,
}: {
  payments: PaymentRecord[];
  onChannels: () => void;
}) {
  const colors = ["#247656", "#519475", "#8caf97", "#bfd0c3", "#dde6df"];
  const grouped = new Map<string, number>();
  for (const payment of payments)
    if (payment.channel_code)
      grouped.set(
        payment.channel_code,
        (grouped.get(payment.channel_code) || 0) + 1,
      );
  const sorted = [...grouped].sort((a, b) => b[1] - a[1]);
  const items = sorted.slice(0, 4);
  if (sorted.length > 4)
    items.push([
      "OTHER",
      sorted.slice(4).reduce((sum, item) => sum + item[1], 0),
    ]);
  const total = items.reduce((sum, item) => sum + item[1], 0);
  let offset = 0;
  const slices = items.map(([, value], index) => {
    const start = offset;
    offset += (value / Math.max(total, 1)) * 100;
    return `${colors[index]} ${start}% ${offset}%`;
  });
  return (
    <section className="panel distribution-panel">
      <div className="panel-heading">
        <div>
          <h2>Metode pembayaran</h2>
          <p>Metode yang paling sering digunakan</p>
        </div>
      </div>
      <div
        className="donut"
        style={{
          background: total
            ? `conic-gradient(${slices.join(", ")})`
            : "var(--border)",
        }}
        role="img"
        aria-label={`${total} pembayaran memiliki metode`}
      >
        <div className="donut-center">
          <strong>{count(total)}</strong>
          <span>pembayaran</span>
        </div>
      </div>
      <div className="distribution-legend">
        {items.length ? (
          items.map(([code, value], index) => (
            <div key={code}>
              <span>
                <i style={{ background: colors[index] }} />
                {code === "OTHER" ? "Metode lainnya" : channelName(code)}
              </span>
              <strong>
                {Math.round((value / total) * 100)}
                <small>%</small>
              </strong>
            </div>
          ))
        ) : (
          <p className="muted text-center">Belum ada metode yang digunakan</p>
        )}
      </div>
      <button className="panel-link" onClick={onChannels}>
        Lihat semua metode <ArrowUpRightIcon size={17} />
      </button>
    </section>
  );
}

export function Overview({
  payments,
  days,
  onSelect,
  onAllPayments,
  onNew,
  onChannels,
}: {
  payments: PaymentRecord[];
  days: number;
  onSelect: (payment: PaymentRecord) => void;
  onAllPayments: () => void;
  onNew: () => void;
  onChannels: () => void;
}) {
  return (
    <>
      <Metrics payments={payments} />
      <div className="dashboard-charts">
        <VolumeChart payments={payments} days={days} />
        <MethodsDistribution payments={payments} onChannels={onChannels} />
      </div>
      <section className="panel recent-panel">
        <div className="panel-heading">
          <div>
            <h2>Pembayaran terbaru</h2>
            <p>Aktivitas terakhir yang tercatat di browser ini</p>
          </div>
          <button className="text-button" onClick={onAllPayments}>
            Lihat semua <ArrowRightIcon size={16} />
          </button>
        </div>
        {payments.length ? (
          <PaymentTable payments={payments.slice(0, 6)} onSelect={onSelect} />
        ) : (
          <EmptyState
            icon={ReceiptIcon}
            title="Pembayaran pertama dimulai di sini"
            description="Buat pembayaran baru atau cari ID pembayaran yang sudah ada di API."
            compact
          >
            <Button onClick={onNew} icon={PlusIcon}>
              Buat pembayaran
            </Button>
          </EmptyState>
        )}
        <div className="table-footnote">
          <span className="subtle-dot" />
          Riwayat tersimpan hanya di browser ini
          <span>{count(payments.length)} pembayaran</span>
        </div>
      </section>
      <div className="overview-bottom">
        <div>
          <ChannelMark channel={channelByCode("QRIS")} small />
          <span>Didukung Xendit Payment Requests v3</span>
        </div>
        <span>Seluruh nominal dalam Rupiah (IDR)</span>
      </div>
    </>
  );
}
