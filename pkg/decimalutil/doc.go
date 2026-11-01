// Package decimalutil provides utility functions for working with decimal.Decimal types,
// particularly for converting values from database drivers that may return different
// numeric types (float64, int64, string, pgtype.Numeric) that need to be unified
// into decimal.Decimal.
package decimalutil
