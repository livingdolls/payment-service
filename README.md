# payment-service

Service Go untuk membuat payment intent, mengirim payment request ke Xendit v3,
menerima webhook, dan menjalankan reconciliation. Proyek ini masih dalam pengembangan.

## Menjalankan pengujian lokal

Butuh Go 1.26, PostgreSQL, `psql`, dan `goose`. Jika database compose belum aktif:

```sh
docker compose up -d postgres
bash scripts/test-payment.sh local
```

Satu perintah tersebut menjalankan unit test dengan race detector/coverage,
lalu integration test HTTP → PostgreSQL → provider mock → webhook worker.
Test mencakup 22 profil IDR yang dapat dijalankan melalui endpoint service,
serta kasus invalid input, idempotency, error provider, webhook duplikat/stale,
dan recovery melalui reconciliation. Setiap integration test menggunakan schema
acak, menerapkan migration goose asli, lalu menghapus schema miliknya.

Database default `payment_service_test` dibuat terpisah dari database aplikasi;
database aplikasi tidak dihapus atau direset. `TEST_DATABASE_URL` dapat ditentukan
untuk PostgreSQL lain, tetapi nama databasenya harus berakhiran `_test`.
Script memperbaiki PATH/GOROOT Go untuk mencegah coverage memakai toolchain berbeda.

Laporan JSON dan Markdown disimpan ke `artifacts/payment-tests/`. Ubah lokasi
dengan `PAYMENT_TEST_REPORT_DIR`. Hasil lokal menguji aplikasi menggunakan
provider mock; hasil tersebut belum membuktikan aktivasi merchant atau
kompatibilitas channel pada Xendit sandbox.

## Menjalankan Xendit sandbox

Isi `.env` berdasarkan `.env.example`, gunakan secret key Xendit **TEST mode**,
dan pastikan `ngrok` telah dikonfigurasi. Python 3 digunakan untuk memeriksa URL
tunnel dari log agent milik script. Jangan menggunakan key live atau kartu asli.
Script hanya mengizinkan `APP_ENV=development/test`, prefix key development/test,
dan database terpisah berakhiran `_test`.

```sh
bash scripts/test-payment.sh sandbox
```

Script membuat `payment_service_sandbox_test`, menjalankan migration asli,
menjalankan binary `cmd/api` beserta kedua worker pada port **8091**, lalu
menyediakan URL HTTPS melalui ngrok. Sebelum payment pertama, daftarkan URL
yang ditampilkan pada entri callback dengan nama tepat
**Payment Requests v3 – Payment Status** di dashboard Xendit TEST mode:

```text
https://PUBLIC-TUNNEL/webhooks/xendit/payments
```

Entry v3 ini membawa event capture, authorization, dan failure. Callback legacy
Virtual Account memakai payload berbeda dan tidak cocok untuk endpoint service
ini. Gunakan token callback yang sama dengan `XENDIT_WEBHOOK_TOKEN`, lalu
pastikan pengujian delivery v3 dashboard berhasil. Script menunggu konfirmasi
`configured` di terminal sebelum membuat
payment. Pada terminal non-interaktif, runner tetap terblokir sampai
`SANDBOX_CALLBACK_CONFIGURED=1` ditentukan secara eksplisit.

Untuk menyiapkan API/tunnel terlebih dahulu tanpa membuat payment:

```sh
bash scripts/test-payment.sh sandbox-setup
```

Biarkan terminal setup aktif ketika mengubah dashboard. Untuk memakai API/tunnel
yang sudah berjalan dan menguji channel tertentu:

```sh
SANDBOX_REUSE_API=1 \
SANDBOX_DATABASE_URL='postgres://payment:payment@127.0.0.1:5433/payment_service_sandbox_test?sslmode=disable' \
SANDBOX_PUBLIC_URL='https://PUBLIC-TUNNEL' \
SANDBOX_CALLBACK_CONFIGURED=1 \
bash scripts/test-payment.sh sandbox -channel BCA_VIRTUAL_ACCOUNT,QRIS -timeout 120s
```

