-- name: CreatePosting :one
-- balance_after is not a parameter: the database derives it. The BEFORE INSERT
-- trigger ledger.set_posting_balance_after() locks the account and computes
-- the balance this posting leaves behind, so the application cannot supply a
-- stale or wrong value.
INSERT INTO ledger.postings (
    id,
    journal_id,
    account_id,
    direction,
    amount,
    currency,
    created_at
) VALUES (
    uuidv7(),
    @journal_id,
    @account_id,
    @direction,
    @amount,
    @currency,
    now()
)
RETURNING *;

-- name: GetPosting :one
SELECT * FROM ledger.postings
WHERE id = @id;

-- name: ListPostings :many
SELECT * FROM ledger.postings
ORDER BY created_at DESC
LIMIT NULLIF(@limit_rows::int, 0)
OFFSET @offset_rows;

-- name: ListPostingsByAccount :many
SELECT * FROM ledger.postings
WHERE account_id = @account_id
ORDER BY created_at DESC
LIMIT NULLIF(@limit_rows::int, 0)
OFFSET @offset_rows;

-- name: ListPostingsByJournal :many
SELECT * FROM ledger.postings
WHERE journal_id = @journal_id
ORDER BY created_at ASC;

-- name: GetPostingsSummaryByAccount :one
-- balance_after must come from the account, not from the postings: grouping by
-- ledger.postings.balance_after would produce one row per distinct balance and
-- the :one variant would silently keep only the first one.
SELECT
    p.account_id,
    COUNT(*) as posting_count,
    COALESCE(SUM(CASE WHEN p.direction = 'debit' THEN p.amount ELSE 0 END), 0)::numeric as total_debits,
    COALESCE(SUM(CASE WHEN p.direction = 'credit' THEN p.amount ELSE 0 END), 0)::numeric as total_credits,
    a.balance as balance_after
FROM ledger.postings p
JOIN ledger.accounts a ON a.id = p.account_id
WHERE p.account_id = @account_id
GROUP BY p.account_id, a.balance;
