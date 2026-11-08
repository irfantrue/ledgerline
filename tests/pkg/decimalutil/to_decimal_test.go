package decimalutiltest

import (
	"math/big"
	"testing"

	"ledgerline/pkg/decimalutil"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAmount is a sample decimal value reused across the test cases.
const testAmount = "12.34"

func TestToDecimal(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{name: "nil", input: nil, want: "0"},
		{name: "decimal", input: decimal.RequireFromString(testAmount), want: testAmount},
		{name: "decimal pointer", input: func() *decimal.Decimal { d := decimal.RequireFromString(testAmount); return &d }(), want: testAmount},
		{name: "nil decimal pointer", input: (*decimal.Decimal)(nil), want: "0"},
		{name: "nil pgtype numeric pointer", input: (*pgtype.Numeric)(nil), want: "0"},
		{name: "int64", input: int64(42), want: "42"},
		{name: "int32", input: int32(42), want: "42"},
		{name: "int", input: 42, want: "42"},
		{name: "float64", input: 12.34, want: testAmount},
		{name: "string", input: testAmount, want: testAmount},
		{name: "bytes", input: []byte(testAmount), want: testAmount},
		{
			name:  "pgtype numeric",
			input: pgtype.Numeric{Int: big.NewInt(12345), Exp: -2, Valid: true},
			want:  "123.45",
		},
		{
			name:  "pgtype numeric pointer",
			input: &pgtype.Numeric{Int: big.NewInt(12345), Exp: -2, Valid: true},
			want:  "123.45",
		},
		{
			name:  "pgtype numeric positive exponent",
			input: pgtype.Numeric{Int: big.NewInt(123), Exp: 2, Valid: true},
			want:  "12300",
		},
		{
			name:  "pgtype numeric without int",
			input: pgtype.Numeric{Valid: true},
			want:  "0",
		},
		{
			name:  "pgtype numeric invalid",
			input: pgtype.Numeric{Valid: false},
			want:  "0",
		},
		{
			name:    "pgtype numeric NaN",
			input:   pgtype.Numeric{NaN: true, Valid: true},
			wantErr: true,
		},
		{
			name:    "pgtype numeric infinity",
			input:   pgtype.Numeric{InfinityModifier: pgtype.Infinity, Valid: true},
			wantErr: true,
		},
		{
			name:    "unsupported type",
			input:   struct{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decimalutil.ToDecimal(tt.input)
			if tt.wantErr {
				require.Error(t, err, "decimalutil.ToDecimal(%v) expected error, got nil", tt.input)
				return
			}
			require.NoError(t, err, "decimalutil.ToDecimal(%v) unexpected error: %v", tt.input, err)
			assert.True(t, got.Equal(decimal.RequireFromString(tt.want)),
				"decimalutil.ToDecimal(%v) = %s, want %s", tt.input, got, tt.want)
		})
	}
}
