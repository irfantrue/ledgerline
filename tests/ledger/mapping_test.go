package ledger

import (
	"testing"
	"time"

	"ledgerline/internal/ledger/domain"
	"ledgerline/internal/ledger/infrastructure/repository"
	"ledgerline/internal/ledger/infrastructure/sqlcgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	currencyUSD = "USD"
	currencyEUR = "EUR"
	descInvoice = "invoice 42"
)

func TestToAccountDomain(t *testing.T) {
	id := uuid.MustParse("11111111-1111-7111-8111-111111111111")
	createdAt := time.Date(2026, time.October, 30, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)

	tests := []struct {
		name string
		row  sqlcgen.LedgerAccount
		want domain.Account
	}{
		{
			name: "debit normal side with balance",
			row: sqlcgen.LedgerAccount{
				ID:            id,
				Currency:      currencyUSD,
				NormalSide:    "debit",
				AllowNegative: false,
				Balance:       decimal.RequireFromString("1234.56"),
				Version:       3,
				CreatedAt:     createdAt,
				UpdatedAt:     updatedAt,
			},
			want: domain.Account{
				ID:            id,
				Currency:      currencyUSD,
				NormalSide:    domain.NormalSideDebit,
				AllowNegative: false,
				Balance:       decimal.RequireFromString("1234.56"),
				Version:       3,
				CreatedAt:     createdAt,
				UpdatedAt:     updatedAt,
			},
		},
		{
			name: "credit normal side with negative balance",
			row: sqlcgen.LedgerAccount{
				ID:            id,
				Currency:      currencyEUR,
				NormalSide:    "credit",
				AllowNegative: true,
				Balance:       decimal.RequireFromString("-42.10"),
				Version:       0,
				CreatedAt:     createdAt,
				UpdatedAt:     updatedAt,
			},
			want: domain.Account{
				ID:            id,
				Currency:      currencyEUR,
				NormalSide:    domain.NormalSideCredit,
				AllowNegative: true,
				Balance:       decimal.RequireFromString("-42.10"),
				Version:       0,
				CreatedAt:     createdAt,
				UpdatedAt:     updatedAt,
			},
		},
		{
			name: "zero value account",
			row:  sqlcgen.LedgerAccount{},
			want: domain.Account{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repository.ToAccountDomain(&tt.row)

			require.NotNil(t, got)
			assert.Equal(t, tt.want.ID, got.ID, "ID")
			assert.Equal(t, tt.want.Currency, got.Currency, "Currency")
			assert.Equal(t, tt.want.NormalSide, got.NormalSide, "NormalSide")
			assert.Equal(t, tt.want.AllowNegative, got.AllowNegative, "AllowNegative")
			assert.True(t, tt.want.Balance.Equal(got.Balance), "Balance = %s, want %s", got.Balance, tt.want.Balance)
			assert.Equal(t, tt.want.Version, got.Version, "Version")
			assert.True(t, tt.want.CreatedAt.Equal(got.CreatedAt), "CreatedAt = %s, want %s", got.CreatedAt, tt.want.CreatedAt)
			assert.True(t, tt.want.UpdatedAt.Equal(got.UpdatedAt), "UpdatedAt = %s, want %s", got.UpdatedAt, tt.want.UpdatedAt)
		})
	}
}

