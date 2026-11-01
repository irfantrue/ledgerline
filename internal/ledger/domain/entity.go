package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// NormalSide represents the normal side of an account in double-entry bookkeeping.
// In accounting, every account has a normal balance side: debit or credit.
// - Asset and Expense accounts normally have debit balances (increase with debits)
// - Liability, Equity, and Revenue accounts normally have credit balances (increase with credits)
type NormalSide string

const (
	// NormalSideDebit indicates the account's normal balance is on the debit side.
	NormalSideDebit NormalSide = "debit"

	// NormalSideCredit indicates the account's normal balance is on the credit side.
	NormalSideCredit NormalSide = "credit"
)

// Direction represents the direction of a posting (debit or credit) in a journal entry.
// A posting direction indicates whether the amount is being debited or credited to an account.
type Direction string

const (
	// DirectionDebit indicates a debit posting.
	DirectionDebit Direction = "debit"

	// DirectionCredit indicates a credit posting.
	DirectionCredit Direction = "credit"
)

// Account represents a ledger account in the domain layer.
// An account is a record in the general ledger that tracks monetary transactions
// for a specific type of asset, liability, equity, revenue, or expense.
type Account struct {
	// ID is the unique identifier for the account.
	ID uuid.UUID

	// Currency is the 3-letter ISO 4217 currency code (e.g., "USD", "EUR", "GBP").
	Currency string

	// NormalSide specifies whether debits or credits increase the account's balance.
	NormalSide NormalSide

	// AllowNegative indicates whether the account can have a negative balance.
	// When false, business rules or database constraints prevent negative balances.
	AllowNegative bool

	// Balance is the current balance of the account.
	// For accounts with normal_side=debit, positive balance means debit balance.
	// For accounts with normal_side=credit, positive balance means credit balance.
	Balance decimal.Decimal

	// Version counts the balance changes of the account. The database
	// increments it every time a posting changes the balance, so two reads
	// that carry the same version saw the same balance.
	//
	// It is a revision counter, not a compare-and-swap token: no account
	// update path compares it.
	Version int64

	// CreatedAt is the timestamp when the account was created.
	CreatedAt time.Time

	// UpdatedAt is the timestamp when the account was last modified.
	UpdatedAt time.Time
}

// Journal represents a journal entry in the domain layer.
// A journal is a chronological record of financial transactions.
// Each journal entry contains one or more postings that must balance (total debits = total credits).
type Journal struct {
	// ID is the unique identifier for the journal entry.
	ID uuid.UUID

	// IdempotencyKey is a unique key provided by the client to prevent duplicate journal creation.
	// If a journal with this key already exists, the same journal will be returned on subsequent requests.
	IdempotencyKey string

	// Reference is an optional reference to another entity (e.g., invoice ID, order ID, employee ID).
	Reference *uuid.UUID

	// Description is a human-readable description of the journal entry.
	Description string

	// CreatedAt is the timestamp when the journal entry was created.
	CreatedAt time.Time
}

// Posting represents a single line item (entry) within a journal in the domain layer.
// A posting records the debiting or crediting of a specific account as part of a journal entry.
// In double-entry bookkeeping, every posting must have a corresponding balancing posting.
type Posting struct {
	// ID is the unique identifier for the posting.
	ID uuid.UUID

	// JournalID is the ID of the journal this posting belongs to.
	// All postings with the same JournalID must balance (total debits = total credits).
	JournalID uuid.UUID

	// AccountID is the ID of the account being debited or credited.
	AccountID uuid.UUID

	// Direction specifies whether this is a debit or credit posting.
	Direction Direction

	// Amount is the monetary amount of the posting.
	// Must be positive. The sign is determined by Direction.
	Amount decimal.Decimal

	// Currency is the 3-letter ISO 4217 currency code for this posting.
	// Must match the currency of the associated account.
	Currency string

	// BalanceAfter is the account balance after this posting was applied.
	// This is stored for audit trail purposes and to validate consistency.
	BalanceAfter decimal.Decimal

	// CreatedAt is the timestamp when the posting was created.
	CreatedAt time.Time
}

// JournalWithPostings represents a journal entry along with all its associated postings.
// This is a convenience structure for retrieving a complete journal entry with all its line items.
type JournalWithPostings struct {
	// Journal is the journal entry itself.
	Journal Journal

	// Postings is the list of all postings belonging to this journal.
	// For a balanced journal, the sum of debits equals the sum of credits.
	Postings []Posting
}

// PostingsSummary represents aggregated posting statistics for an account.
// This provides a quick overview of an account's activity without listing individual postings.
type PostingsSummary struct {
	// AccountID is the ID of the account these statistics apply to.
	AccountID uuid.UUID

	// PostingCount is the total number of postings in the account.
	PostingCount int64

	// TotalDebits is the sum of all debit postings to the account.
	TotalDebits decimal.Decimal

	// TotalCredits is the sum of all credit postings to the account.
	TotalCredits decimal.Decimal

	// BalanceAfter is the current balance of the account.
	// For a debit-normal account: Balance = TotalDebits - TotalCredits.
	// For a credit-normal account: Balance = TotalCredits - TotalDebits.
	BalanceAfter decimal.Decimal
}
