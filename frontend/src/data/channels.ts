export type ChannelCategory =
  "QR code" | "Virtual account" | "E-wallet" | "Retail" | "Kartu" | "Lainnya";

export interface Channel {
  code: string;
  name: string;
  category: ChannelCategory;
  initials: string;
  color: string;
  available: boolean;
  properties: "none" | "name" | "return_url" | "phone" | "cashtag" | "card";
  minimumAmount?: number;
  note?: string;
}

// Matches testdata/xendit-idr-channels.json. CARDS requires explicit properties.
const banks = [
  ["BCA", "BCA", "#1673b8"],
  ["BNC", "Bank Neo", "#bc780b"],
  ["BNI", "BNI", "#bd611e"],
  ["BRI", "BRI", "#2368a0"],
  ["BSI", "BSI", "#278a79"],
  ["BSS", "Bank Sahabat Sampoerna", "#b64b4c"],
  ["CIMB", "CIMB Niaga", "#bd3f40"],
  ["INA", "Bank INA", "#235e8c"],
  ["MANDIRI", "Mandiri", "#416087"],
  ["MUAMALAT", "Muamalat", "#695a92"],
  ["PERMATA", "Permata", "#48825b"],
];

export const channels: Channel[] = [
  {
    code: "QRIS",
    name: "QRIS",
    category: "QR code",
    initials: "QR",
    color: "#303f42",
    available: true,
    properties: "none",
  },
  ...banks.map(([code, name, color]): Channel => ({
    code: `${code}_VIRTUAL_ACCOUNT`,
    name,
    category: "Virtual account",
    initials: code.slice(0, 3),
    color,
    available: true,
    properties: "name",
    // https://docs.xendit.co/docs/bca-virtual-account (checked 2026-10-14)
    minimumAmount: code === "BCA" ? 10000 : undefined,
  })),
  ...[
    ["DANA", "DANA", "#2383bb"],
    ["GOPAY", "GoPay", "#2b8b91"],
    ["SHOPEEPAY", "ShopeePay", "#c55732"],
    ["LINKAJA", "LinkAja", "#bf454b"],
    ["ASTRAPAY", "AstraPay", "#386bb3"],
  ].map(([code, name, color]): Channel => ({
    code,
    name,
    category: "E-wallet",
    initials: name.slice(0, 2),
    color,
    available: true,
    properties: "return_url",
  })),
  {
    code: "OVO",
    name: "OVO",
    category: "E-wallet",
    initials: "OVO",
    color: "#7160a1",
    available: true,
    properties: "phone",
  },
  {
    code: "JENIUSPAY",
    name: "Jenius Pay",
    category: "E-wallet",
    initials: "J",
    color: "#399db5",
    available: true,
    properties: "cashtag",
  },
  {
    code: "ALFAMART",
    name: "Alfamart",
    category: "Retail",
    initials: "A",
    color: "#b93d43",
    available: true,
    properties: "name",
  },
  {
    code: "INDOMARET",
    name: "Indomaret",
    category: "Retail",
    initials: "I",
    color: "#2d719a",
    available: true,
    properties: "name",
  },
  {
    code: "CARDS",
    name: "Kartu kredit / debit",
    category: "Kartu",
    initials: "CC",
    color: "#4d6470",
    available: true,
    properties: "card",
    note: "Isi channel_properties sesuai profil kartu merchant.",
  },
  ...[
    "BJB_VIRTUAL_ACCOUNT",
    "HANA_VIRTUAL_ACCOUNT",
    "AKULAKU",
    "ATOME",
    "INDODANA",
    "KREDIVO",
    "NEXCASH",
    "CIMB_DIRECT_DEBIT",
    "MANDIRI_DIRECT_DEBIT",
    "BRI_DIRECT_DEBIT",
    "GOPAY_RECURRING",
  ].map((code): Channel => ({
    code,
    name: code.replaceAll("_", " "),
    category: "Lainnya",
    initials: code.slice(0, 2),
    color: "#71807b",
    available: false,
    properties: "none",
    note: ["BRI_DIRECT_DEBIT", "GOPAY_RECURRING"].includes(code)
      ? "Memerlukan saved payment token; alurnya belum tersedia di API."
      : "Profil channel v3 belum terkonfirmasi pada katalog proyek.",
  })),
];

export const channelByCode = (code?: string) =>
  channels.find((channel) => channel.code === code);
export const channelName = (code?: string) =>
  channelByCode(code)?.name || code || "Belum dipilih";

export function buildProperties(
  channel: Channel,
  fields: {
    name: string;
    phone: string;
    cashtag: string;
    returnUrl: string;
    advanced: string;
    advancedEnabled: boolean;
  },
): Record<string, unknown> {
  if (fields.advancedEnabled || channel.properties === "card") {
    const value: unknown = JSON.parse(fields.advanced || "{}");
    if (value === null || Array.isArray(value) || typeof value !== "object")
      throw new Error("Properti channel harus berupa objek JSON.");
    if (channel.properties === "card" && !Object.keys(value).length)
      throw new Error("Isi properti channel untuk pembayaran kartu.");
    return value as Record<string, unknown>;
  }
  if (channel.properties === "name")
    return channel.category === "Retail"
      ? { payer_name: fields.name.trim() }
      : { display_name: fields.name.trim() };
  if (channel.properties === "phone")
    return { account_mobile_number: fields.phone.trim() };
  if (channel.properties === "cashtag")
    return { cashtag: fields.cashtag.trim() };
  if (channel.properties === "return_url") {
    const value = new URL(fields.returnUrl);
    if (value.protocol !== "https:")
      throw new Error("URL kembali harus memakai HTTPS.");
    return {
      success_return_url: value.href,
      ...(["GOPAY", "ASTRAPAY"].includes(channel.code)
        ? { failure_return_url: value.href }
        : {}),
      ...(channel.code === "GOPAY" ? { cancel_return_url: value.href } : {}),
    };
  }
  return {};
}
