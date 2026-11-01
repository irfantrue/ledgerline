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

// accountRepository is a PostgreSQL-backed implementation of AccountRepository.
type accountRepository struct {
	q Querier
}

// NewAccountRepository creates a new PostgreSQL-backed AccountRepository.
// The returned repository resolves its database handle per call, so it joins
// the transaction that the context of a call carries.
func NewAccountRepository(q Querier) AccountRepository {
	return &accountRepository{q: q}
}

// queries resolves the database handle for this call. Resolving it per call
// is what lets the repository join the transaction that the context carries.
func (r *accountRepository) queries(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(r.q.Querier(ctx))
}

// Create creates a new account with the given parameters.
// The account ID is generated automatically using uuid_generate_v7().
// Initial balance is always 0 and version starts at 0; the database
// increments version every time a posting changes the balance.
func (r *accountRepository) Create(ctx context.Context, arg CreateAccountParams) (*domain.Account, error) {
	result, err := r.queries(ctx).CreateAccount(ctx, sqlcgen.CreateAccountParams{
		Currency:      arg.Currency,
		NormalSide:    string(arg.NormalSide),
		AllowNegative: arg.AllowNegative,
	})
	if err != nil {
		return nil, fmt.Errorf("account: create: %w", err)
	}
	return ToAccountDomain(&result), nil
}

// GetByID retrieves an account by its unique identifier.
func (r *accountRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	result, err := r.queries(ctx).GetAccount(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("account: get by id %s: %w", id, ErrAccountNotFound)
		}
		return nil, fmt.Errorf("account: get by id %s: %w", id, err)
	}
	return ToAccountDomain(&result), nil
}

// GetByCurrency retrieves the most recently created account for a given currency.
// This is useful when you want to find a default account for a currency.
func (r *accountRepository) GetByCurrency(ctx context.Context, currency string) (*domain.Account, error) {
	result, err := r.queries(ctx).GetAccountByCurrency(ctx, currency)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("account: get by currency %q: %w", currency, ErrAccountNotFound)
		}
		return nil, fmt.Errorf("account: get by currency %q: %w", currency, err)
	}
	return ToAccountDomain(&result), nil
}

// List retrieves accounts with pagination support.
// Results are ordered by created_at descending (newest first).
// Use limit=0 to retrieve all accounts (not recommended for large datasets).
func (r *accountRepository) List(ctx context.Context, limit, offset int32) ([]domain.Account, error) {
	results, err := r.queries(ctx).ListAccounts(ctx, sqlcgen.ListAccountsParams{
		LimitRows:  limit,
		OffsetRows: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("account: list: %w", err)
	}
	accounts := make([]domain.Account, len(results))
	for i := range results {
		accounts[i] = *ToAccountDomain(&results[i])
	}
	return accounts, nil
}

// ToAccountDomain converts a sqlcgen.LedgerAccount to a domain.Account.
func ToAccountDomain(a *sqlcgen.LedgerAccount) *domain.Account {
	return &domain.Account{
		ID:            a.ID,
		Currency:      a.Currency,
		NormalSide:    domain.NormalSide(a.NormalSide),
		AllowNegative: a.AllowNegative,
		Balance:       a.Balance,
		Version:       a.Version,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
}
