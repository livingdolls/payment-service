import { ArrowUpRightIcon } from "@phosphor-icons/react/dist/csr/ArrowUpRight";
import { ReceiptIcon } from "@phosphor-icons/react/dist/csr/Receipt";
import { channelByCode, channelName } from "../data/channels";
import { formatDate, money, shortId } from "../lib/payments";
import type { PaymentRecord } from "../types";
import { ChannelMark, StatusBadge } from "./ui";

export function PaymentTable({
  payments,
  onSelect,
}: {
  payments: PaymentRecord[];
  onSelect: (payment: PaymentRecord) => void;
}) {
  return (
    <div className="table-scroll">
      <table className="payments-table">
        <thead>
          <tr>
            <th>Pembayaran</th>
            <th>Nominal</th>
            <th>Metode</th>
            <th>Status</th>
            <th>Waktu tercatat</th>
            <th>
              <span className="sr-only">Detail</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {payments.map((payment) => (
            <tr key={payment.id}>
              <td>
                <button
                  className="table-payment"
                  onClick={() => onSelect(payment)}
                >
                  <span className="receipt-icon">
                    <ReceiptIcon size={19} />
                  </span>
                  <span>
                    <strong>{payment.order_id}</strong>
                    <small>{shortId(payment.reference_id)}</small>
                  </span>
                </button>
              </td>
              <td className="amount-cell">{money(payment.amount)}</td>
              <td>
                <div className="method-cell">
                  <ChannelMark
                    channel={channelByCode(payment.channel_code)}
                    small
                  />
                  <span>{channelName(payment.channel_code)}</span>
                </div>
              </td>
              <td>
                <StatusBadge status={payment.status} />
              </td>
              <td
                className="date-cell"
                title={
                  payment.created_at_known === false
                    ? "Waktu ditambahkan ke browser; API status tidak mengirim tanggal pembuatan."
                    : "Waktu pembayaran dibuat"
                }
              >
                {formatDate(payment.created_at)}
              </td>
              <td>
                <button
                  className="icon-button table-detail"
                  onClick={() => onSelect(payment)}
                  aria-label={`Detail ${payment.order_id}`}
                >
                  <ArrowUpRightIcon size={18} />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
