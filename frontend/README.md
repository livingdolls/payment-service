# Payment Console

Frontend React + TypeScript + Vite untuk payment-service. Styling memakai CSS
biasa, dialog memakai Radix, dan ikon memakai Phosphor. Tidak menggunakan Next.js.

## Menjalankan

Butuh Node.js 22.12+ dan API payment-service yang sudah berjalan.

```sh
cd frontend
npm ci
npm run dev
```

Buka **http://127.0.0.1:5173**. Backend dijalankan secara terpisah menggunakan
`make dev` atau `make run` dari direktori utama. Dashboard tetap bisa dibuka
tanpa backend; gunakan tombol **Lihat mode demo** di sidebar untuk melihat data
contoh dan mencoba interaksi tanpa mengirim pembayaran ke provider.

Frontend membaca konfigurasi server dari `.env` di direktori utama, lalu
`frontend/.env` jika ada, lalu environment proses sebagai override. File
`.env.local`, `.env.<mode>`, dan `.env.<mode>.local` juga didukung. Nilai `$` dan
`#` pada credential dibaca literal, sesuai loader environment backend. Tulis
komentar di baris terpisah.

```dotenv
PAYMENT_SERVICE_URL=http://127.0.0.1:8080
PAYMENT_INTERNAL_TOKEN=isi_token_yang_sama_dengan_backend_minimal_32_karakter
ADMIN_API_TOKEN=isi_token_admin_yang_sama_dengan_backend_minimal_32_karakter
APP_ENV=development
```

Jika `PAYMENT_SERVICE_URL` tidak diisi, target mengikuti `HTTP_PORT` atau 8080.
Gunakan port yang benar jika 8080 sudah dipakai layanan lain. Contoh override:

```sh
PAYMENT_SERVICE_URL=http://127.0.0.1:8081 npm run dev
```

Restart frontend setelah mengubah konfigurasi koneksi atau token.
Token internal dan admin hanya dibaca server proxy Vite; jangan memberi prefix
`VITE_` pada credential. Browser mengirim permintaan ke `/api`, lalu proxy
menambahkan token yang sesuai. Server development hanya bind ke loopback dan
menolak permintaan browser dari origin lain.

## Fitur

- Ringkasan pembayaran, grafik aktivitas, dan distribusi metode pembayaran.
- Membuat payment intent dengan ID pesanan, nominal IDR, dan kunci idempotensi
  yang dipertahankan saat request diulang setelah gagal.
- Memproses seluruh 22 profil channel dari katalog backend, termasuk properti
  nama pembayar, URL kembali e-wallet, nomor OVO, cashtag Jenius, dan JSON manual.
- Menampilkan QRIS, nomor virtual account, kode retail, dan tautan checkout
  provider. Properti kartu diisi eksplisit melalui JSON dan tidak disimpan.
- Pencarian status menggunakan UUID pembayaran dan pembaruan otomatis setiap
  8 detik saat detail pembayaran yang belum selesai terbuka. Polling berhenti
  ketika tab tidak terlihat atau pembayaran selesai.
- Respons HTTP 202 ditampilkan sebagai proses rekonsiliasi; UI tidak menyatakan
  pembayaran berhasil atau langsung mengirim ulang ke provider.
- Daftar review admin dan antrekan ulang dengan alasan audit 10–500 karakter.
- Filter, pencarian riwayat, pagination, ekspor CSV, mode gelap, dan navigasi ponsel.

## Kontrak API

| Permintaan browser                                            | Endpoint backend                                     | Token proxy |
| ------------------------------------------------------------- | ---------------------------------------------------- | ----------- |
| `GET /api/health/ready`                                       | `/health/ready`                                      | Tidak ada   |
| `POST /api/payments`                                          | `/internal/payments`                                 | Internal    |
| `GET /api/payments/:id/status`                                | `/internal/payments/:id/status`                      | Internal    |
| `POST /api/payment-attempts/:id/process`                      | `/internal/payment-attempts/:id/process`             | Internal    |
| `GET /api/admin/reconciliation-reviews`                       | `/admin/reconciliation-reviews`                      | Admin       |
| `POST /api/admin/payment-attempts/:id/requeue-reconciliation` | `/admin/payment-attempts/:id/requeue-reconciliation` | Admin       |

`GET /api/console-config` adalah endpoint server frontend untuk URL API,
lingkungan, dan flag apakah token dikonfigurasi. Nilai token tidak dikembalikan.

API belum menyediakan daftar seluruh pembayaran. Riwayat UI berisi pembayaran
yang dibuat atau ditemukan di browser ini, maksimal 500 item, dan disimpan
di localStorage. Data demo berada dalam state terpisah dan tidak disimpan ke
riwayat API. Instruksi pembayaran, URL action provider, token, dan properti
channel tidak disimpan ke localStorage. Tanggal dari pembayaran yang diimpor
melalui endpoint status ditandai sebagai waktu ditambahkan ke browser, karena
endpoint status belum mengirim waktu pembuatan. Statistik menggunakan waktu
tercatat pada riwayat browser.

Webhook tetap ditangani backend; dashboard tidak menyimulasikan capture atau
menandai pembayaran API sebagai berhasil tanpa status dari backend.

## Build dan pengujian

```sh
npm run build
npm run preview
npm test
npm run test:e2e
```

Build menghasilkan `frontend/dist`. Preview lokal tersedia pada port 4173
dan memiliki konfigurasi proxy yang sama. File statis hasil build tidak membawa
server proxy Vite: deployment membutuhkan gateway/BFF terautentikasi dengan
rute `/api` dan token pada server. Tidak ada credential dalam bundle React.

Tes browser memakai Chrome lokal di `/usr/bin/google-chrome` serta API mock
terisolasi pada port 18991 dan frontend pada port 5174. Tidak menggunakan Xendit
atau database aplikasi. Ubah `PLAYWRIGHT_CHROME_PATH` jika lokasi Chrome berbeda.
Untuk memeriksa perbaikan query status backend pada schema PostgreSQL sementara:

```sh
# Dari direktori utama proyek
make test-local ARGS='-run TestInternalPaymentStatus'
```

Katalog UI diverifikasi terhadap `../testdata/xendit-idr-channels.json` oleh tes.
Referensi action provider: [Xendit Payment Requests v3](https://docs.xendit.co/apidocs/create-payment-request).
