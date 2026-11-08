package ctxvaltest

import (
	"context"
	"testing"
	"time"

	"ledgerline/pkg/ctxval"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTimezone_DefaultIsUTC(t *testing.T) {
	got := ctxval.GetTimezone(context.Background())
	assert.Same(t, time.UTC, got, "GetTimezone() = %s, want UTC", got)
}

func TestSetAndGetTimezone(t *testing.T) {
	jakarta, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err, "LoadLocation: %v", err)

	ctx := ctxval.SetTimezone(context.Background(), jakarta)

	got := ctxval.GetTimezone(ctx)
	assert.Equal(t, "Asia/Jakarta", got.String(), "GetTimezone() = %s, want Asia/Jakarta", got)
}

// TestSetTimezone_DoesNotLeakToParent makes sure the timezone only lives in
// the returned context.
func TestSetTimezone_DoesNotLeakToParent(t *testing.T) {
	parent := context.Background()

	child := ctxval.SetTimezone(parent, time.UTC)

	assert.Same(t, time.UTC, ctxval.GetTimezone(child), "GetTimezone(child) = %s, want UTC", ctxval.GetTimezone(child))
	assert.Same(t, time.UTC, ctxval.GetTimezone(parent), "GetTimezone(parent) = %s, want UTC", ctxval.GetTimezone(parent))
}
