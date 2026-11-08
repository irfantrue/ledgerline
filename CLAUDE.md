# CLAUDE.md

Guidance for AI coding agents (and humans) working in this repository.

## Dokumentasi

Dokumen di `docs/` menjelaskan **bagaimana repo ini bekerja**. Baca yang relevan sebelum mengubah kode; dokumen ini menunjuk ke detail, bukan mengulanginya.

| Dokumen | Isi |
|---|---|
| [README.md](README.md) | Orientasi: stack, quick start, perintah, status repo |
| [docs/architecture.md](docs/architecture.md) | Layering, aturan import, alur request, pola transaksi |
| [docs/ledger.md](docs/ledger.md) | Model double-entry, entity, cara menulis journal yang balance |
| [docs/database.md](docs/database.md) | Skema tabel, inventaris trigger, alur migration & sqlc |
| [docs/api.md](docs/api.md) | Response envelope, RFC 7807, middleware, endpoint |
| [docs/configuration.md](docs/configuration.md) | Semua env var, validasi config, prasyarat produksi |
| [docs/testing.md](docs/testing.md) | Pohon test, test integrasi, stack service lokal, coverage |

## Peran: code quality gate

Kamu bertindak sebagai **code quality gate** ala SonarQube. Kalau diminta "review", "analisis", atau "quality check", ikuti alur di bawah. Jangan mengubah kode kecuali diminta eksplisit. Tugas default: **laporkan temuan**, bukan memperbaiki.

## Proyek

Go backend untuk aplikasi ledgerline: double-entry ledger di atas Fiber v3.

| | |
|---|---|
| Module path | `ledgerline` — import `ledgerline/internal/...`, `ledgerline/pkg/...` |
| Go | 1.27 |
| HTTP | Fiber v3 (`github.com/gofiber/fiber/v3`) |
| Database | PostgreSQL 18+ (`pgx/v5`), schema `ledger` |
| SQL | sqlc → `internal/ledger/infrastructure/sqlcgen` |
| Money | `shopspring/decimal` — never `float64` |
| IDs | uuidv7 (`google/uuid`); tabel ledger default ke `uuidv7()` di DB |
| Config | viper, dimuat dari `.env` |
| Lint | golangci-lint v2 (`.golangci.yml`) |

Dependency **divendor dan di-commit**. `vendor/` harus tetap lengkap — lihat *Aturan Vendor*.

## Layout

| Path | Isi |
|---|---|
| `cmd/api/` | Entry point; wiring config, logger, DB, Redis, broker, HTTP |
| `internal/shared/` | Kode platform: `config`, `logger`, `database`, `httpx` (+ `middleware`, `response`), `cache`, `broker` |
| `internal/<module>/domain/` | Entity dan aturan bisnis; tanpa import infrastructure |
| `internal/<module>/infrastructure/` | `queries/` (`.sql`), `sqlcgen/` (generated), `repository/` (interface + impl) |
| `pkg/` | Helper reusable, minim dependency: `ctxval`, `crypto`, `decimalutil`, `idgen`, `jwt`, `validator` |
| `tests/` | Semua test, mencerminkan source tree — `tests/ledger/`, `tests/shared/…`, `tests/pkg/…`, plus `tests/mocks/<pkg>/` per package sumber yang punya interface |
| `migrations/ledger/` | Migration SQL tulis-tangan |
| `docs/` | Dokumentasi cara kerja repo (lihat tabel di atas) |

## Aturan arsitektur

Ditegakkan `depguard` di `.golangci.yml` — `make lint` gagal kalau dilanggar:

1. `pkg/**` tidak boleh import `ledgerline/internal/**`.
2. `internal/shared/**` tidak boleh import modul bisnis (`ledgerline/internal/ledger`, …).

Hormati layering `domain` → `infrastructure` → `sqlcgen`:

- `domain` berisi entity dan aturan; tidak boleh import infrastructure.
- `infrastructure/repository` mendefinisikan interface persistensi; implementasi di sebelahnya.
- `sqlcgen` hasil generate — jangan diedit manual.

## Aturan data

- **sqlc**: edit `internal/ledger/infrastructure/queries/*.sql`, lalu `sqlc generate`. Jangan edit `internal/ledger/infrastructure/sqlcgen/` manual.
- **Migration**: tambah `migrations/ledger/NNN_snake_case.sql`, bernomor setelah yang tertinggi. Plain SQL dan idempotent. Apply berurutan:
  `for f in migrations/ledger/*.sql; do psql "$DB_DSN" -f "$f"; done`
  `001_ledger.sql` berisi schema, tabel, index; `002_ledger_triggers.sql` berisi invariant double-entry. Tabel/index baru di `001`, logika trigger baru di `002`.
