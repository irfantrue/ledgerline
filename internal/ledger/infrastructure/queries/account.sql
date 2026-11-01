-- name: CreateAccount :one
INSERT INTO ledger.accounts (
    id,
    currency,
    normal_side,
    allow_negative,
    balance,
    version
) VALUES (
    uuidv7(),
    @currency,
    @normal_side,
    @allow_negative,
    0,
    0
)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM ledger.accounts
WHERE id = @id;

-- name: ListAccounts :many
SELECT * FROM ledger.accounts
ORDER BY created_at DESC
LIMIT NULLIF(@limit_rows::int, 0)
OFFSET @offset_rows;

-- name: GetAccountByCurrency :one
SELECT * FROM ledger.accounts
WHERE currency = @currency
ORDER BY created_at DESC
LIMIT 1;
