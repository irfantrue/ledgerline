package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ledgerline/internal/ledger/domain"
	"ledgerline/internal/ledger/infrastructure/sqlcgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// journalRepository is a PostgreSQL-backed implementation of JournalRepository.
type journalRepository struct {
	q Querier
}

// NewJournalRepository creates a new PostgreSQL-backed JournalRepository.
// The returned repository resolves its database handle per call, so it joins
// the transaction that the context of a call carries.
func NewJournalRepository(q Querier) JournalRepository {
	return &journalRepository{q: q}
}

// queries resolves the database handle for this call. Resolving it per call
// is what lets the repository join the transaction that the context carries.
func (r *journalRepository) queries(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(r.q.Querier(ctx))
}

// Create creates a new journal with the given parameters.
// The journal ID is generated automatically using uuid_generate_v7().
// If a journal with the same idempotency_key already exists, it is returned
// unchanged instead of creating a duplicate.
func (r *journalRepository) Create(ctx context.Context, arg CreateJournalParams) (*domain.Journal, error) {
	var reference pgtype.UUID
	if arg.Reference != nil {
		reference = pgtype.UUID{Bytes: *arg.Reference, Valid: true}
	}

	result, err := r.queries(ctx).CreateJournal(ctx, sqlcgen.CreateJournalParams{
		IdempotencyKey: arg.IdempotencyKey,
		Reference:      reference,
		Description:    arg.Description,
	})
	if err != nil {
		// The insert is a no-op when the idempotency key is already taken, so
		// read the original journal back instead of failing the caller.
		if errors.Is(err, pgx.ErrNoRows) {
			existing, getErr := r.queries(ctx).GetJournalByIdempotencyKey(ctx, arg.IdempotencyKey)
			if getErr != nil {
				return nil, fmt.Errorf("journal: get by idempotency key %q: %w", arg.IdempotencyKey, getErr)
			}
			return ToJournalDomain(&existing), nil
		}
		return nil, fmt.Errorf("journal: create: %w", err)
	}
	return ToJournalDomain(&result), nil
}

// GetByID retrieves a journal by its unique identifier.
func (r *journalRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Journal, error) {
	result, err := r.queries(ctx).GetJournal(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("journal: get by id %s: %w", id, ErrJournalNotFound)
		}
		return nil, fmt.Errorf("journal: get by id %s: %w", id, err)
	}
	return ToJournalDomain(&result), nil
}

// GetByIdempotencyKey retrieves a journal by its idempotency key.
// This allows safe retry of journal creation requests - if a journal with
// the same idempotency_key already exists, it will be returned.
func (r *journalRepository) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.Journal, error) {
	result, err := r.queries(ctx).GetJournalByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("journal: get by idempotency key %q: %w", idempotencyKey, ErrJournalNotFound)
		}
		return nil, fmt.Errorf("journal: get by idempotency key %q: %w", idempotencyKey, err)
	}
	return ToJournalDomain(&result), nil
}

// List retrieves journals with pagination support.
// Results are ordered by created_at descending (newest first).
// Use limit=0 to retrieve all journals (not recommended for large datasets).
func (r *journalRepository) List(ctx context.Context, limit, offset int32) ([]domain.Journal, error) {
	results, err := r.queries(ctx).ListJournals(ctx, sqlcgen.ListJournalsParams{
		LimitRows:  limit,
		OffsetRows: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("journal: list: %w", err)
	}
	journals := make([]domain.Journal, len(results))
	for i := range results {
		journals[i] = *ToJournalDomain(&results[i])
	}
	return journals, nil
}

// GetWithPostings retrieves a journal along with all its associated postings.
// Postings are ordered by created_at ascending (chronological order).
// The postings are returned as a JSON array from the database and unmarshaled
// into domain.Posting objects.
func (r *journalRepository) GetWithPostings(ctx context.Context, id uuid.UUID) (*domain.JournalWithPostings, error) {
	row, err := r.queries(ctx).GetJournalWithPostings(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("journal: get with postings %s: %w", id, ErrJournalNotFound)
		}
		return nil, fmt.Errorf("journal: get with postings %s: %w", id, err)
	}

	journal := ToJournalDomain(&sqlcgen.LedgerJournal{
		ID:             row.ID,
		IdempotencyKey: row.IdempotencyKey,
		Reference:      row.Reference,
		Description:    row.Description,
		CreatedAt:      row.CreatedAt,
	})

	postings, err := ParsePostings(row.Postings)
	if err != nil {
		return nil, fmt.Errorf("journal: get with postings %s: %w", id, err)
	}

	return &domain.JournalWithPostings{
		Journal:  *journal,
		Postings: postings,
	}, nil
}