- **Ledger bersifat append-only.** `ledger.journals` dan `ledger.postings` menolak `UPDATE`/`DELETE`/`TRUNCATE` lewat trigger, dan `ledger.accounts.balance` hanya boleh berubah lewat postings. Nyatakan invariant baru sebagai trigger database, bukan hanya kode aplikasi.
- **Journal harus balance**: total debit = total kredit. Pengecekannya `DEFERRABLE INITIALLY DEFERRED` constraint trigger, jadi berjalan saat COMMIT — menulis posting satu journal karena itu wajib dalam satu transaksi.
- **Cara menulis dalam satu transaksi**: repository ledger resolve handle DB **per call** lewat `repository.Querier`, jadi otomatis ikut transaksi yang ada di context. Jadi cukup bungkus dengan `database.TxManager.Do(ctx, fn)` — jangan bikin `sqlcgen.Queries` per tx manual.
- **PostgreSQL 18+** wajib: query memakai built-in `uuidv7()`.
- Money tidak pernah lewat `float64`: `json_build_object` merender `NUMERIC` sebagai JSON number, jadi query cast ke text dan sisi Go parse dengan `decimal.NewFromString`.

## Konvensi Go

- Setiap package punya `doc.go` berisi package comment. Jaga.
- Money adalah `decimal.Decimal`; `NUMERIC` dipetakan ke decimal lewat override sqlc di `sqlc.yaml`.
- Wrap error dengan konteks dan `%w`: `fmt.Errorf("postgres: %w", err)`.
- Handler mengembalikan `response.WriteSuccess(c, &response.Response[T]{...})`. Error memakai helper RFC 7807: `response.WriteProblem`, `response.BadRequestResponse`, `response.UnauthorizedResponse`, `response.UnprocessableEntityResponse`.
- Validasi body request dengan `validator.ValidateStruct` → `[]InvalidParam` → `response.BadRequestResponse`.
- Urutan middleware disengaja: **Recover harus didaftarkan pertama** di `httpx.RegisterRoutes`.
- Komentari *kenapa*, bukan *apa*.
- Service/layanan eksternal (DB, Redis, broker) dipanggil dengan `context.Context` yang punya timeout.

## Testing

