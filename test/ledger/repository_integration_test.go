package ledger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ledgerline/internal/ledger/domain"
	"ledgerline/internal/ledger/infrastructure/repository"
	"ledgerline/internal/shared/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Integration tests need a real PostgreSQL. Point DB_DSN at a throwaway
// database, e.g.:
//
//	DB_DSN='postgres://postgres:postgres@localhost:5432/app_test?sslmode=disable' go test ./internal/ledger/...
//
// The migrations are applied automatically and are re-runnable, so the same
// database can be reused. Without DB_DSN these tests are skipped, so plain
// `make test` stays DB-free.
//
// Every posting has to belong to a balanced journal, and the balance is
// verified when the transaction commits. Tests that write postings therefore
// run inside a transaction that also writes the balancing entry.

var (
	dbOnce sync.Once
	dbPool *pgxpool.Pool
	dbErr  error
)

// testDB returns the pool shared by the integration tests, applying the
// migrations once per test binary.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN not set; skipping integration test")
	}

	dbOnce.Do(func() {
		ctx := context.Background()

		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			dbErr = fmt.Errorf("connect: %w", err)
			return
		}

		dbPool = pool
		if dbErr = resetLedgerSchema(ctx, pool); dbErr != nil {
			pool.Close()
			dbPool = nil
		}
	})

	if dbErr != nil {
		t.Fatalf("test database: %v", dbErr)
	}
	return dbPool
}

// resetLedgerSchema drops the ledger schema and applies every migration from
// scratch, so each test run starts from a known state.
//
// The ledger schema is owned by these tests: DB_DSN must point at a throwaway
// database, because everything in the schema is dropped and recreated here.
func resetLedgerSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS ledger CASCADE"); err != nil {
		return fmt.Errorf("drop ledger schema: %w", err)
	}
	return applyMigrations(ctx, pool)
}

// applyMigrations executes every migration in migrations/ledger in file order.
// The migrations are plain SQL and are idempotent.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir := filepath.Join("..", "..", "migrations", "ledger")

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		// The path is built from the repository's own migrations directory.
		script, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // G304: the path comes from the migrations directory listed above
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(script)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

// testRepos bundles the three repositories under test.
type testRepos struct {
	accounts repository.AccountRepository
	journals repository.JournalRepository
	postings repository.PostingRepository
	pool     *pgxpool.Pool
}

// newTestRepos returns repositories backed by the shared test database.
//
// The repositories resolve their handle per call through the transaction
// manager, so they pick up the transaction that the context of a call
// carries. That is the same mechanism production uses.
func newTestRepos(t *testing.T) testRepos {
	t.Helper()

	pool := testDB(t)
	tm := database.NewTxManager(pool)

	return testRepos{
		accounts: repository.NewAccountRepository(tm),
		journals: repository.NewJournalRepository(tm),
		postings: repository.NewPostingRepository(tm),
		pool:     pool,
	}
}

// currencySeq backs uniqueCurrency. ledger.accounts.currency is limited to
// three characters by a CHECK constraint, so the counter is encoded as three
// base-26 letters.
var currencySeq atomic.Uint64

// uniqueCurrency returns a currency code that no other test uses.
func uniqueCurrency() string {
	n := currencySeq.Add(1)
	return string([]byte{
		byte('A' + n%26),
		byte('A' + (n/26)%26),
		byte('A' + (n/676)%26),
	})
}

// uniqueIdempotencyKey returns a journal idempotency key unique to this run.
func uniqueIdempotencyKey(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%s-%d", t.Name(), currencySeq.Add(1))
}

// newAccount creates a debit-normal account with a currency unique to the
// caller.
func newAccount(t *testing.T, repos testRepos) *domain.Account {
	t.Helper()

	account, err := repos.accounts.Create(context.Background(), repository.CreateAccountParams{
		Currency:   uniqueCurrency(),
		NormalSide: domain.NormalSideDebit,
	})
	if err != nil {
		t.Fatalf("Create account: %v", err)
	}
	return account
}

