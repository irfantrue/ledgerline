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

			if got.ID != tt.want.ID {
				t.Errorf("ID = %v, want %v", got.ID, tt.want.ID)
			}
			if got.Currency != tt.want.Currency {
				t.Errorf("Currency = %q, want %q", got.Currency, tt.want.Currency)
			}
			if got.NormalSide != tt.want.NormalSide {
				t.Errorf("NormalSide = %q, want %q", got.NormalSide, tt.want.NormalSide)
			}
			if got.AllowNegative != tt.want.AllowNegative {
				t.Errorf("AllowNegative = %t, want %t", got.AllowNegative, tt.want.AllowNegative)
			}
			if !got.Balance.Equal(tt.want.Balance) {
				t.Errorf("Balance = %s, want %s", got.Balance, tt.want.Balance)
			}
			if got.Version != tt.want.Version {
				t.Errorf("Version = %d, want %d", got.Version, tt.want.Version)
			}
			if !got.CreatedAt.Equal(tt.want.CreatedAt) {
				t.Errorf("CreatedAt = %s, want %s", got.CreatedAt, tt.want.CreatedAt)
			}
			if !got.UpdatedAt.Equal(tt.want.UpdatedAt) {
				t.Errorf("UpdatedAt = %s, want %s", got.UpdatedAt, tt.want.UpdatedAt)
			}
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

			if got.ID != tt.want.ID {
				t.Errorf("ID = %v, want %v", got.ID, tt.want.ID)
			}
			if got.JournalID != tt.want.JournalID {
				t.Errorf("JournalID = %v, want %v", got.JournalID, tt.want.JournalID)
			}
			if got.AccountID != tt.want.AccountID {
				t.Errorf("AccountID = %v, want %v", got.AccountID, tt.want.AccountID)
			}
			if got.Direction != tt.want.Direction {
				t.Errorf("Direction = %q, want %q", got.Direction, tt.want.Direction)
			}
			if !got.Amount.Equal(tt.want.Amount) {
				t.Errorf("Amount = %s, want %s", got.Amount, tt.want.Amount)
			}
			if got.Currency != tt.want.Currency {
				t.Errorf("Currency = %q, want %q", got.Currency, tt.want.Currency)
			}
			if !got.BalanceAfter.Equal(tt.want.BalanceAfter) {
				t.Errorf("BalanceAfter = %s, want %s", got.BalanceAfter, tt.want.BalanceAfter)
			}
			if !got.CreatedAt.Equal(tt.want.CreatedAt) {
				t.Errorf("CreatedAt = %s, want %s", got.CreatedAt, tt.want.CreatedAt)
			}
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

		if got.ID != id {
			t.Errorf("ID = %v, want %v", got.ID, id)
		}
		if got.IdempotencyKey != "key-1" {
			t.Errorf("IdempotencyKey = %q, want %q", got.IdempotencyKey, "key-1")
		}
		if got.Reference == nil {
			t.Fatal("Reference = nil, want a value")
		}
		if *got.Reference != reference {
			t.Errorf("Reference = %v, want %v", *got.Reference, reference)
		}
		if got.Description != descInvoice {
			t.Errorf("Description = %q, want %q", got.Description, descInvoice)
		}
		if !got.CreatedAt.Equal(createdAt) {
			t.Errorf("CreatedAt = %s, want %s", got.CreatedAt, createdAt)
		}
	})

	t.Run("without reference", func(t *testing.T) {
		got := repository.ToJournalDomain(&sqlcgen.LedgerJournal{
			ID:             id,
			IdempotencyKey: "key-2",
			Reference:      pgtype.UUID{},
			Description:    "",
			CreatedAt:      createdAt,
		})

		if got.Reference != nil {
			t.Errorf("Reference = %v, want nil", *got.Reference)
		}
	})
}
