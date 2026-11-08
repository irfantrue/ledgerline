package idgentest

import (
	"testing"

	"ledgerline/pkg/idgen"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	got := idgen.New()

	parsed, err := uuid.Parse(got)
	require.NoError(t, err, "idgen.New() = %q, which does not parse as a UUID: %v", got, err)
	assert.Equal(t, uuid.Version(7), parsed.Version(), "version = %d, want 7", parsed.Version())
}

func TestNew_IsUnique(t *testing.T) {
	seen := make(map[string]bool, 100)

	for i := 0; i < 100; i++ {
		id := idgen.New()
		require.False(t, seen[id], "idgen.New() returned the duplicate %q", id)
		seen[id] = true
	}
}
