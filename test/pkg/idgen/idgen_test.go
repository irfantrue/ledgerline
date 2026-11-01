package idgentest

import (
	"testing"

	"ledgerline/pkg/idgen"

	"github.com/google/uuid"
)

func TestNew(t *testing.T) {
	got := idgen.New()

	parsed, err := uuid.Parse(got)
	if err != nil {
		t.Fatalf("idgen.New() = %q, which does not parse as a UUID: %v", got, err)
	}
	if parsed.Version() != 7 {
		t.Errorf("version = %d, want 7", parsed.Version())
	}
}

func TestNew_IsUnique(t *testing.T) {
	seen := make(map[string]bool, 100)

	for i := 0; i < 100; i++ {
		id := idgen.New()
		if seen[id] {
			t.Fatalf("idgen.New() returned the duplicate %q", id)
		}
		seen[id] = true
	}
}
