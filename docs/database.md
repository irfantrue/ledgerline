# Database

PostgreSQL 18+, schema `ledger`. Migrasi ada di `migrations/ledger/`, SQL query sqlc ada di `internal/ledger/infrastructure/queries/`.

## Dua sumber kebenaran

| File | Isi | Edit manual? |
|---|---|---|
| `migrations/ledger/*.sql` | Sumber kebenaran skema yang benar-benar di-deploy | ya |
| `internal/ledger/infrastructure/queries/schema.sql` | Skema yang dibaca sqlc untuk type inference | ya, tapi harus **mirror** migrasi |
| `internal/ledger/infrastructure/queries/{account,journal,posting}.sql` | Query sqlc | ya |
| `internal/ledger/infrastructure/sqlcgen/` | Hasil `sqlc generate` | **jangan** |

Kalau tabel berubah, ubah `001_ledger.sql` **dan** `queries/schema.sql`, lalu `sqlc generate`.

## Migration

| File | Isi |
|---|---|
| `001_ledger.sql` | `CREATE SCHEMA`, tabel, index |
| `002_ledger_triggers.sql` | Semua trigger invariant |

Aturan:

- Nomor file mengikuti nomor tertinggi yang ada, format `NNN_snake_case.sql`.
- Plain SQL, idempotent (`CREATE ... IF NOT EXISTS`, `CREATE OR REPLACE FUNCTION`, `DROP TRIGGER IF EXISTS`).
- Tabel/index baru → `001`. Logika trigger baru → `002`.
- Apply berurutan:

```bash
for f in migrations/ledger/*.sql; do psql "$DB_DSN" -f "$f"; done
```

## Tabel

### `ledger.accounts`

| Kolom | Tipe | Constraint |
|---|---|---|
| `id` | UUID | PK, default `uuidv7()` |
| `currency` | TEXT | NOT NULL, `char_length = 3` (ISO 4217) |
| `normal_side` | TEXT | NOT NULL, IN (`debit`, `credit`) |
| `allow_negative` | BOOLEAN | NOT NULL, default FALSE |
| `balance` | NUMERIC(18,2) | NOT NULL, default 0 |
| `version` | BIGINT | NOT NULL, default 0 |
| `created_at` / `updated_at` | TIMESTAMPTZ | NOT NULL, default `now()` |

Constraint `balance_non_negative`: `allow_negative OR balance >= 0`.

`version` adalah penghitung revisi saldo — incremented setiap posting mengubah saldo. **Bukan** token compare-and-swap; tidak ada jalur update akun yang memakainya sebagai perbandingan.

### `ledger.journals`

| Kolom | Tipe | Constraint |
|---|---|---|
| `id` | UUID | PK, default `uuidv7()` |
| `idempotency_key` | TEXT | NOT NULL, UNIQUE |
| `reference` | UUID | NULL — referensi entitas lain (invoice ID, order ID) |
| `description` | TEXT | NOT NULL, default `''` |
| `created_at` | TIMESTAMPTZ | NOT NULL, default `now()` |

### `ledger.postings`

| Kolom | Tipe | Constraint |
|---|---|---|
| `id` | UUID | PK, default `uuidv7()` |
| `journal_id` | UUID | NOT NULL, FK → `journals(id)` |
| `account_id` | UUID | NOT NULL, FK → `accounts(id)` |
| `direction` | TEXT | NOT NULL, IN (`debit`, `credit`) |
| `amount` | NUMERIC(18,2) | NOT NULL, `> 0` |
| `currency` | TEXT | NOT NULL, `char_length = 3` |
| `balance_after` | NUMERIC(18,2) | NOT NULL — **dihitung trigger**, bukan dari aplikasi |
| `created_at` | TIMESTAMPTZ | NOT NULL, default `now()` |

### Index

```sql
CREATE INDEX idx_postings_account  ON ledger.postings (account_id, id);
CREATE INDEX idx_postings_journal  ON ledger.postings (journal_id);
CREATE INDEX idx_journals_reference ON ledger.journals (reference);
```

FK hanya di dalam schema `ledger`, tidak lintas modul.

## Inventaris trigger

Semua invariant ada di database, jadi berlaku untuk writer mana pun — bukan cuma aplikasi ini.

### Append-only