func TestToPostingDomain(t *testing.T) {
	id := uuid.MustParse("22222222-2222-7222-8222-222222222222")
	journalID := uuid.MustParse("33333333-3333-7333-8333-333333333333")
	accountID := uuid.MustParse("11111111-1111-7111-8111-111111111111")
	createdAt := time.Date(2026, time.October, 30, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		row  sqlcgen.LedgerPosting
		want domain.Posting
	}{
		{
			name: "debit posting",
			row: sqlcgen.LedgerPosting{
				ID:           id,
				JournalID:    journalID,
				AccountID:    accountID,
				Direction:    "debit",
				Amount:       decimal.RequireFromString("500.00"),
				Currency:     currencyUSD,
				BalanceAfter: decimal.RequireFromString("1500.00"),
				CreatedAt:    createdAt,
			},
			want: domain.Posting{
				ID:           id,
				JournalID:    journalID,
				AccountID:    accountID,
				Direction:    domain.DirectionDebit,
				Amount:       decimal.RequireFromString("500.00"),
				Currency:     currencyUSD,
				BalanceAfter: decimal.RequireFromString("1500.00"),
				CreatedAt:    createdAt,
			},
		},
		{
			name: "credit posting",
			row: sqlcgen.LedgerPosting{
				ID:           id,
				JournalID:    journalID,
				AccountID:    accountID,
				Direction:    "credit",
				Amount:       decimal.RequireFromString("0.01"),
				Currency:     currencyEUR,
				BalanceAfter: decimal.Zero,
				CreatedAt:    createdAt,
			},
			want: domain.Posting{
				ID:           id,
				JournalID:    journalID,
				AccountID:    accountID,
				Direction:    domain.DirectionCredit,
				Amount:       decimal.RequireFromString("0.01"),
				Currency:     currencyEUR,
				BalanceAfter: decimal.Zero,
				CreatedAt:    createdAt,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repository.ToPostingDomain(&tt.row)

			require.NotNil(t, got)
			assert.Equal(t, tt.want.ID, got.ID, "ID")
			assert.Equal(t, tt.want.JournalID, got.JournalID, "JournalID")
			assert.Equal(t, tt.want.AccountID, got.AccountID, "AccountID")
			assert.Equal(t, tt.want.Direction, got.Direction, "Direction")
			assert.True(t, tt.want.Amount.Equal(got.Amount), "Amount = %s, want %s", got.Amount, tt.want.Amount)
			assert.Equal(t, tt.want.Currency, got.Currency, "Currency")
			assert.True(t, tt.want.BalanceAfter.Equal(got.BalanceAfter), "BalanceAfter = %s, want %s", got.BalanceAfter, tt.want.BalanceAfter)
			assert.True(t, tt.want.CreatedAt.Equal(got.CreatedAt), "CreatedAt = %s, want %s", got.CreatedAt, tt.want.CreatedAt)
		})
	}
}

func TestToJournalDomain(t *testing.T) {
	id := uuid.MustParse("33333333-3333-7333-8333-333333333333")
	reference := uuid.MustParse("44444444-4444-7444-8444-444444444444")
	createdAt := time.Date(2026, time.October, 30, 10, 0, 0, 0, time.UTC)

	t.Run("with reference", func(t *testing.T) {
		got := repository.ToJournalDomain(&sqlcgen.LedgerJournal{
			ID:             id,
			IdempotencyKey: "key-1",
			Reference:      pgtype.UUID{Bytes: reference, Valid: true},
			Description:    descInvoice,
			CreatedAt:      createdAt,
		})

		require.NotNil(t, got)
		assert.Equal(t, id, got.ID, "ID")
		assert.Equal(t, "key-1", got.IdempotencyKey, "IdempotencyKey")
		require.NotNil(t, got.Reference, "Reference = nil, want a value")
		assert.Equal(t, reference, *got.Reference, "Reference")
		assert.Equal(t, descInvoice, got.Description, "Description")
		assert.True(t, createdAt.Equal(got.CreatedAt), "CreatedAt = %s, want %s", got.CreatedAt, createdAt)
	})

	t.Run("without reference", func(t *testing.T) {
		got := repository.ToJournalDomain(&sqlcgen.LedgerJournal{
			ID:             id,
			IdempotencyKey: "key-2",
			Reference:      pgtype.UUID{},
			Description:    "",
			CreatedAt:      createdAt,
		})

		require.NotNil(t, got)
		assert.Nil(t, got.Reference, "Reference = %v, want nil", got.Reference)
	})
}
