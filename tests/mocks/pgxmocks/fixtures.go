package pgxmocks

import (
	"reflect"

	"github.com/stretchr/testify/mock"
)

// The generated mocks know the shape of the driver types, but they do not
// know what a result set contains. The helpers below configure a generated
// mock so it hands real values to the caller, the way the driver would.

// RowValues returns a row that scans the given values.
//
// The values are handed to the destinations positionally, the same way the
// driver would hand them.
func RowValues(values []any) *Row {
	row := &Row{}

	row.On("Scan", mock.Anything).
		Run(func(args mock.Arguments) {
			dest := args.Get(0).([]any)
			assignAll(dest, values)
		}).
		Return(nil)

	return row
}

// ErrRow returns a row that fails every scan with the given error.
func ErrRow(err error) *Row {
	row := &Row{}
	row.On("Scan", mock.Anything).Return(err)
	return row
}

// RowsValues returns a result set with the given rows.
//
// Every row is a list of column values, handed to the scan destinations
// positionally.
func RowsValues(rows ...[]any) *Rows {
	result := &Rows{}

	index := 0
	result.On("Next").Return(func() bool {
		if index >= len(rows) {
			return false
		}
		index++
		return true
	})
	result.On("Scan", mock.Anything).
		Run(func(args mock.Arguments) {
			dest := args.Get(0).([]any)
			assignAll(dest, rows[index-1])
		}).
		Return(nil)
	result.On("Err").Return(nil)
	result.On("Close").Return()

	return result
}

// ErrRows returns a result set that fails on every read.
func ErrRows(err error) *Rows {
	result := &Rows{}
	result.On("Next").Return(false)
	result.On("Scan", mock.Anything).Return(err)
	result.On("Err").Return(err)
	result.On("Close").Return()
	return result
}

// assignAll writes the values into the destinations.
//
// A type mismatch is reported instead of being ignored, so a test cannot
// assert on a value that was never set.
func assignAll(dest, values []any) {
	if len(dest) != len(values) {
		return
	}

	for i := range dest {
		assign(dest[i], values[i])
	}
}

// assign writes value into the pointer dest.
//
// The destination types are the ones sqlc generates for the ledger tables. A
// value of another type is left alone, so a mistake in a test shows up as a
// failed assertion instead of a silent zero value.
func assign(dest, value any) {
	dv := reflect.ValueOf(dest)
	if dv.Kind() != reflect.Pointer || dv.IsNil() {
		return
	}

	vv := reflect.ValueOf(value)
	if !vv.Type().AssignableTo(dv.Elem().Type()) {
		return
	}

	dv.Elem().Set(vv)
}
