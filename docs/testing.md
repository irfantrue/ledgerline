# Testing

## Aturan utama

- **Test tinggal di `test/`, bukan di sebelah kode.** Pohon test mencerminkan source tree:

| Source | Test |
|---|---|
| `internal/ledger/` | `test/ledger/` |
| `internal/shared/config/` | `test/shared/config/` |
| `internal/shared/httpx/middleware/` | `test/shared/httpx/middleware/` |
| `pkg/jwt/` | `test/pkg/jwt/` |

- Karena package test berada di luar kode yang diuji, test hanya bisa memakai identifier **exported**. Kalau test butuh helper, export helper-nya — jangan memindahkan test balik ke `internal/`.
- Tidak ada framework mocking. Pakai table-driven test dan `httptest`.
- `internal/` dan `pkg/` bebas file test.

## Unit test

Tidak butuh service apa pun:

```bash
make test          # go test -v -race ./...
```

## Test integrasi

Butuh service nyata. Test **skip sendiri** kalau env var-nya kosong, jadi `make test` tetap jalan tanpa DB.

| Service | Variable | Dipakai oleh |
|---|---|---|
| PostgreSQL | `DB_DSN` | `test/ledger/*`, `test/shared/database/*` |
| Redis | `REDIS_HOST`, `REDIS_PORT` (opsional `REDIS_PASSWORD`) | `test/shared/cache/redis_test.go` |
| RabbitMQ | `RABBITMQ_URL` | `test/shared/broker/rabbitmq_test.go` |

> `.env` **tidak** dimuat otomatis oleh test — test membaca `os.Getenv` langsung. Env var harus ada di environment proses.

### Larangan: jangan arahkan `DB_DSN` ke database dev

`test/ledger` menjalankan `DROP SCHEMA IF EXISTS ledger CASCADE` lalu apply ulang seluruh `migrations/ledger/*.sql` **pada setiap run**. Database yang ditunjuk akan kehilangan schema `ledger` dan datanya.

Pakai container sekali pakai:

```bash
docker run -d --rm --name ledgerline-pg-test \
  -e POSTGRES_PASSWORD=test \
  -e POSTGRES_DB=ledgerline_test \
  -p 55433:5432 postgres:18
```

### Menjalankan test integrasi

```bash
# env var diambil dari shell, bukan dari .env
DB_DSN='postgres://postgres:test@127.0.0.1:55433/ledgerline_test?sslmode=disable' \
REDIS_HOST=127.0.0.1 REDIS_PORT=6379 \
RABBITMQ_URL='amqp://guest:guest@127.0.0.1:5672/' \
go test ./... -race
```

Kalau `RABBITMQ_URL` diambil dari `.env`, beware `DB_DSN` di `.env` ikut terbawa — export manual, jangan `source .env` utuh.

## Service lokal

Service **tidak ada di repo ini** — PostgreSQL, Redis, dan RabbitMQ hidup di project environment terpisah (`/home/personal/Work/server`) sebagai docker compose stack.

```bash
cd /home/personal/Work/server
make postgres-up
make redis-up
make rabbitmq-up

make postgres-status   # atau redis-status / rabbitmq-status
make postgres-logs
```

| Service | Image | Port (localhost) |
|---|---|---|
| PostgreSQL | `postgres:18` | 5432 |
| Redis | `redis:7-alpine` | 6379 |
| RabbitMQ | `rabbitmq:4-management` | 5672 (AMQP), 5552 (stream), 15672 (UI) |

Compose file: `postgresql/docker-compose.yml`, `redis/docker-compose.yml`, `rabbitmq/docker-compose.yml`. Stack dev tidak auto-restart — container tetap down sampai di-start manual.

## Coverage

```bash
make coverage       # gate: gagal di bawah COVERAGE_MIN (default 80)
make test-coverage  # tulis coverage.html tanpa gate
```

`make coverage` memakai `-coverpkg=./...` karena test berada di `test/` — tanpa itu, default hanya mengukur package test itu sendiri.

Pengecualian dari pengukuran (lihat `COVERAGE_EXCLUDE` di `Makefile`):

- `ledgerline/cmd/api` — hanya wiring aplikasi.
- `ledgerline/internal/ledger/infrastructure/sqlcgen` — kode generated.

`scripts/coverage.sh` dibaca dari profile, difilter, lalu totalnya dibandingkan dengan minimum.

> Coverage **penuh** butuh ketiga service. Tanpa service, package ledger dan broker tidak pernah dieksekusi dan angka coverage anjlok jauh di bawah gate. Jalankan `make coverage` di server dev, bukan di CI.

## Di mana test berjalan

| Lingkungan | Yang jalan |
|---|---|
| CI | Unit test saja. Test integrasi mendeteksi env var kosong dan skip sendiri |
| Server dev | Seluruh suite melawan service sungguhan + gate coverage |

## Test yang ditulis untuk mengunci invariant

`test/ledger/ledger_invariants_integration_test.go` menguji aturan database, bukan kode aplikasi:

- Journal dan posting menolak UPDATE, DELETE, dan TRUNCATE.
- `accounts.balance` tidak bisa di-UPDATE langsung.
- Saldo negatif butuh `allow_negative = true`.
- Posting harus punya journal yang ada.
- `amount` harus positif.
- Trigger balance benar-benar mengubah saldo akun.

Test ini ada karena invariant yang hanya hidup di kode aplikasi bisa dilewati writer lain.

## Test yang mengunci mekanisme

`test/ledger/repository_querier_test.go` memakai stub `DBTX` untuk membuktikan repository meminta handle **per call**, bukan menyimpan satu handle. Itulah mekanismenya yang membuat satu call bisa ikut transaksi yang dibawa context-nya — dan itu yang dibutuhkan trigger balance yang deferrable.

## Pola umum test integrasi

```go
func testDB(t *testing.T) *pgxpool.Pool {
    t.Helper()

    dsn := os.Getenv("DB_DSN")
    if dsn == "" {
        t.Skip("DB_DSN not set; skipping integration test")
    }
    // ...
}
```

`sync.Once` dipakai untuk apply migration satu kali per test binary, bukan per test.
