package databasemocks

import (
	"strings"

	"github.com/stretchr/testify/mock"
)

// MatchSQL matches a statement that contains the given fragment.
//
// The repositories build their statements in generated code, so a test
// matches on a fragment of the statement instead of on the whole text.
func MatchSQL(fragment string) any {
	return mock.MatchedBy(func(sql string) bool {
		return fragment == "" || strings.Contains(sql, fragment)
	})
}