- **Test tinggal di `tests/`, bukan di sebelah kode.** Pohon test mencerminkan source: `tests/ledger/` untuk `internal/ledger/`, `tests/shared/httpx/middleware/` untuk `internal/shared/httpx/middleware/`. `internal/` dan `pkg/` bebas file test.
- Karena package test berada di luar kode yang diuji, mereka hanya bisa memakai identifier **exported**. Export helper bila test butuh, jangan memindahkan test balik ke `internal/`.
- `make test` menjalankan `go test -v -race ./...`.
- **Tidak ada service yang dibutuhkan.** Seluruh suite berjalan tanpa PostgreSQL, Redis, dan RabbitMQ: dependency-nya dimock pakai mock yang di-generate mockery.
- Assertion pakai `testify/assert` dan `testify/require`.
- Mock **di-generate** dengan [mockery](https://github.com/vektra/mockery) v3 (config `.mockery.yaml`, target `make mocks`); hasilnya tinggal di `tests/mocks/` dan ikut di-commit. Setiap package sumber punya direktori mock sendiri (`tests/mocks/databasemocks/`, `tests/mocks/pgxmocks/`, …) dengan nama package `<sumber>mocks` — jangan-satukan semua double dalam satu package. Jangan tulis mock interface tangan — daftarkan interface-nya di `.mockery.yaml` lalu regenerate.
- Mocking butuh seam di production code: `internal/shared/` punya interface kecil (`database.Pool`, `cache.Backend`, `broker.Channel`, `broker.Connection`) supaya implementasinya bisa dimock. Kalau butuh mock baru, tambahkan interface di package-nya lalu daftarkan di `.mockery.yaml`.
- Invariant ledger hidup di database (trigger/constraint) dan tidak bisa dimock — test-nya membaca file migration dan memastikan aturannya masih ada (`tests/ledger/migration_schema_test.go`, `tests/ledger/ledger_invariants_test.go`).
- **Di mana test berjalan:**
  - **CI** (`Dependencies` → `Lint` → `Test` → `Build`): seluruh suite, karena tidak butuh service.
  - **Server dev**: seluruh suite + gate coverage (`make coverage`).
- **Coverage**: `make coverage` mengukur seluruh modul dengan `-coverpkg=./...` (test ada di `tests/`, jadi default hanya mengukur dirinya sendiri), menulis `coverage.html`, dan gagal di bawah `COVERAGE_MIN` (80 default). `cmd/api` dan `tests/mocks/` dikecualikan: wiring aplikasi dan scaffolding test.

## Aturan vendor (penting)

`vendor/` di-commit agar build jalan tanpa akses network. Dua aturan ini menjaganya:

1. Setelah mengubah import, jalankan `make tidy && make vendor` dan commit `go.mod`, `go.sum`, dan `vendor/` bersama-sama. CI gagal jika ada drift.
2. Pola `.gitignore` **di-anchor ke root repository** (`/coverage.*`, `/.env`, `/go.work`, …). Glob tanpa anchor seperti `coverage.*` juga cocok dengan `vendor/golang.org/x/text/language/coverage.go`, yang diam-diam menghapus file itu dari commit dan merusak `make build` di clone baru. CI sekarang gagal kalau ada file di bawah `vendor/` yang ter-ignore — perbaiki polanya, jangan tambah aturan ignore untuk menewaskan.

---

## Alur kerja quality check

1. Tentukan scope: file/folder yang diminta, atau perubahan terbaru (`git diff main...HEAD`) sebagai "new code".
2. Jalankan tools (lihat bagian Perintah). Jangan menebak hasil, tampilkan output nyata.
3. Baca kode manual untuk temuan yang tidak tertangkap tools (logic bug, race condition, N+1 query, error handling).
4. Klasifikasikan temuan, hitung rating, tentukan Quality Gate pass/fail.
5. Tulis laporan sesuai format di bawah.

## Perintah (Go)
```bash
gofmt -l .                                   # format
go vet ./...                                 # bug dasar
golangci-lint run ./...                      # lint (config: .golangci.yml)
govulncheck ./...                            # vulnerability dependency
go test ./... -race -coverprofile=coverage.out -covermode=atomic
go tool cover -func=coverage.out | tail -n 1 # total coverage
```
Kalau tool belum terpasang, sebutkan dan lanjutkan dengan yang tersedia. Jangan install tanpa izin.

## Kategori Temuan
| Kategori | Contoh |
|---|---|
| **Bug** | nil dereference, error diabaikan, goroutine leak, race condition, `defer` di dalam loop, off-by-one |
| **Vulnerability** | SQL injection (string concat di query), hardcoded secret, weak crypto, missing authz/RBAC check, SSRF, path traversal |
| **Security Hotspot** | `exec.Command` dengan input user, `InsecureSkipVerify`, CORS `*`, logging data sensitif (PII/data pasien) |
| **Code Smell** | fungsi terlalu kompleks, nesting dalam, parameter banyak, dead code, magic number, naming buruk, package god-object |
| **Duplication** | blok kode mirip ≥ 10 baris / 3 kali muncul |
| **Coverage** | logic bisnis tanpa test, error path tidak dites |

Severity: `BLOCKER` > `CRITICAL` > `MAJOR` > `MINOR` > `INFO`.

## Threshold (Quality Gate untuk **new code**)
- Bug/Vulnerability baru: **0** untuk BLOCKER & CRITICAL
- Cognitive complexity per fungsi: **≤ 15**
- Parameter fungsi: **≤ 7**; panjang fungsi: **≤ 60 baris**; nesting: **≤ 4 level**
- Duplikasi: **< 3%**
- Coverage: **≥ 80%** (domain/usecase layer wajib, handler boleh lebih rendah)
- Tidak ada secret hardcoded, tidak ada error yang di-ignore (`_ =`) tanpa komentar alasan

Rating (A–E) per dimensi:
- **Reliability** (Bug): A = 0 bug, B = ada MINOR, C = ada MAJOR, D = ada CRITICAL, E = ada BLOCKER
- **Security** (Vulnerability): skala sama
- **Maintainability**: berdasarkan estimasi technical debt ratio, A ≤ 5%, B ≤ 10%, C ≤ 20%, D ≤ 50%, E > 50%

## Format Laporan
```
## Quality Gate: PASSED | FAILED
Reliability: A | Security: A | Maintainability: B | Coverage: 82% | Duplication: 1.2%

### Temuan
1. [CRITICAL][Vulnerability] internal/repo/user.go:42
   Query dibangun dengan string concat → SQL injection.
   Fix: pakai parameterized query ($1) / sqlc.
2. ...

### Ringkasan
- Total: X bug, Y vuln, Z smell
- Prioritas perbaikan: ...
```
Aturan format:
- Selalu sertakan `path:line` dan alasan singkat + saran fix konkret.
- Urutkan dari severity tertinggi.
- Jangan laporkan hal yang murni selera gaya kalau sudah ditangani `gofmt`/linter.
- Kalau tidak yakin (false positive mungkin), tandai `[Perlu konfirmasi]`, jangan dipaksakan.

## Checklist Review Manual (di luar tools)
- Context propagation: `context.Context` diteruskan, ada timeout untuk call eksternal/DB
- Transaksi DB: commit/rollback jelas, tidak ada transaksi menggantung
- Concurrency: shared state dilindungi, goroutine punya cara berhenti
- Error handling: di-wrap dengan `%w`, tidak ada swallow error, tidak ada panic di alur normal
- Layering: handler → usecase → repository, tidak ada dependency terbalik (Clean Architecture)
- Idempotency & race pada operasi uang/stok
- Query: N+1, index yang kurang, `SELECT *`
- Logging: tidak membocorkan token/PII

## Khusus Claude Code
- Saat diminta "quality check", "review", atau "analisis", jalankan alur di atas dalam mode baca saja. Jangan edit file sebelum diminta.
- Untuk perbaikan yang menyentuh lebih dari 3 file, pakai plan mode dulu dan tunggu persetujuan.
- Jalankan lint/test dari root repo, dan selalu tampilkan ringkasan hasil nyata (bukan asumsi) di laporan.

## Larangan
- Jangan menonaktifkan rule linter (`//nolint`) atau menurunkan threshold untuk "meloloskan" gate.
- Jangan menghapus/melemahkan test supaya coverage atau hasil terlihat bagus.
- Jangan commit atau push perubahan kecuali diminta.
