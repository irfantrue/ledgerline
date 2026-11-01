# Konfigurasi

Config dimuat di `internal/shared/config` dengan viper: file `.env` + environment variables. **Environment variable menang atas isi file.**

```go
conf, err := config.LoadConfig(".env")
```

- File yang hilang bukan error → deploy bisa jalan hanya dari env var saja (container/production).
- Setiap key punya default terdaftar. Tanpa default, `viper.Unmarshal` hanya melihat env var untuk key yang sudah dikenal viper, jadi deploy env-only akan diam-diam menghasilkan `Config` kosong.
- Path relatif ke working directory — aplikasi dijalankan dari root repo.

## Env var

### Aplikasi

| Variabel | Default | Keterangan |
|---|---|---|
| `APP_NAME` | `app` | Nama aplikasi; dipakai sebagai `fiber.Config.AppName` |
| `APP_SERVER_HEADER` | `""` | Nilai header `Server` |
| `APP_PORT` | `8080` | Port HTTP |
| `APP_ENV` | `development` | `production` mengaktifkan JSON log dan validasi ketat |
| `APP_TIMEZONE` | `Asia/Jakarta` | Default timezone; bisa dioverride per-request lewat header `X-Timezone` |
| `APP_COMPRESS_LEVEL` | `1` | `0` mati, `1` best speed, `2` best compression |
| `APP_CORS_ALLOWED_ORIGINS` | `""` | Daftar origin dipisah koma; wajib diisi di production |
| `APP_JWT_SECRET` | — | **Wajib.** Minimal 32 byte |
| `APP_JWT_EXPIRE_HOURS` | `24` | Masa berlaku token; harus > 0 |

### Database

| Variabel | Default | Keterangan |
|---|---|---|
| `DB_DSN` | — | **Wajib.** Connection string PostgreSQL |
| `DB_MAX_OPEN_CONNS` | `10` | `0` → default pgxpool |
| `DB_MIN_OPEN_CONNS` | `2` | `0` → default pgxpool |
| `DB_MAX_CONN_LIFETIME` | `1h` | Format durasi Go (`30m`, `1h`, `30s`) |
| `DB_MAX_CONN_IDLE_TIME` | `30m` | |
| `DB_HEALTH_CHECK_PERIOD` | `1m` | |

### Redis

| Variabel | Default |
|---|---|
| `REDIS_HOST` | `localhost` |
| `REDIS_PORT` | `6379` |
| `REDIS_DB` | `0` |
| `REDIS_PASSWORD` | `""` |

### RabbitMQ

| Variabel | Default |
|---|---|
| `RABBITMQ_URL` | `""` — AMQP URL |

### Log

| Variabel | Default |
|---|---|
| `LOG_LEVEL` | `info` (`debug`, `info`, `warn`, `error`) |

## Validasi saat load

`config.validateConfig` menolak konfigurasi ini:

| Kondisi | Error |
|---|---|
| `DB_DSN` kosong | `DB_DSN is required` |
| `APP_JWT_SECRET` kosong | `APP_JWT_SECRET is required` |
| `APP_JWT_SECRET` < 32 byte | `APP_JWT_SECRET must be at least 32 bytes` |
| `APP_JWT_EXPIRE_HOURS` ≤ 0 | `APP_JWT_EXPIRE_HOURS must be greater than zero` |

Tambahan khusus `APP_ENV=production`:

| Kondisi | Error |
|---|---|
| `APP_JWT_SECRET` = `example` / `secret` / `changeme` (case-insensitive) | placeholder ditolak |
| `APP_CORS_ALLOWED_ORIGINS` kosong | wajib named origins |
| `APP_CORS_ALLOWED_ORIGINS` berisi `*` | wildcard ditolak |

> Panjang minimum 32 byte berlaku di semua environment, bukan hanya production. `APP_JWT_SECRET="example"` di `.env.example` **tidak** cukup — app tidak akan start dengan nilai itu.

> **Gotcha:** key di `.env` harus persis sama dengan nama variabel di tabel di atas. `.env` lokal saat ini memakai `TIMEZONE=Asia/Jakarta` — key itu **tidak** terbaca (nama yang dibaca adalah `APP_TIMEZONE`), jadi nilainya jatuh ke default. Selama default dan nilai yang dimaksud sama, hasilnya kebetulan benar.

## Prasyarat production

1. `APP_ENV=production` → log JSON ke stdout (siap untuk agregator).
2. `APP_JWT_SECRET` random minimal 32 byte, bukan placeholder.
3. `APP_CORS_ALLOWED_ORIGINS` diisi origin eksplisit, tanpa wildcard.
4. `DB_DSN` memakai PostgreSQL 18+ (butuh `uuidv7()`).
5. `APP_COMPRESS_LEVEL` disetel sesuai kebutuhan (`1` atau `2`).

## Catatan runtime

- `NewPostgres` mem-ping pool dengan timeout 10 detik; kegagalan ping menutup pool dan mengembalikan error.
- `NewRedisClient` mem-ping Redis saat startup; koneksi gagal = startup gagal (tidak ada retry).
- `NewRabbitMQ` melakukan dial + declare topologi DLX saat startup; koneksi gagal = startup gagal.
- Graceful shutdown: SIGINT/SIGTERM → `app.ShutdownWithContext` dengan timeout 10 detik, lalu pool Redis dan RabbitMQ ditutup.

## Contoh `.env`

```bash
APP_NAME="Ledgerline"
APP_SERVER_HEADER="Ledgerline"
APP_PORT=8080
APP_ENV=development
APP_COMPRESS_LEVEL=1
APP_CORS_ALLOWED_ORIGINS=
APP_JWT_SECRET="<random minimal 32 byte>"
APP_JWT_EXPIRE_HOURS=24
APP_TIMEZONE=Asia/Jakarta

DB_DSN="postgres://user:pass@localhost:5432/ledgerline?sslmode=disable"
DB_MAX_OPEN_CONNS=25
DB_MIN_OPEN_CONNS=2
DB_MAX_CONN_LIFETIME=1h
DB_MAX_CONN_IDLE_TIME=30m
DB_HEALTH_CHECK_PERIOD=1m

LOG_LEVEL=debug

RABBITMQ_URL="amqp://user:pass@localhost:5672/"
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_DB=0
REDIS_PASSWORD=
```
