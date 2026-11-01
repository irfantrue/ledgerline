# ledgerline

Backend Go untuk aplikasi ledger **double-entry** (jurnal debit/kredit) di atas Fiber v3 dan PostgreSQL.

Aturan main yang dijaga database, bukan kode aplikasi: journal dan posting bersifat **append-only**, setiap journal harus **balance** (total debit = total kredit), saldo akun hanya boleh berubah lewat posting, dan mata uang posting harus sama dengan mata uang akunnya.

## Isi repo

| Path | Isi |
|---|---|
| `cmd/api/` | Entry point; wiring config, logger, DB, Redis, broker, HTTP |
| `internal/shared/` | Kode platform: `config`, `logger`, `database`, `httpx` (+ `middleware`, `response`), `cache`, `broker` |
| `internal/ledger/` | Modul ledger: `domain/` (entity) + `infrastructure/` (`queries/`, `sqlcgen/`, `repository/`) |
| `pkg/` | Helper reusable: `ctxval`, `crypto`, `decimalutil`, `idgen`, `jwt`, `validator` |
| `test/` | Semua test, pohonnya mencerminkan source tree |
| `migrations/ledger/` | Migration SQL tulis-tangan (`001` schema, `002` trigger) |
| `vendor/` | Dependency di-commit agar build jalan tanpa network |

## Stack

| | |
|---|---|
| Go | 1.27 |
| HTTP | Fiber v3 (`github.com/gofiber/fiber/v3`) |
| Database | PostgreSQL 18+ (`pgx/v5`), schema `ledger` |
| SQL | sqlc → `internal/ledger/infrastructure/sqlcgen` |
| Money | `shopspring/decimal` — **never** `float64` |
| ID | uuidv7 (`google/uuid`); tabel ledger default ke `uuidv7()` di DB |
| Config | viper, dimuat dari `.env` + environment |
| Lint | golangci-lint v2 (`.golangci.yml`) |

PostgreSQL 18+ wajib: query memakai built-in `uuidv7()`.

## Quick start

```bash
# 1. Dependency (vendor ikut ter-commit, tapi tetap perlu di-refresh kalau import berubah)
make tidy && make vendor

# 2. Konfigurasi
cp .env.example .env
# edit .env: DB_DSN, RABBITMQ_URL, APP_JWT_SECRET (minimal 32 byte)

# 3. Service (PostgreSQL, Redis, RabbitMQ) — lived di project environment terpisah,
#    bukan di repo ini. Lihat docs/testing.md
cd /home/personal/Work/server && make postgres-up redis-up rabbitmq-up

# 4. Migration
for f in migrations/ledger/*.sql; do psql "$DB_DSN" -f "$f"; done

# 5. Jalankan
make run          # go run ./cmd/api
make watch        # hot reload via air
```

## Perintah

| Perintah | Fungsi |
|---|---|
| `make run` | Jalankan API |
| `make watch` | Hot reload (air) |
| `make build` | Build binary → `bin/app` (linux/amd64) |
| `make tidy && make vendor` | Refresh `go.mod`/`go.sum`/`vendor/` |
| `make lint` | golangci-lint |
| `make test` | `go test -v -race ./...` |
| `make coverage` | Coverage penuh + gate `COVERAGE_MIN` (default 80) |
| `make all` | tidy → vendor → lint → test → build |

`sqlc generate` dijalankan manual setelah mengedit `internal/ledger/infrastructure/queries/*.sql`.

## Endpoint

Hanya dua endpoint yang terdaftar saat ini (`internal/shared/httpx/router.go`):

| Method | Path | Fungsi |
|---|---|---|
| GET | `/healthz` | Liveness — proses hidup |
| GET | `/readyz` | Readiness — DB bisa di-ping |

Modul `internal/ledger` belum punya handler HTTP; ia tersedia lewat package `repository`. Lihat [docs/api.md](docs/api.md).

## Dokumentasi

| Dokumen | Isi |
|---|---|
| [docs/architecture.md](docs/architecture.md) | Layering, aturan import, alur request, pola transaksi |
| [docs/ledger.md](docs/ledger.md) | Model double-entry, entity, cara menulis journal yang balance |
| [docs/database.md](docs/database.md) | Skema tabel, inventaris trigger, alur migration & sqlc |
| [docs/api.md](docs/api.md) | Response envelope, RFC 7807, middleware, endpoint |
| [docs/configuration.md](docs/configuration.md) | Semua env var, validasi config, prasyarat produksi |
| [docs/testing.md](docs/testing.md) | Pohon test, test integrasi, stack service lokal, coverage |
| [CLAUDE.md](CLAUDE.md) | Aturan kerja untuk AI agent: quality gate, konvensi, larangan |

## Status saat ini

- Yang sudah ada: platform (config, DB+transaksi, HTTP+middleware, cache, broker), package `pkg/`, modul ledger lengkap di domain + repository + trigger DB.
- Yang belum: handler/endpoint HTTP untuk ledger, lapisan use-case, modul bisnis lain (`identity`, `account`, `customer`, `transaction`, `notification` — sudah disebut di aturan `depguard` tapi belum ada).
- `pkg/crypto`, `pkg/jwt`, `pkg/decimalutil`, `pkg/idgen` belum dipakai kode produksi; yang memakainya tinggal test.
