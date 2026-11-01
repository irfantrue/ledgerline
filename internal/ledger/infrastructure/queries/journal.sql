-- name: CreateJournal :one
-- ON CONFLICT DO NOTHING keeps the insert idempotent: replaying the same
-- idempotency_key returns no row, and the repository then reads back the
-- journal that already exists. ON CONFLICT DO UPDATE is not an option here
-- because ledger.journals is append-only and rejects UPDATE.
INSERT INTO ledger.journals (
    id,
    idempotency_key,
    reference,
    description,
    created_at
) VALUES (
    uuidv7(),
    @idempotency_key,
    @reference,
    @description,
    now()
)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING *;

-- name: GetJournal :one
SELECT * FROM ledger.journals
WHERE id = @id;

-- name: GetJournalByIdempotencyKey :one
SELECT * FROM ledger.journals
WHERE idempotency_key = @idempotency_key;

-- name: ListJournals :many
SELECT * FROM ledger.journals
ORDER BY created_at DESC
LIMIT NULLIF(@limit_rows::int, 0)
OFFSET @offset_rows;

-- name: GetJournalWithPostings :one
-- Numerics are cast to text on purpose: JSON numbers are decoded as float64 by
-- encoding/json, which would lose precision for NUMERIC(18,2) amounts.
SELECT
    j.*,
    json_agg(
        json_build_object(
            'id', p.id,
            'journal_id', p.journal_id,
            'account_id', p.account_id,
            'direction', p.direction,
            'amount', p.amount::text,
            'currency', p.currency,
            'balance_after', p.balance_after::text,
            'created_at', p.created_at
        )
    ) FILTER (WHERE p.id IS NOT NULL) AS postings
FROM ledger.journals j
LEFT JOIN ledger.postings p ON p.journal_id = j.id
WHERE j.id = @id
GROUP BY j.id;
