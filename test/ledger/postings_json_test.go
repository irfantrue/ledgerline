package ledger

import (
	"testing"
	"time"

	"ledgerline/internal/ledger/domain"
	"ledgerline/internal/ledger/infrastructure/repository"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	testPostingID      = "44444444-4444-7444-8444-444444444444"
	testJournalID      = "33333333-3333-7333-8333-333333333333"
	testAccountID      = "11111111-1111-7111-8111-111111111111"
	testPostingCreated = "2026-10-30T14:00:15.105171+00:00"
)

func TestParsePostings(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []domain.Posting
		wantErr bool
	}{
		{
			name: "amount as json text keeps decimal precision",
			raw:  `[{"id":"` + testPostingID + `","journal_id":"` + testJournalID + `","account_id":"` + testAccountID + `","direction":"debit","amount":"12345678901234.56","currency":"USD","balance_after":"0.01","created_at":"` + testPostingCreated + `"}]`,
			want: []domain.Posting{
				{
					ID:           uuid.MustParse(testPostingID),
					JournalID:    uuid.MustParse(testJournalID),
					AccountID:    uuid.MustParse(testAccountID),
					Direction:    domain.DirectionDebit,
					Amount:       decimal.RequireFromString("12345678901234.56"),
					Currency:     currencyUSD,
					BalanceAfter: decimal.RequireFromString("0.01"),
					CreatedAt:    time.Date(2026, time.October, 30, 14, 0, 15, 105171000, time.UTC),
				},
			},
		},
		{
			name: "amount as json number is still decoded without float rounding",
			raw:  `[{"id":"` + testPostingID + `","journal_id":"` + testJournalID + `","account_id":"` + testAccountID + `","direction":"credit","amount":75.00,"currency":"EUR","balance_after":0,"created_at":"` + testPostingCreated + `"}]`,
			want: []domain.Posting{
				{
					ID:           uuid.MustParse(testPostingID),
					JournalID:    uuid.MustParse(testJournalID),
					AccountID:    uuid.MustParse(testAccountID),
					Direction:    domain.DirectionCredit,
					Amount:       decimal.RequireFromString("75.00"),
					Currency:     currencyEUR,
					BalanceAfter: decimal.Zero,
					CreatedAt:    time.Date(2026, time.October, 30, 14, 0, 15, 105171000, time.UTC),
				},
			},
		},
		{
			name: "empty array",
			raw:  `[]`,
			want: []domain.Posting{},
		},
		{
			name: "null array",
			raw:  `null`,
			want: []domain.Posting{},
		},
		{
			name:    "null amount is rejected",
			raw:     `[{"id":"` + testPostingID + `","journal_id":"` + testJournalID + `","account_id":"` + testAccountID + `","direction":"debit","amount":null,"currency":"USD","balance_after":"0.01","created_at":"` + testPostingCreated + `"}]`,
			wantErr: true,
		},
		{
			name:    "malformed json",
			raw:     `{`,
			wantErr: true,
		},
		{
			name:    "invalid posting id",
			raw:     `[{"id":"not-a-uuid","journal_id":"` + testJournalID + `","account_id":"` + testAccountID + `","direction":"debit","amount":"1.00","currency":"USD","balance_after":"1.00","created_at":"` + testPostingCreated + `"}]`,
			wantErr: true,
		},
		{
			name:    "missing journal id",
			raw:     `[{"id":"` + testPostingID + `","account_id":"` + testAccountID + `","direction":"debit","amount":"1.00","currency":"USD","balance_after":"1.00","created_at":"` + testPostingCreated + `"}]`,
			wantErr: true,
		},
		{
			name:    "invalid amount",
			raw:     `[{"id":"` + testPostingID + `","journal_id":"` + testJournalID + `","account_id":"` + testAccountID + `","direction":"debit","amount":"abc","currency":"USD","balance_after":"1.00","created_at":"` + testPostingCreated + `"}]`,
			wantErr: true,
		},
		{
			name:    "invalid created_at",
			raw:     `[{"id":"` + testPostingID + `","journal_id":"` + testJournalID + `","account_id":"` + testAccountID + `","direction":"debit","amount":"1.00","currency":"USD","balance_after":"1.00","created_at":"yesterday"}]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repository.ParsePostings([]byte(tt.raw))

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePostings() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePostings() error = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("len(postings) = %d, want %d", len(got), len(tt.want))
			}

			for i := range got {
				if got[i].ID != tt.want[i].ID {
					t.Errorf("postings[%d].ID = %v, want %v", i, got[i].ID, tt.want[i].ID)
				}
				if got[i].JournalID != tt.want[i].JournalID {
					t.Errorf("postings[%d].JournalID = %v, want %v", i, got[i].JournalID, tt.want[i].JournalID)
				}
				if got[i].AccountID != tt.want[i].AccountID {
					t.Errorf("postings[%d].AccountID = %v, want %v", i, got[i].AccountID, tt.want[i].AccountID)
				}
				if got[i].Direction != tt.want[i].Direction {
					t.Errorf("postings[%d].Direction = %q, want %q", i, got[i].Direction, tt.want[i].Direction)
				}
				if !got[i].Amount.Equal(tt.want[i].Amount) {
					t.Errorf("postings[%d].Amount = %s, want %s", i, got[i].Amount, tt.want[i].Amount)
				}
				if got[i].Currency != tt.want[i].Currency {
					t.Errorf("postings[%d].Currency = %q, want %q", i, got[i].Currency, tt.want[i].Currency)
				}
				if !got[i].BalanceAfter.Equal(tt.want[i].BalanceAfter) {
					t.Errorf("postings[%d].BalanceAfter = %s, want %s", i, got[i].BalanceAfter, tt.want[i].BalanceAfter)
				}
				if !got[i].CreatedAt.Equal(tt.want[i].CreatedAt) {
					t.Errorf("postings[%d].CreatedAt = %s, want %s", i, got[i].CreatedAt, tt.want[i].CreatedAt)
				}
			}
		})
	}
}

// TestParsePostingsEmptyPayload pins the contract that a journal without
// postings yields an empty, non-nil slice.
func TestParsePostingsEmptyPayload(t *testing.T) {
	got, err := repository.ParsePostings(nil)
	if err != nil {
		t.Fatalf("ParsePostings() error = %v", err)
	}
	if got == nil {
		t.Fatal("ParsePostings() = nil, want an empty slice")
	}
	if len(got) != 0 {
		t.Errorf("len(postings) = %d, want 0", len(got))
	}
}
