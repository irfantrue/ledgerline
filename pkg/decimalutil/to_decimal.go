package decimalutil

import (
	"fmt"
	"math/big"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// ToDecimal converts a value returned by the database driver into a decimal.Decimal.
//
// PostgreSQL's NUMERIC column is scanned differently depending on the target type:
//   - into decimal.Decimal (implements sql.Scanner) -> decoded from the text representation
//   - into any (no type information)                -> decoded into pgtype.Numeric
//
// This helper covers both cases, plus the plain Go numeric types, so callers do not
// have to care which scan path produced the value.
func ToDecimal(v any) (decimal.Decimal, error) {
	switch val := v.(type) {
	case nil:
		return decimal.Zero, nil
	case decimal.Decimal:
		return val, nil
	case *decimal.Decimal:
		if val == nil {
			return decimal.Zero, nil
		}
		return *val, nil
	case pgtype.Numeric:
		return fromPgNumeric(val)
	case *pgtype.Numeric:
		if val == nil {
			return decimal.Zero, nil
		}
		return fromPgNumeric(*val)
	case int64:
		return decimal.NewFromInt(val), nil
	case int32:
		return decimal.NewFromInt(int64(val)), nil
	case int:
		return decimal.NewFromInt(int64(val)), nil
	case float64:
		return decimal.NewFromFloat(val), nil
	case string:
		return decimal.NewFromString(val)
	case []byte:
		return decimal.NewFromString(string(val))
	default:
		return decimal.Zero, fmt.Errorf("decimalutil: cannot convert %T to decimal.Decimal", v)
	}
}

// fromPgNumeric converts a pgtype.Numeric into a decimal.Decimal.
// pgtype.Numeric stores the value as an unscaled big.Int plus a base-10 exponent.
func fromPgNumeric(n pgtype.Numeric) (decimal.Decimal, error) {
	switch {
	case !n.Valid:
		return decimal.Zero, nil
	case n.NaN:
		return decimal.Zero, fmt.Errorf("decimalutil: cannot convert NaN to decimal.Decimal")
	case n.InfinityModifier != pgtype.Finite:
		return decimal.Zero, fmt.Errorf("decimalutil: cannot convert infinity to decimal.Decimal")
	}

	value := new(big.Int)
	if n.Int != nil {
		value.Set(n.Int)
	}

	// Shift the unscaled value into place using the base-10 exponent.
	// A negative exponent means the value is scaled down by a power of ten.
	// The exponent is kept as int32 because decimal.NewFromBigInt expects it;
	// safecast guards the conversion so an out-of-range value cannot silently wrap.
	exp, err := safecast.Convert[int32](n.Exp)
	if err != nil {
		return decimal.Zero, fmt.Errorf("decimalutil: numeric exponent out of range: %w", err)
	}

	if exp >= 0 {
		exp10 := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)
		value.Mul(value, exp10)
		return decimal.NewFromBigInt(value, 0), nil
	}

	return decimal.NewFromBigInt(value, exp), nil
}
