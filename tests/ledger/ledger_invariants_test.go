package ledger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The double-entry invariants are enforced by the database, so an invariant
// that only lives in application code can be bypassed by any other writer.
// These tests pin the rules of the trigger functions down.
//
// They read the migration that defines them, so the check needs no database
// and runs everywhere. The rules themselves are checked against a real
// PostgreSQL by the integration suite.

// TestInvariant_AppendOnlyRejectsMutation checks that a mutation of a
// journal or a posting is rejected with a message that names the table.
func TestInvariant_AppendOnlyRejectsMutation(t *testing.T) {
	sql := migrationSQL(t)

	require.Contains(t, sql, "RAISE EXCEPTION 'ledger.% is append-only', TG_TABLE_NAME")
}

// TestInvariant_BalanceGuardRejectsDirectBalanceUpdate checks that the
// balance of an account cannot be written by hand. Only the posting trigger
// may move it.
func TestInvariant_BalanceGuardRejectsDirectBalanceUpdate(t *testing.T) {
	sql := migrationSQL(t)

	require.Contains(t, sql, "accounts.balance may not be updated directly")
	require.Contains(t, sql, "accounts.currency may not be changed")
	require.Contains(t, sql, "accounts.normal_side may not be changed")
}

// TestInvariant_BalanceIsCheckedPerJournal checks that the balance rule sums
// the debits and the credits of the journal of the posting, so a journal is
// balanced on its own.
func TestInvariant_BalanceIsCheckedPerJournal(t *testing.T) {
	sql := migrationSQL(t)

	require.Contains(t, sql, "j_id := NEW.journal_id")
	require.Contains(t, sql, "SUM(CASE WHEN direction = 'debit'  THEN amount ELSE 0 END)")
	require.Contains(t, sql, "SUM(CASE WHEN direction = 'credit' THEN amount ELSE 0 END)")
	require.Contains(t, sql, "WHERE journal_id = j_id")
	require.Contains(t, sql, "journal % is not balanced: debits=%, credits=%")
}

// TestInvariant_CurrencyMustMatchTheAccount checks that a posting cannot use
// another currency than its account, so the sum of a journal is meaningful.
func TestInvariant_CurrencyMustMatchTheAccount(t *testing.T) {
	sql := migrationSQL(t)

	require.Contains(t, sql, "posting currency % does not match account currency %")
	require.Contains(t, sql, "account % does not exist")
}
