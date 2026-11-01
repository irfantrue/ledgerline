# Modul ledger

Ledger `ledgerline` adalah buku besar **double-entry**: setiap transaksi ekonomi direkam sebagai satu *journal* yang berisi beberapa *posting*, dan journal itu harus balance — total debit = total kredit.

## Konsep

| Entity | Peran | Field penting |
|---|---|---|
| `Account` | Akun buku besar | `currency`, `normal_side`, `allow_negative`, `balance`, `version` |
| `Journal` | Header transaksi | `idempotency_key`, `reference`, `description` |
| `Posting` | Baris debit/kredit dalam journal | `journal_id`, `account_id`, `direction`, `amount`, `currency`, `balance_after` |
| `JournalWithPostings` | Convenience struct journal + postings-nya | |
| `PostingsSummary` | Agregasi posting per akun | `posting_count`, `total_debits`, `total_credits`, `balance_after` |

### Normal side vs direction

`normal_side` = sisi di mana saldo akun normal. `direction` = sisi yang dipakai satu posting.

| Tipe akun | `normal_side` | Bertambah saat |
|---|---|---|
| Asset, Expense | `debit` | debit |
| Liability, Equity, Revenue | `credit` | credit |

Saldo selalu positif di sisi normal. Posting ke sisi berlawanan mengurangi saldo, dan saldo bisa negatif hanya kalau `allow_negative = true` (dijaga CHECK constraint `balance_non_negative` di DB).

### Balance after

`balance_after` **tidak dikirim dari aplikasi**. Field ini dihitung trigger database (`ledger.set_posting_balance_after`) yang mengunci baris akun dengan `FOR UPDATE` lalu menjumlahkan `balance` lama dengan delta posting. Aplikasi tidak mungkin mengirim nilai yang basi atau salah.

## Idempotensi

`ledger.journals.idempotency_key` UNIQUE. Query `CreateJournal` memakai:

```sql
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING *
```

Kalau key-nya sudah dipakai, INSERT mengembalikan 0 baris → `pgx.ErrNoRows` → repository membaca journal yang sudah ada lewat `GetJournalByIdempotencyKey` dan mengembalikannya. Jadi retry dengan key yang sama aman dan idempoten. `ON CONFLICT DO UPDATE` tidak boleh dipakai di sini karena `ledger.journals` menolak UPDATE.

Key dikirim klien (UUID dari klien, atau ID bisnis).

## Menulis journal yang balance

Pengecekan balance adalah **constraint trigger `DEFERRABLE INITIALLY DEFERRED`**, jadi ia baru dievaluasi saat COMMIT. Konsekuensinya: menulis satu posting saja dalam satu transaksi **pasti gagal** — journal-nya belum balance saat commit.

Posting untuk satu journal wajib ditulis dalam satu transaksi:

```go
tm := database.NewTxManager(pool)

err := tm.Do(ctx, func(ctx context.Context) error {
    journal, err := journals.Create(ctx, repository.CreateJournalParams{
        IdempotencyKey: key,
        Description:    "penjualan",
    })
    if err != nil {
        return fmt.Errorf("create journal: %w", err)
    }

    // Debit akun kas (normal side debit, saldo bertambah)
    if _, err := postings.Create(ctx, repository.CreatePostingParams{
        JournalID: journal.ID,
        AccountID: cashAccount.ID,
        Direction: domain.DirectionDebit,
        Amount:    decimal.RequireFromString("100000.00"),
        Currency:  cashAccount.Currency,
    }); err != nil {
        return fmt.Errorf("debit posting: %w", err)
    }

    // Kredit akun pendapatan (normal side credit, saldo bertambah)
    if _, err := postings.Create(ctx, repository.CreatePostingParams{
        JournalID: journal.ID,
        AccountID: revenueAccount.ID,
        Direction: domain.DirectionCredit,
        Amount:    decimal.RequireFromString("100000.00"),
        Currency:  revenueAccount.Currency,
    }); err != nil {
        return fmt.Errorf("credit posting: %w", err)
    }

    return nil
})
```

Transaction manager otomatis menangani commit/rollback; tidak perlu `sqlcgen.Queries` per transaksi manual.

> `amount` selalu positif — tanda ditentukan oleh `direction`. CHECK constraint `amount > 0` juga ada di DB.

## Repository

Tiga repository di `internal/ledger/infrastructure/repository/`:

| Interface | Method |
|---|---|
| `AccountRepository` | `Create`, `GetByID`, `GetByCurrency`, `List` |
| `JournalRepository` | `Create`, `GetByID`, `GetByIdempotencyKey`, `List`, `GetWithPostings` |
| `PostingRepository` | `Create`, `GetByID`, `List`, `ListByAccount`, `ListByJournal`, `GetSummaryByAccount` |

Pola umum:

- Handle DB diselesaikan per call lewat `Querier` → ikut transaksi yang ada di context.
- Error di-wrap dengan `%w` dan konteks (`fmt.Errorf("posting: create: %w", err)`).
- `pgx.ErrNoRows` dipetakan ke sentinel error: `ErrAccountNotFound`, `ErrJournalNotFound`, `ErrPostingNotFound`. Caller wajib mendeteksinya dengan `errors.Is`, bukan perbandingan langsung.
- List memakai `LIMIT NULLIF(@limit_rows::int, 0)` → `limit = 0` berarti ambil semua (tidak disarankan untuk dataset besar). Urutan default `created_at DESC` (terbaru dulu); posting dalam satu journal diurutkan `created_at ASC`.

### GetWithPostings

Query memakai `json_agg` + `json_build_object`, dengan nominal di-cast ke `::text` supaya presisi `NUMERIC(18,2)` tidak hilang saat JSON number di-decode sebagai `float64`. `repository.ParsePostings` mendecode payload itu; entry yang rusak dilaporkan sebagai error, tidak diam-diam dibuang — payload korup tidak boleh tertukar dengan journal kosong.

### GetPostingsSummaryByAccount

`balance_after` diambil dari tabel `accounts`, bukan dari `postings.balance_after`. Kalau dikelompokkan berdasarkan `postings.balance_after`, setiap saldo berbeda jadi satu baris dan varian `:one` akan diam-diam hanya menyimpan baris pertama.

## Layering di dalam modul

```
domain/entity.go        entity + enum NormalSide, Direction (tanpa import infrastructure)
infrastructure/
  queries/*.sql         SQL sumber untuk sqlc
  sqlcgen/             hasil generate — jangan diedit manual
  repository/           interface persistensi + implementasi PostgreSQL
```

Domain tidak boleh import infrastructure. `repository` mendefinisikan interface, implementasi ada di sebelahnya. `sqlcgen` adalah output generator.

## Yang belum ada

Modul ledger belum punya handler HTTP maupun lapisan use-case. Yang tersedia sekarang adalah package `repository` (API Go). Endpoint `/healthz` dan `/readyz` adalah satu-satunya route yang terdaftar.
