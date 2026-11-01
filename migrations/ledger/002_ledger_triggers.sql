-- Migration: ledger triggers - the double-entry invariants.
--
-- The tables live in 001_ledger.sql. Every rule below is enforced by the
-- database, so it holds for any writer and not only for this application.
--
-- Note: uuidv7() requires PostgreSQL 18+. On older versions use the app's pkg/idgen.

-- =============================================================================
-- APPEND-ONLY ENFORCEMENT
-- Journals and postings are append-only: UPDATE/DELETE/TRUNCATE are rejected.
-- =============================================================================

CREATE OR REPLACE FUNCTION ledger.reject_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ledger.% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS journals_append_only ON ledger.journals;

CREATE TRIGGER journals_append_only
    BEFORE UPDATE OR DELETE ON ledger.journals
    FOR EACH ROW EXECUTE FUNCTION ledger.reject_mutation();

DROP TRIGGER IF EXISTS postings_append_only ON ledger.postings;

CREATE TRIGGER postings_append_only
    BEFORE UPDATE OR DELETE ON ledger.postings
    FOR EACH ROW EXECUTE FUNCTION ledger.reject_mutation();

-- Trigger row-level does not fire on TRUNCATE, so separate statement-level trigger.
DROP TRIGGER IF EXISTS journals_no_truncate ON ledger.journals;

CREATE TRIGGER journals_no_truncate
    BEFORE TRUNCATE ON ledger.journals
    FOR EACH STATEMENT EXECUTE FUNCTION ledger.reject_mutation();

DROP TRIGGER IF EXISTS postings_no_truncate ON ledger.postings;

CREATE TRIGGER postings_no_truncate
    BEFORE TRUNCATE ON ledger.postings
    FOR EACH STATEMENT EXECUTE FUNCTION ledger.reject_mutation();

-- =============================================================================
-- DOUBLE-ENTRY ENFORCEMENT
-- Every journal must have balanced debits and credits (SUM(amount) per journal).
-- =============================================================================

-- Function to check that a journal's postings are balanced.
CREATE OR REPLACE FUNCTION ledger.ensure_balanced_postings() RETURNS trigger AS $$
DECLARE
    debit_sum  NUMERIC(18,2);
    credit_sum NUMERIC(18,2);
    j_id UUID;
BEGIN
    IF TG_OP = 'INSERT' THEN
        j_id := NEW.journal_id;
    ELSIF TG_OP = 'DELETE' THEN
        j_id := OLD.journal_id;
    ELSE
        RETURN NEW; -- UPDATE should not happen on append-only table, but guard anyway
    END IF;

    SELECT
        COALESCE(SUM(CASE WHEN direction = 'debit'  THEN amount ELSE 0 END), 0),
        COALESCE(SUM(CASE WHEN direction = 'credit' THEN amount ELSE 0 END), 0)
    INTO debit_sum, credit_sum
    FROM ledger.postings
    WHERE journal_id = j_id;

    IF debit_sum <> credit_sum THEN
        RAISE EXCEPTION 'journal % is not balanced: debits=%, credits=%',
            j_id, debit_sum, credit_sum;
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

-- The check is deferred to COMMIT: a journal holds several postings, and they
-- are written one at a time inside a transaction. An ordinary trigger would
-- reject the first posting because the journal is not balanced yet.
DROP TRIGGER IF EXISTS postings_balanced ON ledger.postings;

CREATE CONSTRAINT TRIGGER postings_balanced
    AFTER INSERT OR DELETE ON ledger.postings
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION ledger.ensure_balanced_postings();

-- =============================================================================
-- BALANCE DERIVATION
-- The balance a posting leaves behind is derived by the database, not by the
-- application. The application cannot supply a stale or wrong value, so the
-- balance of an account is always what its postings actually say.
-- =============================================================================

-- Function to compute the balance a posting leaves behind.
--
-- The account row is locked FOR UPDATE, so the computed balance is the one
-- this posting really produces: a second posting to the same account waits for
-- this transaction and then reads the balance the first one left behind.
-- Without the lock both postings would read the same starting balance and
-- disagree about the result.
CREATE OR REPLACE FUNCTION ledger.set_posting_balance_after() RETURNS trigger AS $$
DECLARE
    acc_balance     NUMERIC(18,2);
    acc_normal_side TEXT;
    delta           NUMERIC(18,2);