// postingJSON mirrors a single entry of the json_agg() payload produced by the
// GetJournalWithPostings query.
type postingJSON struct {
	ID           string      `json:"id"`
	JournalID    string      `json:"journal_id"`
	AccountID    string      `json:"account_id"`
	Direction    string      `json:"direction"`
	Amount       numericText `json:"amount"`
	Currency     string      `json:"currency"`
	BalanceAfter numericText `json:"balance_after"`
	CreatedAt    string      `json:"created_at"`
}

// numericText holds a decimal that the database emitted as JSON text. It also
// accepts a bare JSON number so the parser keeps working if the query stops
// casting to text. The raw text is kept on purpose: decoding into float64
// would lose precision for NUMERIC(18,2) amounts.
type numericText string

// UnmarshalJSON implements json.Unmarshaler.
func (n *numericText) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*n = ""
		return nil
	}

	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*n = numericText(s)
		return nil
	}

	*n = numericText(b)
	return nil
}

// ParsePostings decodes the json_agg() payload of GetJournalWithPostings.
// Malformed entries are reported as errors instead of being silently dropped,
// so a corrupt payload can never be mistaken for an empty journal.
func ParsePostings(raw []byte) ([]domain.Posting, error) {
	if len(raw) == 0 {
		return []domain.Posting{}, nil
	}

	var entries []postingJSON
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode postings: %w", err)
	}

	postings := make([]domain.Posting, 0, len(entries))
	for i, entry := range entries {
		posting, err := entry.toDomain()
		if err != nil {
			return nil, fmt.Errorf("decode posting %d: %w", i, err)
		}
		postings = append(postings, posting)
	}
	return postings, nil
}

// toDomain converts a postingJSON entry into a domain.Posting.
func (p postingJSON) toDomain() (domain.Posting, error) {
	id, err := uuid.Parse(p.ID)
	if err != nil {
		return domain.Posting{}, fmt.Errorf("id: %w", err)
	}

	journalID, err := uuid.Parse(p.JournalID)
	if err != nil {
		return domain.Posting{}, fmt.Errorf("journal_id: %w", err)
	}

	accountID, err := uuid.Parse(p.AccountID)
	if err != nil {
		return domain.Posting{}, fmt.Errorf("account_id: %w", err)
	}

	amount, err := decimal.NewFromString(string(p.Amount))
	if err != nil {
		return domain.Posting{}, fmt.Errorf("amount: %w", err)
	}

	balanceAfter, err := decimal.NewFromString(string(p.BalanceAfter))
	if err != nil {
		return domain.Posting{}, fmt.Errorf("balance_after: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339, p.CreatedAt)
	if err != nil {
		return domain.Posting{}, fmt.Errorf("created_at: %w", err)
	}

	return domain.Posting{
		ID:           id,
		JournalID:    journalID,
		AccountID:    accountID,
		Direction:    domain.Direction(p.Direction),
		Amount:       amount,
		Currency:     p.Currency,
		BalanceAfter: balanceAfter,
		CreatedAt:    createdAt,
	}, nil
}

// ToJournalDomain converts a sqlcgen.LedgerJournal to a domain.Journal.
func ToJournalDomain(j *sqlcgen.LedgerJournal) *domain.Journal {
	var reference *uuid.UUID
	if j.Reference.Valid {
		refUUID := uuid.UUID(j.Reference.Bytes)
		reference = &refUUID
	}
	return &domain.Journal{
		ID:             j.ID,
		IdempotencyKey: j.IdempotencyKey,
		Reference:      reference,
		Description:    j.Description,
		CreatedAt:      j.CreatedAt,
	}
}
