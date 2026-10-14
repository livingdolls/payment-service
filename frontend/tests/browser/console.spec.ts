import { test, expect } from "@playwright/test";

const internalToken = "frontend-test-internal-token-at-least-32-characters";
const mockUrl = "http://127.0.0.1:18991";

test.beforeEach(async ({ request }) => {
  await request.post(`${mockUrl}/__test/reset`);
});

test("creates a real API request, processes QRIS, and keeps provider actions out of storage", async ({
  page,
  request,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await expect(
    page.getByRole("button", { name: "API terhubung" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Buat pembayaran", exact: true })
    .first()
    .click();
  const create = page.getByRole("dialog");
  await create.getByLabel("ID pesanan").fill("ORDER-UI-001");
  await create.getByLabel("Nominal pembayaran").fill("185000");
  await create
    .getByRole("button", { name: "Buat pembayaran", exact: true })
    .click();
  const detail = page.getByRole("dialog");
  await expect(
    detail.getByRole("heading", { name: "Detail pembayaran" }),
  ).toBeVisible();
  await detail.getByRole("button", { name: "Lanjutkan dengan QRIS" }).click();
  await expect(
    detail.getByRole("img", { name: "Kode QRIS pembayaran" }),
  ).toBeVisible();
  await expect(
    detail.getByText("Menunggu bayar", { exact: true }),
  ).toBeVisible();
  const traffic = await (
    await request.get(`${mockUrl}/__test/requests`)
  ).json();
  const created = traffic.find(
    (entry: { path: string }) => entry.path === "/internal/payments",
  );
  expect(created.authorization).toBe(`Bearer ${internalToken}`);
  expect(created.key).toMatch(/^[0-9a-f-]{36}$/);
  expect(created.body).toEqual({
    order_id: "ORDER-UI-001",
    amount: 185000,
    currency: "IDR",
  });
  const processed = traffic.find((entry: { path: string }) =>
    entry.path.endsWith("/process"),
  );
  expect(processed.body).toEqual({
    country: "ID",
    channel_code: "QRIS",
    channel_properties: {},
    description: "",
  });
  const stored = await page.evaluate(() =>
    localStorage.getItem("payment-console:history:v1"),
  );
  expect(stored).not.toContain("MOCK-QRIS");
  expect(stored).not.toContain("actions");
  expect(stored).not.toContain(internalToken);
  expect(errors).toEqual([]);
  await detail.getByRole("button", { name: "Tutup dialog" }).click();
  await page.reload();
  await expect(
    page.getByRole("button", { name: "ORDER-UI-001" }).first(),
  ).toBeVisible();
});

test("retries creation with the same idempotency key and handles uncertain provider outcomes", async ({
  page,
  request,
}) => {
  await request.post(`${mockUrl}/__test/mode`, {
    data: { mode: "create-fail" },
  });
  await page.goto("/");
  await page
    .getByRole("button", { name: "Buat pembayaran", exact: true })
    .first()
    .click();
  let dialog = page.getByRole("dialog");
  await dialog.getByLabel("ID pesanan").fill("ORDER-RETRY-001");
  await dialog.getByLabel("Nominal pembayaran").fill("75000");
  await dialog
    .getByRole("button", { name: "Buat pembayaran", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toContainText(
    "Layanan sementara tidak tersedia",
  );
  await dialog
    .getByRole("button", { name: "Buat pembayaran", exact: true })
    .click();
  dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("heading", { name: "Detail pembayaran" }),
  ).toBeVisible();
  await request.post(`${mockUrl}/__test/mode`, {
    data: { mode: "unknown-outcome" },
  });
  await dialog.getByRole("button", { name: "Lanjutkan dengan QRIS" }).click();
  await expect(dialog.getByText("Diproses", { exact: true })).toBeVisible();
  await expect(dialog.getByText("UNKNOWN", { exact: true })).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "Lanjutkan dengan QRIS" }),
  ).toHaveCount(0);
  const traffic = await (
    await request.get(`${mockUrl}/__test/requests`)
  ).json();
  const creates = traffic.filter(
    (entry: { path: string }) => entry.path === "/internal/payments",
  );
  expect(creates).toHaveLength(2);
  expect(creates[0].key).toBe(creates[1].key);
});

test("looks up an existing payment and displays provider failures", async ({
  page,
  request,
}) => {
  const result = await request.post("/api/payments", {
    headers: { "Idempotency-Key": "lookup-test-key" },
    data: { order_id: "ORDER-LOOKUP-001", amount: 125000, currency: "IDR" },
  });
  expect(result.status()).toBe(201);
  const item = await result.json();
  await page.goto("/");
  await page.getByRole("button", { name: "Cari pembayaran…" }).click();
  const dialog = page.getByRole("dialog");
  await dialog
    .getByLabel("ID pembayaran")
    .fill("00000000-0000-4000-8000-000000000001");
  await dialog
    .getByRole("button", { name: "Cari pembayaran", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toContainText("payment not found");
  await dialog.getByLabel("ID pembayaran").fill(item.id);
  await dialog
    .getByRole("button", { name: "Cari pembayaran", exact: true })
    .click();
  await expect(
    page.getByRole("dialog").getByText("ORDER-LOOKUP-001", { exact: true }),
  ).toBeVisible();
});

test("loads and requeues authenticated reconciliation reviews", async ({
  page,
  request,
}) => {
  await page.goto("/#reviews");
  await expect(
    page.getByText("ORDER-REVIEW-01", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Antrekan ulang", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog
    .getByLabel("Alasan peninjauan ulang")
    .fill("Provider kembali tersedia, periksa status final.");
  await dialog
    .getByRole("button", { name: "Antrekan ulang", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "Tidak ada pembayaran yang perlu ditinjau",
    }),
  ).toBeVisible();
  const traffic = await (
    await request.get(`${mockUrl}/__test/requests`)
  ).json();
  const queued = traffic.find((entry: { path: string }) =>
    entry.path.endsWith("/requeue-reconciliation"),
  );
  expect(queued.authorization).toBe(
    "Bearer frontend-test-admin-token-at-least-32-characters",
  );
  expect(queued.body.reason).toBe(
    "Provider kembali tersedia, periksa status final.",
  );
});

test("isolates demo data, filters records, exports CSV, and changes theme", async ({
  page,
}) => {
  await page.goto("/");
  await page
    .getByRole("button", { name: "Lihat mode demo", exact: true })
    .click();
  await expect(
    page.getByText("Semua angka dan pembayaran adalah data contoh."),
  ).toBeVisible();
  await page.getByRole("button", { name: "Tutup notifikasi" }).click();
  await page.screenshot({
    path: "/tmp/payment-console-desktop.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Lihat semua", exact: true }).click();
  await page.getByRole("button", { name: /^Berhasil/ }).click();
  await expect(page.locator("tbody .status-badge").first()).toHaveText(
    "Berhasil",
  );
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Ekspor CSV" }).click();
  expect((await download).suggestedFilename()).toMatch(/^pembayaran-.*\.csv$/);
  await page.getByRole("button", { name: "Aktifkan mode gelap" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.goto("/#overview");
  await page.getByRole("button", { name: "Tutup notifikasi" }).click();
  await page.screenshot({
    path: "/tmp/payment-console-dark.png",
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Kembali ke data API", exact: true })
    .last()
    .click();
  await expect(
    page.getByRole("heading", { name: "Pembayaran pertama dimulai di sini" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () =>
        JSON.parse(localStorage.getItem("payment-console:history:v1") || "{}")
          .items.length,
    ),
  ).toBe(0);
});

test("shows a usable mobile layout, navigation, and modal", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await page.getByRole("button", { name: "Buka navigasi" }).click();
  await page
    .getByRole("button", { name: "Lihat mode demo", exact: true })
    .click();
  await expect(page.locator(".sidebar")).toBeHidden();
  await page.getByRole("button", { name: "Tutup notifikasi" }).click();
  await page.screenshot({
    path: "/tmp/payment-console-mobile.png",
    fullPage: true,
  });
  const layout = await page.evaluate(() => ({
    width: innerWidth,
    scroll: document.documentElement.scrollWidth,
    overflow: [...document.querySelectorAll("*")]
      .filter(
        (element) => element.getBoundingClientRect().right > innerWidth + 1,
      )
      .map((element) => ({
        tag: element.tagName,
        class: element.getAttribute("class"),
        width: element.getBoundingClientRect().width,
      }))
      .slice(0, 15),
  }));
  expect(layout.scroll, JSON.stringify(layout)).toBeLessThanOrEqual(
    layout.width,
  );
  await page
    .getByRole("button", { name: "Buat pembayaran", exact: true })
    .first()
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page
    .getByRole("dialog")
    .getByLabel("ID pesanan")
    .fill("ORDER-MOBILE-001");
  await page.getByRole("dialog").getByLabel("Nominal pembayaran").fill("95000");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Buat pembayaran", exact: true })
    .click();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("button", { name: "Lanjutkan dengan QRIS" }),
  ).toBeVisible();
  expect(
    await page
      .getByRole("dialog")
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
});

test("blocks requests from a foreign browser origin before injecting server credentials", async ({
  request,
}) => {
  const response = await request.post("/api/payments", {
    headers: {
      Origin: "https://unrelated.example",
      "Idempotency-Key": "foreign-origin",
    },
    data: { order_id: "UNAUTHORIZED", amount: 1, currency: "IDR" },
  });
  expect(response.status()).toBe(403);
  const traffic = await (
    await request.get(`${mockUrl}/__test/requests`)
  ).json();
  expect(traffic).toHaveLength(0);
});

test("restores virtual account instructions from the API after reloading local history", async ({
  page,
  request,
}) => {
  await page.goto("/");
  await page
    .getByRole("button", { name: "Buat pembayaran", exact: true })
    .first()
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("ID pesanan").fill("ORDER-VA-001");
  await dialog.getByLabel("Nominal pembayaran").fill("95000");
  await dialog
    .getByRole("button", { name: "Buat pembayaran", exact: true })
    .click();
  await expect(
    dialog.getByRole("heading", { name: "Detail pembayaran" }),
  ).toBeVisible();
  await dialog
    .getByLabel("Semua metode pembayaran")
    .selectOption("BCA_VIRTUAL_ACCOUNT");
  await dialog.getByLabel("Nama pembayar").fill("Ayu Prameswari");
  await dialog.getByRole("button", { name: "Lanjutkan dengan BCA" }).click();
  await expect(
    dialog.getByText("8808123456789", { exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(() =>
      localStorage.getItem("payment-console:history:v1"),
    ),
  ).not.toContain("8808123456789");
  await dialog.getByRole("button", { name: "Tutup dialog" }).click();
  await page.reload();
  await page
    .getByRole("button", { name: "Detail ORDER-VA-001", exact: true })
    .click();
  await expect(
    page.getByRole("dialog").getByText("8808123456789", { exact: true }),
  ).toBeVisible();
  const traffic = await (
    await request.get(`${mockUrl}/__test/requests`)
  ).json();
  const processed = traffic.find((entry: { path: string }) =>
    entry.path.endsWith("/process"),
  );
  expect(processed.body.channel_properties).toEqual({
    display_name: "Ayu Prameswari",
  });
});
