# Arsitektur

## Peta layer

```
cmd/api/                     wiring aplikasi (entry point)
  │
  ├── internal/shared/       platform: config, logger, database, httpx, cache, broker
  │     └── internal/shared/httpx/{middleware,response}
  │
  ├── internal/ledger/       modul bisnis
  │     ├── domain/          entity + aturan bisnis (tanpa import infrastructure)
  │     └── infrastructure/  queries/ (SQL) → sqlcgen/ (generated) → repository/
  │
  └── pkg/                   helper reusable (tanpa import internal/**)
```

Aturan import ditegakkan `depguard` di `.golangci.yml`:

| Rule | Larang |
|---|---|
| `pkg-no-internal-modules` | `pkg/**` → `ledgerline/internal/**` |
| `shared-no-business-modules` | `internal/shared/**` → modul bisnis (`ledger`, `account`, `customer`, `identity`, `transaction`, `notification`) |

Jadi `internal/shared` tidak boleh tahu apa pun tentang ledger. Arah ketergantungan selalu satu arah: `domain` ← `infrastructure` ← `sqlcgen`.

## Alur request

```
client
  │
  ▼
Fiber (httpx.NewFiber)
  │  JSON encoder/decoder: bytedance/sonic
  │  read 10s / write 15s / idle 60s / body limit 1 MiB
  ▼
middleware (urutan di httpx.RegisterRoutes)
  1. Recover      ← harus #1, menangkap panic
  2. CORS         ← asalkan header cross-origin secepatnya
  3. Helmet       ← security headers
  4. Compress     ← level dari APP_COMPRESS_LEVEL
  5. Timezone     ← parse header X-Timezone → context
  6. RequestID    ← isi X-Request-ID + Fiber locals
  7. RequestLogger← log metadata response (body tidak pernah di-log)
  ▼
handler
  ▼
response.WriteSuccess / response.WriteProblem
```

Handler mengembalikan error; `middleware.NewErrorHandler` yang memetakan error ke HTTP:

| Error | Hasil |
|---|---|
| `*response.Problem` | ditulis apa adanya (RFC 7807) |
| `*fiber.Error` | status + `http.StatusText` + pesan |
| lainnya | 500 generik; detail asli hanya masuk log, tidak pernah ke client |

## Pola transaksi

Ini bagian yang paling mudah salah. Repository **menyelesaikan handle database per call**, bukan per repository:

```go
// internal/ledger/infrastructure/repository/querier.go
type Querier interface {
    Querier(ctx context.Context) database.DBTX
}
```

`accountRepository.queries(ctx)` memanggil `r.q.Querier(ctx)` setiap kali. Handle yang dikembalikan adalah transaksi yang ada di context, kalau ada; kalau tidak, pool biasa. Karena itu menulis journal + postings dalam satu transaksi cukup dilakukan begini:

```go
tm := database.NewTxManager(pool)

err := tm.Do(ctx, func(ctx context.Context) error {
    journal, err := journals.Create(ctx, repository.CreateJournalParams{
        IdempotencyKey: key,
        Description:    "sale",
    })
    if err != nil {
        return fmt.Errorf("create journal: %w", err)
    }

    if _, err := postings.Create(ctx, repository.CreatePostingParams{
        JournalID: journal.ID,
        AccountID: debitAccount.ID,
        Direction: domain.DirectionDebit,
        Amount:    amount,
        Currency:  debitAccount.Currency,
    }); err != nil {
        return fmt.Errorf("create debit posting: %w", err)
    }
    // ... posting kredit
    return nil
})
```

`database.TxManager` sudah menangani:

- **Reuse**: kalau context sudah membawa transaksi, `Do` memanggil fn-nya langsung tanpa nesting.
- **Commit/rollback**: rollback memakai `context.WithoutCancel(ctx)` supaya tetap jalan walau request context dibatalkan.
- **Retry**: deadlock (`40P01`) dan serialization failure (`40001`) dicoba ulang, default 3x, backoff eksponensial `delay * 2^(attempt-1)` dengan shift dibatasi 10 (mencegah overflow int64 yang membuat delay negatif).
- **Isolasi**: default `READ COMMITTED`, bisa diubah lewat `WithIsolationLevel`.

> Penting: fn yang dipanggil ulang harus idempotent dan tidak boleh punya efek samping di luar DB. Publish ke broker atau tulis ke cache akan terjadi sekali per percobaan.

## Aturan nominal

- Money selalu `decimal.Decimal`. Tidak ada `float64` di jalur mana pun yang menyentuh uang.
- `NUMERIC` dipetakan ke `decimal.Decimal` lewat override sqlc di `sqlc.yaml`.
- `json_build_object` merender `NUMERIC` sebagai JSON number, jadi query yang mengembalikan nominal di-cast ke `::text` dan sisi Go parse dengan `decimal.NewFromString`. `repository.numericText` menjaga teks mentah — decode ke `float64` akan kehilangan presisi untuk `NUMERIC(18,2)`.
- ID uuidv7. Di DB, tabel ledger memakai default `uuidv7()` (butuh PostgreSQL 18+). `pkg/idgen.New()` menghasilkan uuidv7 di sisi Go untuk kasus yang butuh ID sebelum INSERT.

## Inventaris package

### `internal/shared/`

| Package | Isi | Catatan |
|---|---|---|
| `config` | `Config` + `LoadConfig` (viper) | memuat semua env var; validasi saat load |
| `logger` | `slog` wrapper | JSON di production, text di selainnya |
| `database` | `NewPostgres`, `NewRedisClient`, `TxManager` | `DBTX` = interface yang sama dengan yang sqlc generate |
| `httpx` | `NewFiber`, `RegisterRoutes` | middleware + health/readiness |
| `httpx/middleware` | recover, cors, helmet, compress, timezone, requestid, request_logger, error | |
| `httpx/response` | `Response[T]`, `Problem`, helper writer | RFC 7807 |
| `cache` | `Service` interface + `RedisCache` | Set/Get/Delete/DeleteByPrefix |
| `broker` | `MessageBroker` interface + `RabbitMQ` | publish dengan publisher confirms, DLX topology |

### `pkg/`

| Package | Isi | Dipakai kode produksi? |
|---|---|---|
| `crypto` | Hash/verify password Argon2id (PHC format) | belum |
| `ctxval` | Simpan/baca timezone di context | ya (`middleware/timezone.go`) |
| `decimalutil` | Konversi nilai driver → `decimal.Decimal` | belum |
| `idgen` | uuidv7 string | belum |
| `jwt` | Generate/validate JWT HMAC-SHA256 | belum |
| `validator` | `ValidateStruct` → `[]InvalidParam` | ya (`httpx/response`) |

## Middleware timezone

`middleware.NewTimezone` membaca header `X-Timezone`, mem-parse-nya, lalu menyimpannya ke context lewat `ctxval.SetTimezone`. Hasil parse di-cache:

- Batas 1000 entry, LRU sederhana dengan slice `order`.
- TTL 10 menit per entry.
- `singleflight.Group` supaya banyak request untuk zona yang sama tidak mem-parse berulang.
- Key yang tidak valid → fallback ke `APP_TIMEZONE`; kalau default-nya juga invalid → `time.UTC`.

## Middleware request logger

Log berisi: `request_id`, `method`, `path`, `route`, `status`, `duration_ms`, `client_ip`, `user_agent`, `resp_size`. **Body response tidak pernah di-log** — isi ledger memuat saldo dan data sensitif.
