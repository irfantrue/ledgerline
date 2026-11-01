package repository

import (
	"context"
	"errors"
	"fmt"

	"ledgerline/internal/ledger/domain"
	"ledgerline/internal/ledger/infrastructure/sqlcgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// postingRepository is a PostgreSQL-backed implementation of PostingRepository.
type postingRepository struct {
	q Querier
}

// NewPostingRepository creates a new PostgreSQL-backed PostingRepository.
// The returned repository resolves its database handle per call, so it joins
// the transaction that the context of a call carries.
func NewPostingRepository(q Querier) PostingRepository {
	return &postingRepository{q: q}
}

// queries resolves the database handle for this call. Resolving it per call
// is what lets the repository join the transaction that the context carries.
func (r *postingRepository) queries(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(r.q.Querier(ctx))
}

// Create creates a new posting with the given parameters.
// The posting ID is generated automatically using uuid_generate_v7().
// This operation will also update the associated account's balance via database triggers.
func (r *postingRepository) Create(ctx context.Context, arg CreatePostingParams) (*domain.Posting, error) {
	result, err := r.queries(ctx).CreatePosting(ctx, sqlcgen.CreatePostingParams{
		JournalID: arg.JournalID,
		AccountID: arg.AccountID,
		Direction: string(arg.Direction),
		Amount:    arg.Amount,
		Currency:  arg.Currency,
	})
	if err != nil {
		return nil, fmt.Errorf("posting: create: %w", err)
	}
	return ToPostingDomain(&result), nil
}

// GetByID retrieves a posting by its unique identifier.
func (r *postingRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Posting, error) {
	result, err := r.queries(ctx).GetPosting(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("posting: get by id %s: %w", id, ErrPostingNotFound)
		}
		return nil, fmt.Errorf("posting: get by id %s: %w", id, err)
	}
	return ToPostingDomain(&result), nil
}

// List retrieves all postings with pagination support.
// Results are ordered by created_at descending (newest first).
func (r *postingRepository) List(ctx context.Context, limit, offset int32) ([]domain.Posting, error) {
	results, err := r.queries(ctx).ListPostings(ctx, sqlcgen.ListPostingsParams{
		LimitRows:  limit,
		OffsetRows: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("posting: list: %w", err)
	}
	postings := make([]domain.Posting, len(results))
	for i := range results {
		postings[i] = *ToPostingDomain(&results[i])
	}
	return postings, nil
}

// ListByAccount retrieves all postings for a specific account with pagination.
// Results are ordered by created_at descending (newest first).
func (r *postingRepository) ListByAccount(ctx context.Context, accountID uuid.UUID, limit, offset int32) ([]domain.Posting, error) {
	results, err := r.queries(ctx).ListPostingsByAccount(ctx, sqlcgen.ListPostingsByAccountParams{
		AccountID:  accountID,
		LimitRows:  limit,
		OffsetRows: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("posting: list by account %s: %w", accountID, err)
	}
	postings := make([]domain.Posting, len(results))
	for i := range results {
		postings[i] = *ToPostingDomain(&results[i])
	}
	return postings, nil
}

// ListByJournal retrieves all postings for a specific journal.
// Results are ordered by created_at ascending (chronological order within the journal).
func (r *postingRepository) ListByJournal(ctx context.Context, journalID uuid.UUID) ([]domain.Posting, error) {
	results, err := r.queries(ctx).ListPostingsByJournal(ctx, journalID)
	if err != nil {
		return nil, fmt.Errorf("posting: list by journal %s: %w", journalID, err)
	}
	postings := make([]domain.Posting, len(results))
	for i := range results {
		postings[i] = *ToPostingDomain(&results[i])
	}
	return postings, nil
}

// GetSummaryByAccount retrieves aggregated posting statistics for an account.
// This includes total debits, total credits, posting count, and current balance.
func (r *postingRepository) GetSummaryByAccount(ctx context.Context, accountID uuid.UUID) (*domain.PostingsSummary, error) {
	result, err := r.queries(ctx).GetPostingsSummaryByAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("posting: summary by account %s: %w", accountID, ErrPostingNotFound)
		}
		return nil, fmt.Errorf("posting: summary by account %s: %w", accountID, err)
	}

	return &domain.PostingsSummary{
		AccountID:    result.AccountID,
		PostingCount: result.PostingCount,
		TotalDebits:  result.TotalDebits,
		TotalCredits: result.TotalCredits,
		BalanceAfter: result.BalanceAfter,
	}, nil
}

// ToPostingDomain converts a sqlcgen.LedgerPosting to a domain.Posting.
func ToPostingDomain(p *sqlcgen.LedgerPosting) *domain.Posting {
	return &domain.Posting{
		ID:           p.ID,
		JournalID:    p.JournalID,
		AccountID:    p.AccountID,
		Direction:    domain.Direction(p.Direction),
		Amount:       p.Amount,
		Currency:     p.Currency,
		BalanceAfter: p.BalanceAfter,
		CreatedAt:    p.CreatedAt,
	}
}
