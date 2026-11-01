package repository

import (
	"context"

	"ledgerline/internal/ledger/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// AccountRepository defines the interface for account persistence operations.
// Implementations should handle validation and mapping between domain and storage models.
type AccountRepository interface {
	// Create creates a new account and returns the created account.
	// The account ID is generated automatically using uuid_generate_v7().
	// Initial balance is always 0 and version starts at 0; the database
	// increments version every time a posting changes the balance.
	Create(ctx context.Context, arg CreateAccountParams) (*domain.Account, error)

	// GetByID retrieves an account by its ID.
	// Returns ErrAccountNotFound if the account does not exist.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error)

	// GetByCurrency retrieves an account by its currency code.
	// Returns the most recently created account for the given currency.
	// Returns ErrAccountNotFound if no account exists for the currency.
	GetByCurrency(ctx context.Context, currency string) (*domain.Account, error)

	// List retrieves accounts with pagination.
	// Results are ordered by created_at descending (newest first).
	// Use limit=0 to retrieve all accounts (not recommended for large datasets).
	List(ctx context.Context, limit, offset int32) ([]domain.Account, error)
}

// CreateAccountParams holds parameters for creating a new account.
// All fields are required unless otherwise specified.
type CreateAccountParams struct {
	// Currency is the 3-letter ISO currency code (e.g., "USD", "EUR").
	Currency string

	// NormalSide specifies whether the account's normal balance is debit or credit.
	// Asset and Expense accounts typically use debit.
	// Liability, Equity, and Revenue accounts typically use credit.
	NormalSide domain.NormalSide

	// AllowNegative indicates whether the account balance is allowed to go negative.
	// When false, database constraints prevent balance from becoming negative.
	AllowNegative bool
}

// PostingRepository defines the interface for posting (ledger entry) persistence operations.
// Postings are append-only and cannot be modified or deleted after creation.
type PostingRepository interface {
	// Create creates a new posting and returns the created posting.
	// The posting ID is generated automatically using uuid_generate_v7().
	// This will also update the account's balance via database triggers.
	Create(ctx context.Context, arg CreatePostingParams) (*domain.Posting, error)

	// GetByID retrieves a posting by its ID.
	// Returns ErrPostingNotFound if the posting does not exist.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Posting, error)

	// List retrieves all postings with pagination.
	// Results are ordered by created_at descending (newest first).
	List(ctx context.Context, limit, offset int32) ([]domain.Posting, error)

	// ListByAccount retrieves postings for a specific account with pagination.
	// Results are ordered by created_at descending (newest first).
	ListByAccount(ctx context.Context, accountID uuid.UUID, limit, offset int32) ([]domain.Posting, error)

	// ListByJournal retrieves all postings for a specific journal.
	// Results are ordered by created_at ascending (chronological order).
	ListByJournal(ctx context.Context, journalID uuid.UUID) ([]domain.Posting, error)

	// GetSummaryByAccount retrieves aggregated posting data for an account.
	// Returns total debits, total credits, posting count, and current balance.
	GetSummaryByAccount(ctx context.Context, accountID uuid.UUID) (*domain.PostingsSummary, error)
}

// CreatePostingParams holds parameters for creating a new posting.
// All fields are required unless otherwise specified.
type CreatePostingParams struct {
	// JournalID is the ID of the journal this posting belongs to.
	// All postings in a journal must balance (total debits = total credits).
	JournalID uuid.UUID

	// AccountID is the ID of the account to post to.
	AccountID uuid.UUID

	// Direction specifies whether this is a debit or credit posting.
	Direction domain.Direction

	// Amount is the posting amount. Must be positive.
	// The amount's currency must match the account's currency.
	Amount decimal.Decimal

	// Currency is the 3-letter ISO currency code for this posting.
	// Must match the account's currency.
	Currency string
}

// JournalRepository defines the interface for journal persistence operations.
// Journals are the parent entity for postings and represent a complete transaction.
type JournalRepository interface {
	// Create creates a new journal and returns the created journal.
	// The journal ID is generated automatically using uuid_generate_v7().
	// The idempotency_key ensures duplicate requests return the same journal.
	Create(ctx context.Context, arg CreateJournalParams) (*domain.Journal, error)

	// GetByID retrieves a journal by its ID.
	// Returns ErrJournalNotFound if the journal does not exist.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Journal, error)

	// GetByIdempotencyKey retrieves a journal by its idempotency key.
	// This allows safe retry of journal creation requests.
	// Returns ErrJournalNotFound if no journal exists with the given key.
	GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.Journal, error)

	// List retrieves journals with pagination.
	// Results are ordered by created_at descending (newest first).
	List(ctx context.Context, limit, offset int32) ([]domain.Journal, error)

	// GetWithPostings retrieves a journal along with all its postings.
	// Postings are ordered by created_at ascending (chronological order).
	// Returns ErrJournalNotFound if the journal does not exist.
	GetWithPostings(ctx context.Context, id uuid.UUID) (*domain.JournalWithPostings, error)
}

// CreateJournalParams holds parameters for creating a new journal.
// All fields are required unless otherwise specified.
type CreateJournalParams struct {
	// IdempotencyKey is a unique key to prevent duplicate journal creation.
	// If a journal with this key already exists, it will be returned instead of creating a new one.
	// Should be provided by the client (e.g., client-generated UUID or business ID).
	IdempotencyKey string

	// Reference is an optional reference to another entity (e.g., invoice ID, order ID).
	Reference *uuid.UUID

	// Description is a human-readable description of the journal entry.
	Description string
}
