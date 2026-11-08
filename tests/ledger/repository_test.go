package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"ledgerline/internal/ledger/domain"
	"ledgerline/internal/ledger/infrastructure/repository"
	"ledgerline/tests/mocks/databasemocks"
	"ledgerline/tests/mocks/pgxmocks"
	"ledgerline/tests/mocks/repositorymocks"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// The repositories read and write through sqlc, so the doubles below hand
// back the columns in the order the generated queries scan them.

// accountRow returns the columns of ledger.accounts in the order
// GetAccount scans them.
func accountRow(a sqlcAccount) []any {
	return []any{
		a.id,
		a.currency,
		a.normalSide,
		a.allowNegative,
		a.balance,
		a.version,
		a.createdAt,
		a.updatedAt,
	}
}

// journalRow returns the columns of ledger.journals in the order GetJournal
// scans them.
func journalRow(j sqlcJournal) []any {
	return []any{
		j.id,
		j.idempotencyKey,
		j.reference,
		j.description,
		j.createdAt,
	}
}

// postingRow returns the columns of ledger.postings in the order GetPosting
// scans them.
func postingRow(p sqlcPosting) []any {
	return []any{
		p.id,
		p.journalID,
		p.accountID,
		p.direction,
		p.amount,
		p.currency,
		p.balanceAfter,
		p.createdAt,
	}
}

// sqlcAccount holds the values of an account row.
type sqlcAccount struct {
	id            uuid.UUID
	currency      string
	normalSide    string
	allowNegative bool
	balance       decimal.Decimal
	version       int64
	createdAt     time.Time
	updatedAt     time.Time
}

// sqlcJournal holds the values of a journal row.
type sqlcJournal struct {
	id             uuid.UUID
	idempotencyKey string
	reference      pgtype.UUID
	description    string
	createdAt      time.Time
}

// sqlcPosting holds the values of a posting row.
type sqlcPosting struct {
	id           uuid.UUID
	journalID    uuid.UUID
	accountID    uuid.UUID
	direction    string
	amount       decimal.Decimal
	currency     string
	balanceAfter decimal.Decimal
	createdAt    time.Time
}

// newQuerier returns a querier that hands out the given database handle.
func newQuerier(db *databasemocks.DBTX) *repositorymocks.Querier {
	q := &repositorymocks.Querier{}
	q.On("Querier", mock.Anything).Return(db)
	return q
}

// newAccountID returns a fixed account id.
func newAccountID() uuid.UUID {
	return uuid.MustParse("11111111-1111-7111-8111-111111111111")
}

func TestAccountRepository_Create(t *testing.T) {
	id := uuid.New()
	createdAt := time.Date(2026, time.October, 30, 10, 0, 0, 0, time.UTC)

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("INSERT INTO ledger.accounts"), mock.Anything).
		Return(pgxmocks.RowValues(accountRow(sqlcAccount{
			id:            id,
			currency:      "USD",
			normalSide:    string(domain.NormalSideDebit),
			allowNegative: false,
			balance:       decimal.Zero,
			version:       0,
			createdAt:     createdAt,
			updatedAt:     createdAt,
		})), nil).
		Once()

	repo := repository.NewAccountRepository(newQuerier(db))

	account, err := repo.Create(context.Background(), repository.CreateAccountParams{
		Currency:   "USD",
		NormalSide: domain.NormalSideDebit,
	})

	require.NoError(t, err)
	require.Equal(t, id, account.ID)
	require.Equal(t, "USD", account.Currency)
	require.Equal(t, domain.NormalSideDebit, account.NormalSide)
	require.False(t, account.AllowNegative)
	require.True(t, account.Balance.IsZero())
	require.Equal(t, int64(0), account.Version)
	require.Equal(t, createdAt, account.CreatedAt)
	db.AssertExpectations(t)
}

// TestAccountRepository_Create_Error checks that a failed insert is reported
// with the context of the statement.
func TestAccountRepository_Create_Error(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("INSERT INTO ledger.accounts"), mock.Anything).
		Return(pgxmocks.ErrRow(errors.New("constraint violated")), nil).
		Once()

	repo := repository.NewAccountRepository(newQuerier(db))

	_, err := repo.Create(context.Background(), repository.CreateAccountParams{
		Currency:   "US",
		NormalSide: domain.NormalSideDebit,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "account: create")
	db.AssertExpectations(t)
}

