-- Schema used by sqlc for type inference. It must mirror the tables created by
-- migrations/ledger/*.sql - the migrations are the source of truth.

CREATE SCHEMA IF NOT EXISTS ledger;

CREATE TABLE IF NOT EXISTS ledger.accounts (
    id             UUID PRIMARY KEY DEFAULT uuidv7(),
    currency       TEXT          NOT NULL CHECK (char_length(currency) = 3),
    normal_side    TEXT          NOT NULL CHECK (normal_side IN ('debit', 'credit')),
    allow_negative BOOLEAN       NOT NULL DEFAULT FALSE,
    balance        NUMERIC(18,2) NOT NULL DEFAULT 0,
    version        BIGINT        NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CONSTRAINT balance_non_negative CHECK (allow_negative OR balance >= 0)
);

CREATE TABLE IF NOT EXISTS ledger.journals (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    idempotency_key TEXT        NOT NULL UNIQUE,
    reference       UUID        DEFAULT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ledger.postings (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    journal_id    UUID          NOT NULL REFERENCES ledger.journals (id),
    account_id    UUID          NOT NULL REFERENCES ledger.accounts (id),
    direction     TEXT          NOT NULL CHECK (direction IN ('debit', 'credit')),
    amount        NUMERIC(18,2) NOT NULL CHECK (amount > 0),
    currency      TEXT          NOT NULL CHECK (char_length(currency) = 3),
    balance_after NUMERIC(18,2) NOT NULL,
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT now()
);