BEGIN
    -- Determine sign of delta based on normal_side and direction.
    -- Asset/Expense accounts: debit increases, credit decreases.
    -- Liability/Equity/Revenue accounts: credit increases, debit decreases.
    SELECT
        a.balance,
        a.normal_side
    INTO
        acc_balance,
        acc_normal_side
    FROM ledger.accounts a
    WHERE a.id = NEW.account_id
    FOR UPDATE; -- Lock the account row to prevent concurrent modification.

    IF acc_normal_side IS NULL THEN
        RAISE EXCEPTION 'account % does not exist', NEW.account_id;
    END IF;

    delta := CASE
        WHEN acc_normal_side = 'debit'  AND NEW.direction = 'debit'   THEN  NEW.amount
        WHEN acc_normal_side = 'debit'  AND NEW.direction = 'credit'  THEN -NEW.amount
        WHEN acc_normal_side = 'credit' AND NEW.direction = 'credit'  THEN  NEW.amount
        WHEN acc_normal_side = 'credit' AND NEW.direction = 'debit'   THEN -NEW.amount
        ELSE 0
    END;

    NEW.balance_after := acc_balance + delta;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- The trigger name sorts after postings_currency_check on purpose: PostgreSQL
-- runs the triggers of one event in alphabetical order, so a posting with a
-- mismatching currency is rejected before this trigger takes the account lock.
DROP TRIGGER IF EXISTS postings_set_balance_after ON ledger.postings;

CREATE TRIGGER postings_set_balance_after
    BEFORE INSERT ON ledger.postings
    FOR EACH ROW EXECUTE FUNCTION ledger.set_posting_balance_after();


-- =============================================================================
-- BALANCE GUARD
-- accounts.balance may only be modified through posting triggers,
-- not by direct UPDATE. This ensures auditability.
-- =============================================================================

-- Function to apply the balance a posting derived.
-- Called by trigger on ledger.postings AFTER INSERT.
CREATE OR REPLACE FUNCTION ledger.update_account_balance() RETURNS trigger AS $$
BEGIN
    -- balance_after was derived while the lock on this account row was held, so
    -- it is already the balance this posting produces. The lock is still held
    -- here, so nothing can have changed the account in between.
    --
    -- version counts the balance changes of the account. It is a revision
    -- counter, not a compare-and-swap token.
    UPDATE ledger.accounts
    SET
        balance    = NEW.balance_after,
        updated_at = now(),
        version    = version + 1
    WHERE id = NEW.account_id;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS postings_update_balance ON ledger.postings;

CREATE TRIGGER postings_update_balance
    AFTER INSERT ON ledger.postings
    FOR EACH ROW EXECUTE FUNCTION ledger.update_account_balance();

-- Reject direct UPDATE on accounts.balance (only postings may modify it).
-- We allow INSERT (new accounts) and DELETE (cascade from drop), but not UPDATE.
CREATE OR REPLACE FUNCTION ledger.reject_balance_update() RETURNS trigger AS $$
BEGIN
    -- Nested inside another trigger (i.e. ledger.update_account_balance): the
    -- balance was moved by a posting, which is exactly what this guard allows.
    -- A direct UPDATE runs at depth 1 and is rejected below.
    IF pg_trigger_depth() > 1 THEN
        RETURN NEW;
    END IF;

    IF OLD.balance IS DISTINCT FROM NEW.balance THEN
        RAISE EXCEPTION 'accounts.balance may not be updated directly; use postings';
    END IF;
    IF OLD.currency IS DISTINCT FROM NEW.currency THEN
        RAISE EXCEPTION 'accounts.currency may not be changed';
    END IF;
    IF OLD.normal_side IS DISTINCT FROM NEW.normal_side THEN
        RAISE EXCEPTION 'accounts.normal_side may not be changed after creation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS accounts_balance_guard ON ledger.accounts;

CREATE TRIGGER accounts_balance_guard
    BEFORE UPDATE ON ledger.accounts
    FOR EACH ROW EXECUTE FUNCTION ledger.reject_balance_update();

-- =============================================================================
-- CURRENCY CONSISTENCY
-- A posting must use the same currency as its account.
-- =============================================================================

CREATE OR REPLACE FUNCTION ledger.ensure_currency_match() RETURNS trigger AS $$
DECLARE
    acc_currency TEXT;
BEGIN
    SELECT a.currency INTO acc_currency
    FROM ledger.accounts a
    WHERE a.id = NEW.account_id;

    IF acc_currency IS NULL THEN
        RAISE EXCEPTION 'account % does not exist', NEW.account_id;
    END IF;

    IF NEW.currency <> acc_currency THEN
        RAISE EXCEPTION 'posting currency % does not match account currency %',
            NEW.currency, acc_currency;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Fire before insert to catch mismatched currency before posting is recorded.
DROP TRIGGER IF EXISTS postings_currency_check ON ledger.postings;

CREATE TRIGGER postings_currency_check
    BEFORE INSERT ON ledger.postings
    FOR EACH ROW EXECUTE FUNCTION ledger.ensure_currency_match();
