# Testing

## Aturan utama

- **Test tinggal di `tests/`, bukan di sebelah kode.** Pohon test mencerminkan source tree:

| Source | Test |
|---|---|
| `internal/ledger/` | `tests/ledger/` |
| `internal/shared/config/` | `tests/shared/config/` |
| `internal/shared/httpx/middleware/` | `tests/shared/httpx/middleware/` |
| `pkg/jwt/` | `tests/pkg/jwt/` |

- Karena package test berada di luar kode yang diuji, test hanya bisa memakai identifier **exported**. Kalau test butuh helper, export helper-nya — jangan memindahkan test balik ke `internal/`.
- **Tidak ada service yang dibutuhkan.** Seluruh suite berjalan tanpa PostgreSQL, Redis, dan RabbitMQ: dependency-nya dimock lewat `testify/mock`.
- `internal/` dan `pkg/` bebas file test.

## Framework dan alat

- Assertion: `github.com/stretchr/testify/assert` (pemeriksaan nilai) dan `github.com/stretchr/testify/require` (precondition yang harus menghentikan test).
- Mock: `github.com/stretchr/testify/mock`. Setiap package sumber yang punya interface punya package mock sendiri di `tests/mocks/`, jadi double diimpor dari tempat yang memiliki interface-nya.
- Generator mock: [mockery](https://github.com/vektra/mockery) v3, dikonfigurasi di `.mockery.yaml`.
- Test tetap table-driven kalau memang ada banyak kasus; `httptest` untuk yang menyentuh HTTP.

## Mocking

Mock di-generate dengan mockery, bukan ditulis tangan. Konfigurasinya ada di `.mockery.yaml`; file hasil generate tinggal di `tests/mocks/` dan ikut di-commit, jadi clone baru bisa build tanpa mockery terpasang.

Setiap package sumber punya direktori sendiri, dengan nama package yang berbeda dari package sumbernya. Jadi `database.DBTX` diimpor sebagai `databasemocks.DBTX`, bukan sebagai `mocks.DBTX`.

```bash
make mocks      # regenerate setelah interface berubah
```

| Package mock | Double | Berperan sebagai |
|---|---|---|
| `databasemocks` | `DBTX` | `database.DBTX` — handle database |
| `databasemocks` | `Pool` | `database.Pool` — pool yang menyerahkan transaksi mock |
| `repositorymocks` | `Querier` | `repository.Querier` — resolver handle per call |
| `pgxmocks` | `Tx` | `pgx.Tx` — transaksi |
| `pgxmocks` | `Row`, `Rows` | `pgx.Row`, `pgx.Rows` — baris dan result set |
| `cachemocks` | `Backend` | `cache.Backend` — perintah Redis |
| `brokermocks` | `Channel`, `Connection`, `Confirmation` | channel, connection, dan konfirmasi publisher AMQP |
| `amqpmocks` | `Acknowledger` | `amqp.Acknowledger` — mencatat ack/nack delivery |
| `httpxmocks` | `Pinger` | `httpx.Pinger` — handle database untuk readiness probe |

Mock interface pakai pola testify: `On(...)` menyiapkan expectation, `Return(...)` memberi hasil, `AssertExpectations(t)` memverifikasi panggilan terjadi.

### Fixture di atas mock generated

Mock generated hanya tahu bentuk method-nya, bukan isi result set. Fixture meletakkan diri di package mock yang dikonfigurasi:

| Helper | Package | Kegunaan |
|---|---|---|
| `RowValues(values)` | `pgxmocks` | baris yang menyerahkan nilai ke destination, seperti driver |
| `ErrRow(err)` | `pgxmocks` | baris yang gagal di setiap scan |
| `RowsValues(rows...)` | `pgxmocks` | result set dengan beberapa baris |
| `ErrRows(err)` | `pgxmocks` | result set yang gagal |
| `MatchSQL(fragment)` | `databasemocks` | pencocokan statement berdasarkan potongan teks |
| `Delivery(ack, body)` | `amqpmocks` | delivery AMQP yang melapor ke acknowledger |

Helper ini hanya berisi setup expectation — tidak ada mock interface yang ditulis tangan.

Untuk kode yang punya side effect (goroutine, callback), test broker menyinkronkan lewat channel yang ditutup saat settlement terjadi, bukan menebak durasi sleep.

### Mocking butuh seam di production code

`internal/shared/` sengaja punya interface kecil agar implementasi bisa dimock:

- `database.Pool` — `DBTX` plus `BeginTx`.
- `cache.Backend` — perintah Redis yang dipakai cache, dengan nilai berupa byte mentah.
- `broker.Channel` dan `broker.Connection` — subset dari API AMQP yang dipakai broker.
- `broker.NewRabbitMQWithDialer` — constructor yang menerima dialer, jadi test bisa memberi connection palsu.

Kalau production code perlu mock baru, tambahkan interface di package-nya, bukan pakai tipe konkret.

## Unit test

Tidak butuh service apa pun:

```bash
make test          # go test -v -race ./...
```

## Invariant database

Invariant ledger hidup di database (trigger dan constraint), jadi tidak bisa dimock. Test-nya membaca file migration dan memastikan aturannya masih ada:

- `tests/ledger/migration_schema_test.go` — struktur tabel dan constraint.
- `tests/ledger/ledger_invariants_test.go` — aturan trigger: append-only, balance guard, cek currency, dan balance yang didefer ke COMMIT.

Test ini memeriksa definisi invariant, bukan perilakunya terhadap data nyata. Kalau aturannya hilang dari migration, test gagal di sini.

## Coverage

```bash
make coverage       # gate: gagal di bawah COVERAGE_MIN (default 80)
make test-coverage  # tulis coverage.html tanpa gate
```

`make coverage` memakai `-coverpkg=./...` karena test berada di `tests/` — tanpa itu, default hanya mengukur package test itu sendiri.

Pengecualian dari pengukuran (lihat `COVERAGE_EXCLUDE` di `Makefile`):

- `ledgerline/cmd/api` — hanya wiring aplikasi.
- `ledgerline/internal/ledger/infrastructure/sqlcgen` — kode generated.
- `ledgerline/tests/mocks` — scaffolding test double, bukan kode produksi.

`scripts/coverage.sh` dibaca dari profile, difilter, lalu totalnya dibandingkan dengan minimum.

## Di mana test berjalan

| Lingkungan | Yang jalan |
|---|---|
| CI | Seluruh suite — tidak butuh service |
| Server dev | Seluruh suite + gate coverage |

## Test yang mengunci mekanisme

`tests/ledger/repository_querier_test.go` membuktikan repository meminta handle **per call**, bukan menyimpan satu handle. Itulah mekanismenya yang membuat satu call bisa ikut transaksi yang dibawa context-nya — dan itu yang dibutuhkan trigger balance yang deferrable.