`SANDBOX_PUBLIC_URL` dapat memakai tunnel selain ngrok, asalkan meneruskan ke API
yang sama. `PAYMENT_SANDBOX_PORT` mengganti port API. Untuk menjalankan runner
langsung terhadap binary API yang telah aktif, export environment sandbox yang
sama lalu gunakan `go run ./cmd/payment-test -api-url http://127.0.0.1:8091`.

Runner membuat payment melalui API aplikasi, memprosesnya melalui endpoint
payment-attempt, dan menggunakan endpoint simulasi resmi untuk bank/QR/retail.
Wallet/kartu mengikuti action atau layar TEST-mode/3DS yang disediakan provider;
channel yang belum diselesaikan pengguna diberi `BLOCKED`. Batas tunggu default
60 detik per channel, dapat diubah dengan `-timeout`.
Pada halaman wallet TEST mode yang menyediakan tombol **Proceed to Pay**, klik
tombol tersebut untuk menyelesaikan checkout. Untuk kartu, selesaikan challenge
3DS memakai OTP uji yang ditampilkan pada halaman provider. OVO dan JENIUSPAY
dapat selesai otomatis di sandbox; cashtag JENIUSPAY harus diawali `$`.
Runner tidak mengirim callback buatan pada pengujian sandbox.

**PASS** mensyaratkan intent dan attempt `CAPTURED`, captured amount tepat sama
dengan amount permintaan, dan webhook capture `PROCESSED` dengan reference ID,
payment request ID, serta payment ID yang cocok. Capture melalui response
provider/reconciliation tanpa webhook dipisahkan dalam `completion_path` dan
tetap gagal memverifikasi jalur webhook. Verifikasi database menggunakan koneksi
read-only. Laporan tidak menyimpan credential, body provider, detail kartu,
atau URL action yang mengandung credential.

## Katalog channel dan arti laporan

Fixture `testdata/xendit-idr-channels.json` mencatat 33 channel IDR, 22 profil
yang dapat diuji, dan 11 blocker dengan tautan bukti dokumentasi resmi.
Melihat katalog tanpa koneksi jaringan:

```sh
go run ./cmd/payment-test -list
```

| Status | Arti |
| --- | --- |
| PASS | Alur lengkap dan keadaan akhir memenuhi kriteria verifikasi. |
| FAIL | Request, amount, status akhir, worker, atau delivery webhook bermasalah. |
| SKIP | Channel tidak dipilih atau run dibatalkan sebelum channel dimulai. |
| BLOCKED | Profil v3 belum terkonfirmasi, flow token belum tersedia, merchant belum aktif, credential ditolak, action pengguna belum selesai, atau callback belum dikonfigurasi. |

Sebelas blocker tidak dihitung sebagai keberhasilan: BJB/HANA VA;
AKULAKU, ATOME, INDODANA, KREDIVO, NEXCASH; CIMB/MANDIRI/BRI direct debit;
dan GOPAY_RECURRING. BRI direct debit/GoPay recurring membutuhkan flow token
yang belum diekspos service. Dukungan delapan profil lain ditandai Ignore
dalam CSV channel resmi; HANA belum memiliki profil v3 yang terkonfirmasi.
Runner keluar dengan kode 1 jika ada `FAIL`, dan kode 2 jika channel runnable
yang dipilih terblokir. Sebelas blocker katalog bersifat informasional dan
tidak mengubah exit code ketika seluruh channel runnable yang dipilih lulus.
Pilihan yang hanya berisi blocker katalog keluar dengan kode 2 karena tidak
ada alur runnable yang diuji. Untuk memberi reconciliation waktu bekerja,
gunakan `-timeout 180s` atau lebih; worker mulai memeriksa attempt setelah
minimal dua menit.

Referensi: [metode pembayaran](https://docs.xendit.co/docs/available-payment-methods),
[properti channel v3](https://doc-widget.xendit.co/reference-files/channel-properties.csv),
[tabel PAY](https://doc-widget.xendit.co/channel-table/),
[simulasi TEST mode](https://docs.xendit.co/apidocs/simulate-payment-test-mode),
[create payment request](https://docs.xendit.co/apidocs/create-payment-request).