func TestAccountRepository_GetByID(t *testing.T) {
	id := newAccountID()
	balance := decimal.RequireFromString("1234.56")

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.accounts"), mock.Anything).
		Return(pgxmocks.RowValues(accountRow(sqlcAccount{
			id:         id,
			currency:   "USD",
			normalSide: string(domain.NormalSideDebit),
			balance:    balance,
			version:    3,
			createdAt:  time.Now(),
			updatedAt:  time.Now(),
		})), nil).
		Once()

	repo := repository.NewAccountRepository(newQuerier(db))

	account, err := repo.GetByID(context.Background(), id)

	require.NoError(t, err)
	require.Equal(t, id, account.ID)
	require.True(t, balance.Equal(account.Balance))
	require.Equal(t, int64(3), account.Version)
	db.AssertExpectations(t)
}

// TestAccountRepository_GetByID_NotFound checks that a missing account is
// reported with the sentinel error, so a caller can tell it apart from a
// database failure.
func TestAccountRepository_GetByID_NotFound(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.accounts"), mock.Anything).
		Return(pgxmocks.ErrRow(pgx.ErrNoRows), nil).
		Once()

	repo := repository.NewAccountRepository(newQuerier(db))

	_, err := repo.GetByID(context.Background(), uuid.New())

	require.ErrorIs(t, err, repository.ErrAccountNotFound)
	db.AssertExpectations(t)
}

func TestAccountRepository_GetByCurrency(t *testing.T) {
	id := newAccountID()

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.accounts"), mock.Anything).
		Return(pgxmocks.RowValues(accountRow(sqlcAccount{
			id:         id,
			currency:   "EUR",
			normalSide: string(domain.NormalSideCredit),
		})), nil).
		Once()

	repo := repository.NewAccountRepository(newQuerier(db))

	account, err := repo.GetByCurrency(context.Background(), "EUR")

	require.NoError(t, err)
	require.Equal(t, id, account.ID)
	require.Equal(t, domain.NormalSideCredit, account.NormalSide)
	db.AssertExpectations(t)
}

// TestAccountRepository_GetByCurrency_NotFound checks the sentinel error of
// the currency lookup.
func TestAccountRepository_GetByCurrency_NotFound(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.accounts"), mock.Anything).
		Return(pgxmocks.ErrRow(pgx.ErrNoRows), nil).
		Once()

	repo := repository.NewAccountRepository(newQuerier(db))

	_, err := repo.GetByCurrency(context.Background(), "ZZZ")

	require.ErrorIs(t, err, repository.ErrAccountNotFound)
	db.AssertExpectations(t)
}

func TestAccountRepository_List(t *testing.T) {
	first := newAccountID()
	second := uuid.New()

	db := &databasemocks.DBTX{}
	db.On("Query", mock.Anything, databasemocks.MatchSQL("FROM ledger.accounts"), mock.Anything).
		Return(pgxmocks.RowsValues(
			accountRow(sqlcAccount{id: first, currency: "USD", normalSide: "debit"}),
			accountRow(sqlcAccount{id: second, currency: "EUR", normalSide: "credit"}),
		), nil).
		Once()

	repo := repository.NewAccountRepository(newQuerier(db))

	accounts, err := repo.List(context.Background(), 10, 0)

	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.Equal(t, first, accounts[0].ID)
	require.Equal(t, second, accounts[1].ID)
	db.AssertExpectations(t)
}

// TestAccountRepository_List_Error checks that a failed query is reported.
func TestAccountRepository_List_Error(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("Query", mock.Anything, databasemocks.MatchSQL("FROM ledger.accounts"), mock.Anything).
		Return(nil, errors.New("connection lost")).
		Once()

	repo := repository.NewAccountRepository(newQuerier(db))

	_, err := repo.List(context.Background(), 10, 0)

	require.Error(t, err)
	require.Contains(t, err.Error(), "account: list")
	db.AssertExpectations(t)
}