// newJournal creates a journal with an idempotency key unique to the caller.
func newJournal(t *testing.T, repos testRepos) *domain.Journal {
	t.Helper()

	journal, err := repos.journals.Create(context.Background(), repository.CreateJournalParams{
		IdempotencyKey: uniqueIdempotencyKey(t),
		Description:    "integration test",
	})
	if err != nil {
		t.Fatalf("Create journal: %v", err)
	}
	return journal
}

// inTx runs fn inside a transaction and commits it. The balance trigger is
// deferred to COMMIT, so a journal that fn leaves unbalanced is rejected when
// the transaction is committed.
func inTx(t *testing.T, repos testRepos, fn func(ctx context.Context) error) {
	t.Helper()

	ctx := context.Background()

	tx, err := repos.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(database.ContextWithTx(ctx, tx)); err != nil {
		t.Fatalf("transaction: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
}

// tryInTx runs fn inside a transaction and returns the error instead of
// failing the test. It is used for the cases where an error is expected.
func tryInTx(t *testing.T, repos testRepos, fn func(ctx context.Context) error) error {
	t.Helper()

	ctx := context.Background()

	tx, err := repos.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(database.ContextWithTx(ctx, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// createBalancedPostings writes a journal with one posting per entry inside a
// single transaction and returns the created journal and postings.
func createBalancedPostings(t *testing.T, repos testRepos, amount string) (*domain.Journal, []*domain.Posting) {
	t.Helper()

	var (
		journal  *domain.Journal
		postings []*domain.Posting
	)

	inTx(t, repos, func(ctx context.Context) error {
		journals := repos.journals
		postingsRepo := repos.postings
		accounts := repos.accounts

		created, err := journals.Create(ctx, repository.CreateJournalParams{
			IdempotencyKey: uniqueIdempotencyKey(t),
			Description:    "balanced journal",
		})
		if err != nil {
			return fmt.Errorf("create journal: %w", err)
		}
		journal = created

		value := decimal.RequireFromString(amount)

		debitAccount, err := accounts.Create(ctx, repository.CreateAccountParams{
			Currency:   uniqueCurrency(),
			NormalSide: domain.NormalSideDebit,
		})
		if err != nil {
			return fmt.Errorf("create debit account: %w", err)
		}

		creditAccount, err := accounts.Create(ctx, repository.CreateAccountParams{
			Currency:   uniqueCurrency(),
			NormalSide: domain.NormalSideCredit,
		})
		if err != nil {
			return fmt.Errorf("create credit account: %w", err)
		}

		debit, err := postingsRepo.Create(ctx, repository.CreatePostingParams{
			JournalID: journal.ID,
			AccountID: debitAccount.ID,
			Direction: domain.DirectionDebit,
			Amount:    value,
			Currency:  debitAccount.Currency,
		})
		if err != nil {
			return fmt.Errorf("create debit posting: %w", err)
		}

		credit, err := postingsRepo.Create(ctx, repository.CreatePostingParams{
			JournalID: journal.ID,
			AccountID: creditAccount.ID,
			Direction: domain.DirectionCredit,
			Amount:    value,
			Currency:  creditAccount.Currency,
		})
		if err != nil {
			return fmt.Errorf("create credit posting: %w", err)
		}

		postings = []*domain.Posting{debit, credit}
		return nil
	})

	return journal, postings
}

// createPosting writes a single posting to the given account together with a
// balancing entry on the opposite side, so the journal balances.
func createPosting(t *testing.T, repos testRepos, account *domain.Account, direction domain.Direction, amount decimal.Decimal) *domain.Posting {
	t.Helper()

	var posting *domain.Posting

	inTx(t, repos, func(ctx context.Context) error {
		journals := repos.journals
		postingsRepo := repos.postings
		accounts := repos.accounts

		journal, err := journals.Create(ctx, repository.CreateJournalParams{
			IdempotencyKey: uniqueIdempotencyKey(t),
			Description:    "balanced posting",
		})
		if err != nil {
			return fmt.Errorf("create journal: %w", err)
		}

		oppositeSide := domain.NormalSideCredit
		oppositeDirection := domain.DirectionCredit
		if direction == domain.DirectionCredit {
			oppositeSide = domain.NormalSideDebit
			oppositeDirection = domain.DirectionDebit
		}

		opposite, err := accounts.Create(ctx, repository.CreateAccountParams{
			Currency:   uniqueCurrency(),
			NormalSide: oppositeSide,
		})
		if err != nil {
			return fmt.Errorf("create opposite account: %w", err)
		}

		posting, err = postingsRepo.Create(ctx, repository.CreatePostingParams{
			JournalID: journal.ID,
			AccountID: account.ID,
			Direction: direction,
			Amount:    amount,
			Currency:  account.Currency,
		})
		if err != nil {
			return fmt.Errorf("create posting: %w", err)
		}

		if _, err := postingsRepo.Create(ctx, repository.CreatePostingParams{
			JournalID: journal.ID,
			AccountID: opposite.ID,
			Direction: oppositeDirection,
			Amount:    amount,
			Currency:  opposite.Currency,
		}); err != nil {
			return fmt.Errorf("create opposite posting: %w", err)
		}

		return nil
	})

	return posting
}

func TestAccountRepository_Create(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	currency := uniqueCurrency()
	account, err := repos.accounts.Create(ctx, repository.CreateAccountParams{
		Currency:      currency,
		NormalSide:    domain.NormalSideDebit,
		AllowNegative: false,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if account.ID == uuid.Nil {
		t.Error("Create() returned a nil UUID")
	}
	if account.Currency != currency {
		t.Errorf("Currency = %q, want %q", account.Currency, currency)
	}
	if account.NormalSide != domain.NormalSideDebit {
		t.Errorf("NormalSide = %q, want %q", account.NormalSide, domain.NormalSideDebit)
	}
	if !account.Balance.IsZero() {
		t.Errorf("Balance = %s, want 0", account.Balance)
	}
	if account.Version != 0 {
		t.Errorf("Version = %d, want 0", account.Version)
	}
	if account.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if account.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
}

func TestAccountRepository_Create_RejectsInvalidValues(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		params repository.CreateAccountParams
	}{
		{
			name:   "currency too short",
			params: repository.CreateAccountParams{Currency: "US", NormalSide: domain.NormalSideDebit},
		},
		{
			name:   "currency too long",
			params: repository.CreateAccountParams{Currency: "USDX", NormalSide: domain.NormalSideDebit},
		},
		{
			name:   "unknown normal side",
			params: repository.CreateAccountParams{Currency: currencyUSD, NormalSide: domain.NormalSide("sideways")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := repos.accounts.Create(ctx, tt.params); err == nil {
				t.Fatal("Create() error = nil, want a constraint violation")
			}
		})
	}
}

func TestAccountRepository_GetByID(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	created := newAccount(t, repos)

	got, err := repos.accounts.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %v, want %v", got.ID, created.ID)
	}
	if got.Currency != created.Currency {
		t.Errorf("Currency = %q, want %q", got.Currency, created.Currency)
	}
}

func TestAccountRepository_GetByID_NotFound(t *testing.T) {
	repos := newTestRepos(t)

	_, err := repos.accounts.GetByID(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("GetByID() error = nil, want repository.ErrAccountNotFound")
	}
	if !errors.Is(err, repository.ErrAccountNotFound) {
		t.Errorf("GetByID() error = %v, want repository.ErrAccountNotFound", err)
	}
}

func TestAccountRepository_GetByCurrency(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	created := newAccount(t, repos)

	got, err := repos.accounts.GetByCurrency(ctx, created.Currency)
	if err != nil {
		t.Fatalf("GetByCurrency() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %v, want %v", got.ID, created.ID)
	}
}

func TestAccountRepository_GetByCurrency_NotFound(t *testing.T) {
	repos := newTestRepos(t)

	_, err := repos.accounts.GetByCurrency(context.Background(), "ZZZ")
	if err == nil {
		t.Fatal("GetByCurrency() error = nil, want repository.ErrAccountNotFound")
	}
	if !errors.Is(err, repository.ErrAccountNotFound) {
		t.Errorf("GetByCurrency() error = %v, want repository.ErrAccountNotFound", err)
	}
}

func TestAccountRepository_List(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	currency := uniqueCurrency()
	created := make([]*domain.Account, 0, 3)
	for i := 0; i < 3; i++ {
		account, err := repos.accounts.Create(ctx, repository.CreateAccountParams{
			Currency:   currency,
			NormalSide: domain.NormalSideDebit,
		})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		created = append(created, account)
		// ledger.accounts.created_at defaults to now(), so keep the inserts
		// far enough apart for the ordering to be unambiguous.
		time.Sleep(2 * time.Millisecond)
	}

	// limit = 0 means "no limit".
	all, err := repos.accounts.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) < len(created) {
		t.Fatalf("List() returned %d accounts, want at least %d", len(all), len(created))
	}

	positions := make([]int, 0, len(created))
	for _, want := range created {
		index := indexOfAccount(all, want.ID)
		if index < 0 {
			t.Fatalf("List() does not contain the created account %s", want.ID)
		}
		positions = append(positions, index)
	}

	// The accounts are ordered by created_at DESC, so the last created one
	// comes first.
	for i := 1; i < len(positions); i++ {
		if positions[i-1] <= positions[i] {
			t.Errorf("List() order = %v, want strictly descending created_at", positions)
		}
	}
}

func TestAccountRepository_List_Pagination(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	all, err := repos.accounts.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) < 3 {
		t.Skipf("need at least 3 accounts to paginate, have %d", len(all))
	}

	page, err := repos.accounts.List(ctx, 2, 1)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("len(page) = %d, want 2", len(page))
	}
	for i := range page {
		if page[i].ID != all[i+1].ID {
			t.Errorf("page[%d].ID = %v, want %v", i, page[i].ID, all[i+1].ID)
		}
	}
}

func TestJournalRepository_Create(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	key := uniqueIdempotencyKey(t)
	journal, err := repos.journals.Create(ctx, repository.CreateJournalParams{
		IdempotencyKey: key,
		Description:    descInvoice,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if journal.ID == uuid.Nil {
		t.Error("Create() returned a nil UUID")
	}
	if journal.IdempotencyKey != key {
		t.Errorf("IdempotencyKey = %q, want %q", journal.IdempotencyKey, key)
	}
	if journal.Description != descInvoice {
		t.Errorf("Description = %q, want %q", journal.Description, descInvoice)
	}
	if journal.Reference != nil {
		t.Errorf("Reference = %v, want nil", journal.Reference)
	}
}

func TestJournalRepository_Create_WithReference(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	reference := uuid.New()
	journal, err := repos.journals.Create(ctx, repository.CreateJournalParams{
		IdempotencyKey: uniqueIdempotencyKey(t),
		Reference:      &reference,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if journal.Reference == nil {
		t.Fatal("Reference = nil, want a value")
	}
	if *journal.Reference != reference {
		t.Errorf("Reference = %v, want %v", *journal.Reference, reference)
	}
}

// TestJournalRepository_Create_IsIdempotent covers the documented contract:
// replaying the same idempotency key returns the original journal instead of
// creating a duplicate.
func TestJournalRepository_Create_IsIdempotent(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	key := uniqueIdempotencyKey(t)

	first, err := repos.journals.Create(ctx, repository.CreateJournalParams{
		IdempotencyKey: key,
		Description:    "first",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	second, err := repos.journals.Create(ctx, repository.CreateJournalParams{
		IdempotencyKey: key,
		Description:    "second",
	})
	if err != nil {
		t.Fatalf("Create() with a duplicate key error = %v, want the original journal", err)
	}
	if second.ID != first.ID {
		t.Errorf("ID = %v, want the original journal %v", second.ID, first.ID)
	}
	if second.Description != "first" {
		t.Errorf("Description = %q, want %q", second.Description, "first")
	}
}

func TestJournalRepository_GetByID(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	created := newJournal(t, repos)

	got, err := repos.journals.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %v, want %v", got.ID, created.ID)
	}
}

func TestJournalRepository_GetByID_NotFound(t *testing.T) {
	repos := newTestRepos(t)

	_, err := repos.journals.GetByID(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("GetByID() error = nil, want repository.ErrJournalNotFound")
	}
	if !errors.Is(err, repository.ErrJournalNotFound) {
		t.Errorf("GetByID() error = %v, want repository.ErrJournalNotFound", err)
	}
}

func TestJournalRepository_GetByIdempotencyKey(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	created := newJournal(t, repos)

	got, err := repos.journals.GetByIdempotencyKey(ctx, created.IdempotencyKey)
	if err != nil {
		t.Fatalf("GetByIdempotencyKey() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %v, want %v", got.ID, created.ID)
	}
}

func TestJournalRepository_GetByIdempotencyKey_NotFound(t *testing.T) {
	repos := newTestRepos(t)

	_, err := repos.journals.GetByIdempotencyKey(context.Background(), "no-such-key")
	if err == nil {
		t.Fatal("GetByIdempotencyKey() error = nil, want repository.ErrJournalNotFound")
	}
	if !errors.Is(err, repository.ErrJournalNotFound) {
		t.Errorf("GetByIdempotencyKey() error = %v, want repository.ErrJournalNotFound", err)
	}
}

func TestJournalRepository_List(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	created := make([]*domain.Journal, 0, 3)
	for i := 0; i < 3; i++ {
		created = append(created, newJournal(t, repos))
		time.Sleep(2 * time.Millisecond)
	}

	all, err := repos.journals.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	positions := make([]int, 0, len(created))
	for _, want := range created {
		index := -1
		for i, journal := range all {
			if journal.ID == want.ID {
				index = i
				break
			}
		}
		if index < 0 {
			t.Fatalf("List() does not contain the created journal %s", want.ID)
		}
		positions = append(positions, index)
	}

	for i := 1; i < len(positions); i++ {
		if positions[i-1] <= positions[i] {
			t.Errorf("List() order = %v, want strictly descending created_at", positions)
		}
	}
}

// TestJournalRepository_GetWithPostings checks that the JSON payload built by
// the query is decoded into complete postings.
func TestJournalRepository_GetWithPostings(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	journal, postings := createBalancedPostings(t, repos, "1234.56")

	got, err := repos.journals.GetWithPostings(ctx, journal.ID)
	if err != nil {
		t.Fatalf("GetWithPostings() error = %v", err)
	}

	if got.Journal.ID != journal.ID {
		t.Errorf("Journal.ID = %v, want %v", got.Journal.ID, journal.ID)
	}
	if len(got.Postings) != 2 {
		t.Fatalf("len(Postings) = %d, want 2", len(got.Postings))
	}

	for _, posting := range got.Postings {
		if posting.JournalID != journal.ID {
			t.Errorf("Posting.JournalID = %v, want %v", posting.JournalID, journal.ID)
		}
		if posting.AccountID == uuid.Nil {
			t.Error("Posting.AccountID is nil")
		}
		if posting.Currency == "" {
			t.Error("Posting.Currency is empty")
		}
		if !posting.Amount.Equal(decimal.RequireFromString("1234.56")) {
			t.Errorf("Posting.Amount = %s, want 1234.56", posting.Amount)
		}
		if !posting.BalanceAfter.Equal(decimal.RequireFromString("1234.56")) {
			t.Errorf("Posting.BalanceAfter = %s, want 1234.56", posting.BalanceAfter)
		}
		if posting.CreatedAt.IsZero() {
			t.Error("Posting.CreatedAt is zero")
		}
	}

	found := map[uuid.UUID]bool{}
	for _, posting := range postings {
		found[posting.ID] = false
	}
	for _, posting := range got.Postings {
		if _, ok := found[posting.ID]; !ok {
			t.Errorf("unexpected posting %s", posting.ID)
			continue
		}
		found[posting.ID] = true
	}
	for id, seen := range found {
		if !seen {
			t.Errorf("posting %s is missing from the result", id)
		}
	}
}

func TestJournalRepository_GetWithPostings_NotFound(t *testing.T) {
	repos := newTestRepos(t)

	_, err := repos.journals.GetWithPostings(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("GetWithPostings() error = nil, want repository.ErrJournalNotFound")
	}
	if !errors.Is(err, repository.ErrJournalNotFound) {
		t.Errorf("GetWithPostings() error = %v, want repository.ErrJournalNotFound", err)
	}
}

func TestPostingRepository_Create_UpdatesAccountBalance(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	account := newAccount(t, repos)
	amount := decimal.RequireFromString("250.00")

	posting := createPosting(t, repos, account, domain.DirectionDebit, amount)
	if posting.ID == uuid.Nil {
		t.Error("Create() returned a nil UUID")
	}
	if !posting.Amount.Equal(amount) {
		t.Errorf("Amount = %s, want %s", posting.Amount, amount)
	}

	// The database trigger must have moved the account balance.
	updated, err := repos.accounts.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !updated.Balance.Equal(amount) {
		t.Errorf("Balance = %s, want %s", updated.Balance, amount)
	}
	// The revision counter must have moved with it.
	if updated.Version != 1 {
		t.Errorf("Version = %d, want 1", updated.Version)
	}
}

func TestPostingRepository_Create_RejectsUnbalancedJournal(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	account := newAccount(t, repos)
	journal := newJournal(t, repos)

	amount := decimal.RequireFromString("10.00")
	_, err := repos.postings.Create(ctx, repository.CreatePostingParams{
		JournalID: journal.ID,
		AccountID: account.ID,
		Direction: domain.DirectionDebit,
		Amount:    amount,
		Currency:  account.Currency,
	})
	if err == nil {
		t.Fatal("Create() error = nil, want the unbalanced journal to be rejected")
	}
}

func TestPostingRepository_Create_RejectsCurrencyMismatch(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	account := newAccount(t, repos)
	journal := newJournal(t, repos)

	amount := decimal.RequireFromString("10.00")
	_, err := repos.postings.Create(ctx, repository.CreatePostingParams{
		JournalID: journal.ID,
		AccountID: account.ID,
		Direction: domain.DirectionDebit,
		Amount:    amount,
		Currency:  "XXX", // differs from the account currency
	})
	if err == nil {
		t.Fatal("Create() error = nil, want the currency mismatch to be rejected")
	}
}

// TestPostingRepository_Create_DerivesBalanceAfter checks that the database
// derives the balance a posting leaves behind. The application cannot supply
// it, so the stored value is always what the postings actually say.
func TestPostingRepository_Create_DerivesBalanceAfter(t *testing.T) {
	repos := newTestRepos(t)

	account := newAccount(t, repos)

	first := createPosting(t, repos, account, domain.DirectionDebit, decimal.RequireFromString("25.00"))
	if !first.BalanceAfter.Equal(decimal.RequireFromString("25.00")) {
		t.Errorf("first BalanceAfter = %s, want 25.00", first.BalanceAfter)
	}

	second := createPosting(t, repos, account, domain.DirectionDebit, decimal.RequireFromString("75.00"))
	if !second.BalanceAfter.Equal(decimal.RequireFromString("100.00")) {
		t.Errorf("second BalanceAfter = %s, want 100.00", second.BalanceAfter)
	}
}

// TestPostingRepository_Create_CreditReducesTheBalance covers the other
// direction: a credit lowers the balance of a debit-normal account, and the
// database derives the balance that the posting leaves behind.
func TestPostingRepository_Create_CreditReducesTheBalance(t *testing.T) {
	repos := newTestRepos(t)

	account := newAccount(t, repos)

	// The account needs a balance to credit from.
	createPosting(t, repos, account, domain.DirectionDebit, decimal.RequireFromString("100.00"))

	credit := createPosting(t, repos, account, domain.DirectionCredit, decimal.RequireFromString("30.00"))
	if !credit.BalanceAfter.Equal(decimal.RequireFromString("70.00")) {
		t.Errorf("BalanceAfter = %s, want 70.00", credit.BalanceAfter)
	}

	updated, err := repos.accounts.GetByID(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if want := decimal.RequireFromString("70.00"); !updated.Balance.Equal(want) {
		t.Errorf("Balance = %s, want %s", updated.Balance, want)
	}
}

// TestPostingRepository_Create_ConcurrentPostingsToTheSameAccount is the
// regression test for the balance race. Two transactions post to the same
// account at the same time. The database locks the account row while it
// derives the balance, so the second transaction waits and then derives the
// balance the first one left behind. Without that lock both transactions read
// the same starting balance, and one of them was rejected as a mismatch.
func TestPostingRepository_Create_ConcurrentPostingsToTheSameAccount(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	account := newAccount(t, repos)
	amount := decimal.RequireFromString("10.00")

	// The barrier makes both transactions overlap on the account row.
	var start sync.WaitGroup
	start.Add(1)

	errs := make(chan error, 2)
	var done sync.WaitGroup

	for range 2 {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()

			errs <- postToAccount(t, repos, account, amount)
		}()
	}

	start.Done()
	done.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent posting failed: %v", err)
		}
	}

	updated, err := repos.accounts.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if want := decimal.RequireFromString("20.00"); !updated.Balance.Equal(want) {
		t.Errorf("Balance = %s, want %s", updated.Balance, want)
	}
}

// postToAccount writes one debit posting to the account and balances it on
// the opposite side, inside a single transaction.
//
// It returns the error instead of failing the test, because it runs in a
// goroutine of the concurrency test and t.Fatal is not allowed off the test
// goroutine.
func postToAccount(t *testing.T, repos testRepos, account *domain.Account, amount decimal.Decimal) error {
	ctx := t.Context()

	tx, err := repos.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txCtx := database.ContextWithTx(ctx, tx)

	journal, err := repos.journals.Create(txCtx, repository.CreateJournalParams{
		IdempotencyKey: uniqueIdempotencyKey(t),
		Description:    "concurrent posting",
	})
	if err != nil {
		return fmt.Errorf("create journal: %w", err)
	}

	opposite, err := repos.accounts.Create(txCtx, repository.CreateAccountParams{
		Currency:   uniqueCurrency(),
		NormalSide: domain.NormalSideCredit,
	})
	if err != nil {
		return fmt.Errorf("create opposite account: %w", err)
	}

	if _, err := repos.postings.Create(txCtx, repository.CreatePostingParams{
		JournalID: journal.ID,
		AccountID: account.ID,
		Direction: domain.DirectionDebit,
		Amount:    amount,
		Currency:  account.Currency,
	}); err != nil {
		return fmt.Errorf("create posting: %w", err)
	}

	if _, err := repos.postings.Create(txCtx, repository.CreatePostingParams{
		JournalID: journal.ID,
		AccountID: opposite.ID,
		Direction: domain.DirectionCredit,
		Amount:    amount,
		Currency:  opposite.Currency,
	}); err != nil {
		return fmt.Errorf("create opposite posting: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func TestPostingRepository_GetByID(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	journal, postings := createBalancedPostings(t, repos, "42.00")

	got, err := repos.postings.GetByID(ctx, postings[0].ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.ID != postings[0].ID {
		t.Errorf("ID = %v, want %v", got.ID, postings[0].ID)
	}
	if got.JournalID != journal.ID {
		t.Errorf("JournalID = %v, want %v", got.JournalID, journal.ID)
	}
}

func TestPostingRepository_GetByID_NotFound(t *testing.T) {
	repos := newTestRepos(t)

	_, err := repos.postings.GetByID(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("GetByID() error = nil, want repository.ErrPostingNotFound")
	}
	if !errors.Is(err, repository.ErrPostingNotFound) {
		t.Errorf("GetByID() error = %v, want repository.ErrPostingNotFound", err)
	}
}

func TestPostingRepository_ListByAccount(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	account := newAccount(t, repos)
	posting := createPosting(t, repos, account, domain.DirectionDebit, decimal.RequireFromString("10.00"))

	got, err := repos.postings.ListByAccount(ctx, account.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListByAccount() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(postings) = %d, want 1", len(got))
	}
	if got[0].ID != posting.ID {
		t.Errorf("ID = %v, want %v", got[0].ID, posting.ID)
	}
}

func TestPostingRepository_ListByJournal(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	journal, postings := createBalancedPostings(t, repos, "77.00")

	got, err := repos.postings.ListByJournal(ctx, journal.ID)
	if err != nil {
		t.Fatalf("ListByJournal() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(postings) = %d, want 2", len(got))
	}

	found := map[uuid.UUID]bool{}
	for _, posting := range postings {
		found[posting.ID] = false
	}
	for _, posting := range got {
		if _, ok := found[posting.ID]; !ok {
			t.Errorf("unexpected posting %s", posting.ID)
			continue
		}
		found[posting.ID] = true
	}
	for id, seen := range found {
		if !seen {
			t.Errorf("posting %s is missing from the result", id)
		}
	}
}

// TestPostingRepository_GetSummaryByAccount covers the aggregation over every
// posting of an account, not just the first one.
func TestPostingRepository_GetSummaryByAccount(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	account := newAccount(t, repos)

	inTx(t, repos, func(ctx context.Context) error {
		journals := repos.journals
		postings := repos.postings
		accounts := repos.accounts

		journal, err := journals.Create(ctx, repository.CreateJournalParams{
			IdempotencyKey: uniqueIdempotencyKey(t),
			Description:    "summary journal",
		})
		if err != nil {
			return fmt.Errorf("create journal: %w", err)
		}

		creditAccount, err := accounts.Create(ctx, repository.CreateAccountParams{
			Currency:   uniqueCurrency(),
			NormalSide: domain.NormalSideCredit,
		})
		if err != nil {
			return fmt.Errorf("create credit account: %w", err)
		}

		// Two debits against the same account: 25.00 and 75.00.
		if _, err := postings.Create(ctx, repository.CreatePostingParams{
			JournalID: journal.ID,
			AccountID: account.ID,
			Direction: domain.DirectionDebit,
			Amount:    decimal.RequireFromString("25.00"),
			Currency:  account.Currency,
		}); err != nil {
			return fmt.Errorf("create first debit posting: %w", err)
		}

		if _, err := postings.Create(ctx, repository.CreatePostingParams{
			JournalID: journal.ID,
			AccountID: account.ID,
			Direction: domain.DirectionDebit,
			Amount:    decimal.RequireFromString("75.00"),
			Currency:  account.Currency,
		}); err != nil {
			return fmt.Errorf("create second debit posting: %w", err)
		}

		// The balancing credit.
		if _, err := postings.Create(ctx, repository.CreatePostingParams{
			JournalID: journal.ID,
			AccountID: creditAccount.ID,
			Direction: domain.DirectionCredit,
			Amount:    decimal.RequireFromString("100.00"),
			Currency:  creditAccount.Currency,
		}); err != nil {
			return fmt.Errorf("create credit posting: %w", err)
		}

		return nil
	})

	summary, err := repos.postings.GetSummaryByAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetSummaryByAccount() error = %v", err)
	}

	if summary.AccountID != account.ID {
		t.Errorf("AccountID = %v, want %v", summary.AccountID, account.ID)
	}
	if summary.PostingCount != 2 {
		t.Errorf("PostingCount = %d, want 2", summary.PostingCount)
	}
	if !summary.TotalDebits.Equal(decimal.RequireFromString("100.00")) {
		t.Errorf("TotalDebits = %s, want 100.00", summary.TotalDebits)
	}
	if !summary.TotalCredits.IsZero() {
		t.Errorf("TotalCredits = %s, want 0", summary.TotalCredits)
	}
	if !summary.BalanceAfter.Equal(decimal.RequireFromString("100.00")) {
		t.Errorf("BalanceAfter = %s, want 100.00", summary.BalanceAfter)
	}
}

// indexOfAccount returns the position of the account in the slice, or -1.
func indexOfAccount(accounts []domain.Account, id uuid.UUID) int {
	for i, account := range accounts {
		if account.ID == id {
			return i
		}
	}
	return -1
}
