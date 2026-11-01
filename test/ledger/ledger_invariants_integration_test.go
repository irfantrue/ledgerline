package ledger

import (
	"context"
	"strings"
	"testing"

	"ledgerline/internal/ledger/domain"
	"ledgerline/internal/ledger/infrastructure/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// The ledger schema enforces its invariants in the database. These tests pin
// that behaviour down, because an invariant that only lives in application
// code can be bypassed by any other writer.

// exec runs a statement and returns the error, if any.
func exec(t *testing.T, pool *pgxpool.Pool, statement string) error {
	t.Helper()

	_, err := pool.Exec(context.Background(), statement)
	return err
}

func TestLedgerInvariants_JournalsAreAppendOnly(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	journal, _ := createBalancedPostings(t, repos, "10.00")

	t.Run("update is rejected", func(t *testing.T) {
		err := exec(t, repos.pool, "UPDATE ledger.journals SET description = 'tampered' WHERE id = '"+journal.ID.String()+"'")
		if err == nil {
			t.Fatal("UPDATE ledger.journals succeeded, want it to be rejected")
		}
		if !strings.Contains(err.Error(), "append-only") {
			t.Errorf("error = %v, want an append-only violation", err)
		}
	})

	t.Run("delete is rejected", func(t *testing.T) {
		err := exec(t, repos.pool, "DELETE FROM ledger.journals WHERE id = '"+journal.ID.String()+"'")
		if err == nil {
			t.Fatal("DELETE ledger.journals succeeded, want it to be rejected")
		}
		if !strings.Contains(err.Error(), "append-only") {
			t.Errorf("error = %v, want an append-only violation", err)
		}
	})

	t.Run("truncate is rejected", func(t *testing.T) {
		// CASCADE is needed because ledger.postings references ledger.journals.
		err := exec(t, repos.pool, "TRUNCATE ledger.journals CASCADE")
		if err == nil {
			t.Fatal("TRUNCATE ledger.journals succeeded, want it to be rejected")
		}
		if !strings.Contains(err.Error(), "append-only") {
			t.Errorf("error = %v, want an append-only violation", err)
		}
	})

	t.Run("the journal is unchanged", func(t *testing.T) {
		got, err := repos.journals.GetByID(ctx, journal.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if got.Description != "balanced journal" {
			t.Errorf("Description = %q, want %q", got.Description, "balanced journal")
		}
	})
}

func TestLedgerInvariants_PostingsAreAppendOnly(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	journal, postings := createBalancedPostings(t, repos, "10.00")
	posting := postings[0]

	t.Run("update is rejected", func(t *testing.T) {
		err := exec(t, repos.pool, "UPDATE ledger.postings SET amount = 999 WHERE id = '"+posting.ID.String()+"'")
		if err == nil {
			t.Fatal("UPDATE ledger.postings succeeded, want it to be rejected")
		}
		if !strings.Contains(err.Error(), "append-only") {
			t.Errorf("error = %v, want an append-only violation", err)
		}
	})

	t.Run("delete is rejected", func(t *testing.T) {
		err := exec(t, repos.pool, "DELETE FROM ledger.postings WHERE id = '"+posting.ID.String()+"'")
		if err == nil {
			t.Fatal("DELETE ledger.postings succeeded, want it to be rejected")
		}
		if !strings.Contains(err.Error(), "append-only") {
			t.Errorf("error = %v, want an append-only violation", err)
		}
	})

	t.Run("truncate is rejected", func(t *testing.T) {
		err := exec(t, repos.pool, "TRUNCATE ledger.postings")
		if err == nil {
			t.Fatal("TRUNCATE ledger.postings succeeded, want it to be rejected")
		}
		if !strings.Contains(err.Error(), "append-only") {
			t.Errorf("error = %v, want an append-only violation", err)
		}
	})

	t.Run("the posting is unchanged", func(t *testing.T) {
		got, err := repos.postings.GetByID(ctx, posting.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !got.Amount.Equal(decimal.RequireFromString("10.00")) {
			t.Errorf("Amount = %s, want 10.00", got.Amount)
		}
		if got.JournalID != journal.ID {
			t.Errorf("JournalID = %v, want %v", got.JournalID, journal.ID)
		}
	})
}

func TestLedgerInvariants_AccountBalanceIsProtected(t *testing.T) {
	repos := newTestRepos(t)

	account := newAccount(t, repos)

	t.Run("direct balance update is rejected", func(t *testing.T) {
		err := exec(t, repos.pool, "UPDATE ledger.accounts SET balance = 999 WHERE id = '"+account.ID.String()+"'")
		if err == nil {
			t.Fatal("UPDATE ledger.accounts SET balance succeeded, want it to be rejected")
		}
		if !strings.Contains(err.Error(), "may not be updated directly") {
			t.Errorf("error = %v, want a balance protection violation", err)
		}
	})

	t.Run("currency change is rejected", func(t *testing.T) {
		err := exec(t, repos.pool, "UPDATE ledger.accounts SET currency = 'EUR' WHERE id = '"+account.ID.String()+"'")
		if err == nil {
			t.Fatal("UPDATE ledger.accounts SET currency succeeded, want it to be rejected")
		}
		if !strings.Contains(err.Error(), "currency may not be changed") {
			t.Errorf("error = %v, want a currency protection violation", err)
		}
	})

	t.Run("normal side change is rejected", func(t *testing.T) {
		err := exec(t, repos.pool, "UPDATE ledger.accounts SET normal_side = 'credit' WHERE id = '"+account.ID.String()+"'")
		if err == nil {
			t.Fatal("UPDATE ledger.accounts SET normal_side succeeded, want it to be rejected")
		}
		if !strings.Contains(err.Error(), "normal_side may not be changed") {
			t.Errorf("error = %v, want a normal_side protection violation", err)
		}
	})
}

// TestLedgerInvariants_NegativeBalanceNeedsAllowNegative checks the
// balance_non_negative CHECK constraint.
func TestLedgerInvariants_NegativeBalanceNeedsAllowNegative(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	strict, err := repos.accounts.Create(ctx, repository.CreateAccountParams{
		Currency:      uniqueCurrency(),
		NormalSide:    domain.NormalSideDebit,
		AllowNegative: false,
	})
	if err != nil {
		t.Fatalf("Create account: %v", err)
	}

	permissive, err := repos.accounts.Create(ctx, repository.CreateAccountParams{
		Currency:      uniqueCurrency(),
		NormalSide:    domain.NormalSideDebit,
		AllowNegative: true,
	})
	if err != nil {
		t.Fatalf("Create account: %v", err)
	}

	// A credit of 10.00 drives the balance of a debit-normal account to -10.00.
	// The posting is written inside a balanced journal, so the only thing that
	// can reject it is the balance_non_negative CHECK constraint.
	credit := func(account *domain.Account) error {
		return tryInTx(t, repos, func(ctx context.Context) error {
			journals := repos.journals
			postings := repos.postings
			accounts := repos.accounts

			entry, err := journals.Create(ctx, repository.CreateJournalParams{
				IdempotencyKey: uniqueIdempotencyKey(t),
				Description:    "negative balance",
			})
			if err != nil {
				return err
			}

			other, err := accounts.Create(ctx, repository.CreateAccountParams{
				Currency:   uniqueCurrency(),
				NormalSide: domain.NormalSideDebit,
			})
			if err != nil {
				return err
			}

			if _, err := postings.Create(ctx, repository.CreatePostingParams{
				JournalID: entry.ID,
				AccountID: account.ID,
				Direction: domain.DirectionCredit,
				Amount:    decimal.RequireFromString("10.00"),
				Currency:  account.Currency,
			}); err != nil {
				return err
			}

			if _, err := postings.Create(ctx, repository.CreatePostingParams{
				JournalID: entry.ID,
				AccountID: other.ID,
				Direction: domain.DirectionDebit,
				Amount:    decimal.RequireFromString("10.00"),
				Currency:  other.Currency,
			}); err != nil {
				return err
			}

			return nil
		})
	}

	if err := credit(strict); err == nil {
		t.Error("Create() on an account that forbids negative balances succeeded, want it to be rejected")
	} else if !strings.Contains(err.Error(), "balance_non_negative") {
		t.Errorf("error = %v, want a balance_non_negative violation", err)
	}

	if err := credit(permissive); err != nil {
		t.Errorf("Create() on an account that allows negative balances error = %v, want it to succeed", err)
	}
}

// TestLedgerInvariants_PostingsBelongToAnExistingJournal checks the foreign
// keys of ledger.postings.
func TestLedgerInvariants_PostingsBelongToAnExistingJournal(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	account := newAccount(t, repos)
	journal := newJournal(t, repos)

	t.Run("unknown account is rejected", func(t *testing.T) {
		_, err := repos.postings.Create(ctx, repository.CreatePostingParams{
			JournalID: journal.ID,
			AccountID: uuid.MustParse("99999999-9999-7999-8999-999999999999"),
			Direction: domain.DirectionDebit,
			Amount:    decimal.RequireFromString("10.00"),
			Currency:  account.Currency,
		})
		if err == nil {
			t.Fatal("Create() with an unknown account succeeded, want it to be rejected")
		}
	})

	t.Run("unknown journal is rejected", func(t *testing.T) {
		_, err := repos.postings.Create(ctx, repository.CreatePostingParams{
			JournalID: uuid.MustParse("99999999-9999-7999-8999-999999999999"),
			AccountID: account.ID,
			Direction: domain.DirectionDebit,
			Amount:    decimal.RequireFromString("10.00"),
			Currency:  account.Currency,
		})
		if err == nil {
			t.Fatal("Create() with an unknown journal succeeded, want it to be rejected")
		}
	})
}

// TestLedgerInvariants_PostingAmountMustBePositive checks the amount CHECK
// constraint.
func TestLedgerInvariants_PostingAmountMustBePositive(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	account := newAccount(t, repos)
	journal := newJournal(t, repos)

	_, err := repos.postings.Create(ctx, repository.CreatePostingParams{
		JournalID: journal.ID,
		AccountID: account.ID,
		Direction: domain.DirectionDebit,
		Amount:    decimal.Zero,
		Currency:  account.Currency,
	})
	if err == nil {
		t.Fatal("Create() with a zero amount succeeded, want it to be rejected")
	}
}

// TestLedgerInvariants_BalanceTriggerUpdatesTheAccount makes sure the balance
// is maintained by the database and not by the application.
func TestLedgerInvariants_BalanceTriggerUpdatesTheAccount(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()

	account := newAccount(t, repos)
	amount := decimal.RequireFromString("64.00")

	createPosting(t, repos, account, domain.DirectionDebit, amount)

	updated, err := repos.accounts.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !updated.Balance.Equal(amount) {
		t.Errorf("Balance = %s, want %s", updated.Balance, amount)
	}

	// The credit side of the same entry moved its own account too.
	summary, err := repos.postings.GetSummaryByAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetSummaryByAccount() error = %v", err)
	}
	if summary.PostingCount != 1 {
		t.Errorf("PostingCount = %d, want 1", summary.PostingCount)
	}
	if !summary.TotalDebits.Equal(amount) {
		t.Errorf("TotalDebits = %s, want %s", summary.TotalDebits, amount)
	}
}