func TestJournalRepository_Create(t *testing.T) {
	id := uuid.New()
	createdAt := time.Date(2026, time.October, 30, 10, 0, 0, 0, time.UTC)

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("INSERT INTO ledger.journals"), mock.Anything).
		Return(pgxmocks.RowValues(journalRow(sqlcJournal{
			id:             id,
			idempotencyKey: "key-1",
			description:    descInvoice,
			createdAt:      createdAt,
		})), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	journal, err := repo.Create(context.Background(), repository.CreateJournalParams{
		IdempotencyKey: "key-1",
		Description:    descInvoice,
	})

	require.NoError(t, err)
	require.Equal(t, id, journal.ID)
	require.Equal(t, "key-1", journal.IdempotencyKey)
	require.Equal(t, descInvoice, journal.Description)
	require.Nil(t, journal.Reference)
	db.AssertExpectations(t)
}

// TestJournalRepository_Create_WithReference checks that a reference is
// carried into the row and back out of it.
func TestJournalRepository_Create_WithReference(t *testing.T) {
	id := uuid.New()
	reference := uuid.New()

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("INSERT INTO ledger.journals"), mock.Anything).
		Return(pgxmocks.RowValues(journalRow(sqlcJournal{
			id:             id,
			idempotencyKey: "key-1",
			reference:      pgtype.UUID{Bytes: reference, Valid: true},
			createdAt:      time.Now(),
		})), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	journal, err := repo.Create(context.Background(), repository.CreateJournalParams{
		IdempotencyKey: "key-1",
		Reference:      &reference,
	})

	require.NoError(t, err)
	require.NotNil(t, journal.Reference)
	require.Equal(t, reference, *journal.Reference)
	db.AssertExpectations(t)
}

// TestJournalRepository_Create_IsIdempotent covers the documented contract:
// replaying the same idempotency key returns the original journal instead of
// creating a duplicate. The insert reports that it inserted nothing, so the
// repository reads the original journal back.
func TestJournalRepository_Create_IsIdempotent(t *testing.T) {
	original := uuid.New()

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("INSERT INTO ledger.journals"), mock.Anything).
		Return(pgxmocks.ErrRow(pgx.ErrNoRows), nil).
		Once()
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.journals"), mock.Anything).
		Return(pgxmocks.RowValues(journalRow(sqlcJournal{
			id:             original,
			idempotencyKey: "key-1",
			description:    "first",
			createdAt:      time.Now(),
		})), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	journal, err := repo.Create(context.Background(), repository.CreateJournalParams{
		IdempotencyKey: "key-1",
		Description:    "second",
	})

	require.NoError(t, err)
	require.Equal(t, original, journal.ID)
	require.Equal(t, "first", journal.Description, "the original journal should be returned")
	db.AssertExpectations(t)
}