| Trigger | Timing | Fungsi |
|---|---|---|
| `journals_append_only` | BEFORE UPDATE OR DELETE, per row | `ledger.reject_mutation()` → raise "append-only" |
| `postings_append_only` | BEFORE UPDATE OR DELETE, per row | idem |
| `journals_no_truncate` | BEFORE TRUNCATE, per statement | idem |
| `postings_no_truncate` | BEFORE TRUNCATE, per statement | idem |

Trigger per-row tidak menyala pada `TRUNCATE`, jadi butuh trigger terpisah di level statement.

### Balance

| Trigger | Timing | Fungsi |
|---|---|---|
| `postings_balanced` | AFTER INSERT OR DELETE, **CONSTRAINT TRIGGER DEFERRABLE INITIALLY DEFERRED** | `ledger.ensure_balanced_postings()` |

Hitung `SUM(amount)` per `direction` untuk journal terkait; kalau debit ≠ kredit, raise. Karena deferrable, pengecekan berjalan saat COMMIT —posting satu journal boleh ditulis satu per satu dalam satu transaksi.

### Derivasi saldo

| Trigger | Timing | Fungsi |
|---|---|---|
| `postings_set_balance_after` | BEFORE INSERT | `ledger.set_posting_balance_after()` |
| `postings_update_balance` | AFTER INSERT | `ledger.update_account_balance()` |

`set_posting_balance_after` mengunci baris akun dengan `FOR UPDATE`, lalu menghitung:

```
delta = +amount  bila (normal_side = debit  AND direction = debit)
      = -amount  bila (normal_side = debit  AND direction = credit)
      = +amount  bila (normal_side = credit AND direction = credit)
      = -amount  bila (normal_side = credit AND direction = debit)

balance_after = accounts.balance + delta
```

Lock itu penting: tanpa `FOR UPDATE`, dua posting ke akun yang sama bisa membaca saldo awal yang sama lalu berbeda pendapat tentang hasil. Dengan lock, posting kedua menunggu transaksi pertama lalu membaca saldo yang ditinggalkan transaksi itu.

Trigger `postings_set_balance_after` dinamai sengaja supaya urutannya **setelah** `postings_currency_check`: PostgreSQL menjalankan trigger satu event dalam urutan alfabetik, jadi posting dengan mata uang salah ditolak sebelum trigger ini mengambil lock akun.

`update_account_balance` menulis `accounts.balance = NEW.balance_after`, `updated_at = now()`, `version = version + 1`.

### Penjaga saldo

| Trigger | Timing | Fungsi |
|---|---|---|
| `accounts_balance_guard` | BEFORE UPDATE pada accounts | `ledger.reject_balance_update()` |

Menolak `UPDATE` langsung terhadap `accounts.balance`, `accounts.currency`, dan `accounts.normal_side`. Yang diperbolehkan: INSERT (akun baru) dan DELETE (cascade).

Trik `pg_trigger_depth() > 1`: saat dipanggil dari dalam trigger lain (`ledger.update_account_balance`), depth > 1 dan update diizinkan — itu memang jalur yang sah. `UPDATE` langsung depth 1 → ditolak.

### Konsistensi mata uang

| Trigger | Timing | Fungsi |
|---|---|---|
| `postings_currency_check` | BEFORE INSERT | `ledger.ensure_currency_match()` |

`posting.currency` harus sama dengan `currency` akun. Akun yang tidak ada → raise "account % does not exist".

## sqlc

`sqlc.yaml`:

```yaml
schema: internal/ledger/infrastructure/queries   # schema.sql untuk type inference
queries: internal/ledger/infrastructure/queries
out: internal/ledger/infrastructure/sqlcgen
sql_package: pgx/v5
```

Override tipe:

| db_type | go_type |
|---|---|
| `uuid` | `github.com/google/uuid.UUID` |
| `timestamptz` | `time.Time` |
| `pg_catalog.numeric` | `github.com/shopspring/decimal.Decimal` |

Alur:

```bash
# edit internal/ledger/infrastructure/queries/*.sql
sqlc generate

# kalau import Go berubah:
make tidy && make vendor
```

Hasil `sqlc generate` harus identik dengan yang sudah ada di repo (deterministik).

## Peringatan

Schema `ledger` di-drop dan di-create ulang oleh test integrasi (`DROP SCHEMA IF EXISTS ledger CASCADE`). `DB_DSN` untuk test **wajib** menunjuk database buang, bukan database produksi/dev yang berisi data.