// TestJournalRepository_Create_ConflictReadError checks that a failing read
// back of the original journal is reported instead of returning a half-built
// journal.
func TestJournalRepository_Create_ConflictReadError(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("INSERT INTO ledger.journals"), mock.Anything).
		Return(pgxmocks.ErrRow(pgx.ErrNoRows), nil).
		Once()
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.journals"), mock.Anything).
		Return(pgxmocks.ErrRow(errors.New("connection lost")), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	_, err := repo.Create(context.Background(), repository.CreateJournalParams{IdempotencyKey: "key-1"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "get by idempotency key")
	db.AssertExpectations(t)
}

func TestJournalRepository_GetByID(t *testing.T) {
	id := uuid.New()

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.journals"), mock.Anything).
		Return(pgxmocks.RowValues(journalRow(sqlcJournal{
			id:             id,
			idempotencyKey: "key-1",
			createdAt:      time.Now(),
		})), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	journal, err := repo.GetByID(context.Background(), id)

	require.NoError(t, err)
	require.Equal(t, id, journal.ID)
	db.AssertExpectations(t)
}

// TestJournalRepository_GetByID_NotFound checks the sentinel error of the
// journal lookup.
func TestJournalRepository_GetByID_NotFound(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.journals"), mock.Anything).
		Return(pgxmocks.ErrRow(pgx.ErrNoRows), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	_, err := repo.GetByID(context.Background(), uuid.New())

	require.ErrorIs(t, err, repository.ErrJournalNotFound)
	db.AssertExpectations(t)
}

func TestJournalRepository_GetByIdempotencyKey(t *testing.T) {
	id := uuid.New()

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.journals"), mock.Anything).
		Return(pgxmocks.RowValues(journalRow(sqlcJournal{
			id:             id,
			idempotencyKey: "key-1",
			createdAt:      time.Now(),
		})), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	journal, err := repo.GetByIdempotencyKey(context.Background(), "key-1")

	require.NoError(t, err)
	require.Equal(t, id, journal.ID)
	db.AssertExpectations(t)
}

// TestJournalRepository_GetByIdempotencyKey_NotFound checks the sentinel
// error of the idempotency key lookup.
func TestJournalRepository_GetByIdempotencyKey_NotFound(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.journals"), mock.Anything).
		Return(pgxmocks.ErrRow(pgx.ErrNoRows), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	_, err := repo.GetByIdempotencyKey(context.Background(), "no-such-key")

	require.ErrorIs(t, err, repository.ErrJournalNotFound)
	db.AssertExpectations(t)
}

func TestJournalRepository_List(t *testing.T) {
	first := uuid.New()
	second := uuid.New()

	db := &databasemocks.DBTX{}
	db.On("Query", mock.Anything, databasemocks.MatchSQL("FROM ledger.journals"), mock.Anything).
		Return(pgxmocks.RowsValues(
			journalRow(sqlcJournal{id: first, idempotencyKey: "key-1"}),
			journalRow(sqlcJournal{id: second, idempotencyKey: "key-2"}),
		), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	journals, err := repo.List(context.Background(), 10, 0)

	require.NoError(t, err)
	require.Len(t, journals, 2)
	require.Equal(t, first, journals[0].ID)
	db.AssertExpectations(t)
}

// TestJournalRepository_GetWithPostings checks that the JSON payload built by
// the query is decoded into complete postings.
func TestJournalRepository_GetWithPostings(t *testing.T) {
	journalID := uuid.New()
	accountID := uuid.New()
	postingID := uuid.New()
	createdAt := time.Date(2026, time.October, 30, 14, 0, 15, 105171000, time.UTC)

	payload := `[{"id":"` + postingID.String() +
		`","journal_id":"` + journalID.String() +
		`","account_id":"` + accountID.String() +
		`","direction":"debit","amount":"1234.56","currency":"USD","balance_after":"1234.56","created_at":"2026-10-30T14:00:15.105171+00:00"}]`

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.journals"), mock.Anything).
		Return(pgxmocks.RowValues(append(
			journalRow(sqlcJournal{id: journalID, idempotencyKey: "key-1", createdAt: createdAt}),
			[]byte(payload),
		)), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	got, err := repo.GetWithPostings(context.Background(), journalID)

	require.NoError(t, err)
	require.Equal(t, journalID, got.Journal.ID)
	require.Len(t, got.Postings, 1)

	posting := got.Postings[0]
	require.Equal(t, postingID, posting.ID)
	require.Equal(t, journalID, posting.JournalID)
	require.Equal(t, accountID, posting.AccountID)
	require.Equal(t, domain.DirectionDebit, posting.Direction)
	require.True(t, decimal.RequireFromString("1234.56").Equal(posting.Amount))
	require.Equal(t, "USD", posting.Currency)
	require.True(t, decimal.RequireFromString("1234.56").Equal(posting.BalanceAfter))
	// The timestamp is decoded from RFC 3339, so it carries a fixed offset
	// instead of the UTC location. The instant is the same.
	require.True(t, posting.CreatedAt.Equal(createdAt), "CreatedAt = %s, want %s", posting.CreatedAt, createdAt)
	db.AssertExpectations(t)
}

// TestJournalRepository_GetWithPostings_NotFound checks the sentinel error of
// the journal lookup.
func TestJournalRepository_GetWithPostings_NotFound(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.journals"), mock.Anything).
		Return(pgxmocks.ErrRow(pgx.ErrNoRows), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	_, err := repo.GetWithPostings(context.Background(), uuid.New())

	require.ErrorIs(t, err, repository.ErrJournalNotFound)
	db.AssertExpectations(t)
}

// TestJournalRepository_GetWithPostings_BrokenPayload checks that a corrupt
// payload is reported instead of being read as an empty journal.
func TestJournalRepository_GetWithPostings_BrokenPayload(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.journals"), mock.Anything).
		Return(pgxmocks.RowValues(append(
			journalRow(sqlcJournal{id: uuid.New(), idempotencyKey: "key-1"}),
			[]byte(`{"not":"an array"}`),
		)), nil).
		Once()

	repo := repository.NewJournalRepository(newQuerier(db))

	_, err := repo.GetWithPostings(context.Background(), uuid.New())

	require.Error(t, err)
	db.AssertExpectations(t)
}

func TestPostingRepository_Create(t *testing.T) {
	id := uuid.New()
	journalID := uuid.New()
	accountID := uuid.New()
	amount := decimal.RequireFromString("250.00")

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("INSERT INTO ledger.postings"), mock.Anything).
		Return(pgxmocks.RowValues(postingRow(sqlcPosting{
			id:           id,
			journalID:    journalID,
			accountID:    accountID,
			direction:    string(domain.DirectionDebit),
			amount:       amount,
			currency:     "USD",
			balanceAfter: amount,
			createdAt:    time.Now(),
		})), nil).
		Once()

	repo := repository.NewPostingRepository(newQuerier(db))

	posting, err := repo.Create(context.Background(), repository.CreatePostingParams{
		JournalID: journalID,
		AccountID: accountID,
		Direction: domain.DirectionDebit,
		Amount:    amount,
		Currency:  "USD",
	})

	require.NoError(t, err)
	require.Equal(t, id, posting.ID)
	require.True(t, amount.Equal(posting.Amount))
	require.True(t, amount.Equal(posting.BalanceAfter))
	db.AssertExpectations(t)
}

// TestPostingRepository_Create_Error checks that a failed insert is reported
// with the context of the statement.
func TestPostingRepository_Create_Error(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("INSERT INTO ledger.postings"), mock.Anything).
		Return(pgxmocks.ErrRow(errors.New("journal does not balance")), nil).
		Once()

	repo := repository.NewPostingRepository(newQuerier(db))

	_, err := repo.Create(context.Background(), repository.CreatePostingParams{
		JournalID: uuid.New(),
		AccountID: uuid.New(),
		Direction: domain.DirectionDebit,
		Amount:    decimal.RequireFromString("10.00"),
		Currency:  "USD",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "posting: create")
	db.AssertExpectations(t)
}

func TestPostingRepository_GetByID(t *testing.T) {
	id := uuid.New()

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.postings"), mock.Anything).
		Return(pgxmocks.RowValues(postingRow(sqlcPosting{
			id:        id,
			journalID: uuid.New(),
			accountID: uuid.New(),
			direction: string(domain.DirectionCredit),
			amount:    decimal.RequireFromString("42.00"),
			currency:  "EUR",
		})), nil).
		Once()

	repo := repository.NewPostingRepository(newQuerier(db))

	posting, err := repo.GetByID(context.Background(), id)

	require.NoError(t, err)
	require.Equal(t, id, posting.ID)
	require.Equal(t, domain.DirectionCredit, posting.Direction)
	db.AssertExpectations(t)
}

// TestPostingRepository_GetByID_NotFound checks the sentinel error of the
// posting lookup.
func TestPostingRepository_GetByID_NotFound(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.postings"), mock.Anything).
		Return(pgxmocks.ErrRow(pgx.ErrNoRows), nil).
		Once()

	repo := repository.NewPostingRepository(newQuerier(db))

	_, err := repo.GetByID(context.Background(), uuid.New())

	require.ErrorIs(t, err, repository.ErrPostingNotFound)
	db.AssertExpectations(t)
}

func TestPostingRepository_List(t *testing.T) {
	first := uuid.New()
	second := uuid.New()

	db := &databasemocks.DBTX{}
	db.On("Query", mock.Anything, databasemocks.MatchSQL("FROM ledger.postings"), mock.Anything).
		Return(pgxmocks.RowsValues(
			postingRow(sqlcPosting{id: first, direction: "debit"}),
			postingRow(sqlcPosting{id: second, direction: "credit"}),
		), nil).
		Once()

	repo := repository.NewPostingRepository(newQuerier(db))

	postings, err := repo.List(context.Background(), 10, 0)

	require.NoError(t, err)
	require.Len(t, postings, 2)
	require.Equal(t, first, postings[0].ID)
	db.AssertExpectations(t)
}

func TestPostingRepository_ListByAccount(t *testing.T) {
	accountID := uuid.New()
	postingID := uuid.New()

	db := &databasemocks.DBTX{}
	db.On("Query", mock.Anything, databasemocks.MatchSQL("FROM ledger.postings"), mock.Anything).
		Return(pgxmocks.RowsValues(
			postingRow(sqlcPosting{id: postingID, accountID: accountID, direction: "debit"}),
		), nil).
		Once()

	repo := repository.NewPostingRepository(newQuerier(db))

	postings, err := repo.ListByAccount(context.Background(), accountID, 10, 0)

	require.NoError(t, err)
	require.Len(t, postings, 1)
	require.Equal(t, postingID, postings[0].ID)
	db.AssertExpectations(t)
}

func TestPostingRepository_ListByJournal(t *testing.T) {
	journalID := uuid.New()

	db := &databasemocks.DBTX{}
	db.On("Query", mock.Anything, databasemocks.MatchSQL("FROM ledger.postings"), mock.Anything).
		Return(pgxmocks.RowsValues(
			postingRow(sqlcPosting{id: uuid.New(), journalID: journalID, direction: "debit"}),
			postingRow(sqlcPosting{id: uuid.New(), journalID: journalID, direction: "credit"}),
		), nil).
		Once()

	repo := repository.NewPostingRepository(newQuerier(db))

	postings, err := repo.ListByJournal(context.Background(), journalID)

	require.NoError(t, err)
	require.Len(t, postings, 2)
	db.AssertExpectations(t)
}

// TestPostingRepository_GetSummaryByAccount covers the aggregation over every
// posting of an account, not just the first one.
func TestPostingRepository_GetSummaryByAccount(t *testing.T) {
	accountID := uuid.New()

	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.postings"), mock.Anything).
		Return(pgxmocks.RowValues([]any{
			accountID,
			int64(2),
			decimal.RequireFromString("100.00"),
			decimal.Zero,
			decimal.RequireFromString("100.00"),
		}), nil).
		Once()

	repo := repository.NewPostingRepository(newQuerier(db))

	summary, err := repo.GetSummaryByAccount(context.Background(), accountID)

	require.NoError(t, err)
	require.Equal(t, accountID, summary.AccountID)
	require.Equal(t, int64(2), summary.PostingCount)
	require.True(t, decimal.RequireFromString("100.00").Equal(summary.TotalDebits))
	require.True(t, summary.TotalCredits.IsZero())
	require.True(t, decimal.RequireFromString("100.00").Equal(summary.BalanceAfter))
	db.AssertExpectations(t)
}

// TestPostingRepository_GetSummaryByAccount_NotFound checks the sentinel
// error of the summary lookup.
func TestPostingRepository_GetSummaryByAccount_NotFound(t *testing.T) {
	db := &databasemocks.DBTX{}
	db.On("QueryRow", mock.Anything, databasemocks.MatchSQL("FROM ledger.postings"), mock.Anything).
		Return(pgxmocks.ErrRow(pgx.ErrNoRows), nil).
		Once()

	repo := repository.NewPostingRepository(newQuerier(db))

	_, err := repo.GetSummaryByAccount(context.Background(), uuid.New())

	require.ErrorIs(t, err, repository.ErrPostingNotFound)
	db.AssertExpectations(t)
}
