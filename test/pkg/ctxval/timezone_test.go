package ctxvaltest

import (
	"context"
	"testing"
	"time"

	"ledgerline/pkg/ctxval"
)

func TestGetTimezone_DefaultIsUTC(t *testing.T) {
	got := ctxval.GetTimezone(context.Background())
	if got != time.UTC {
		t.Errorf("GetTimezone() = %s, want UTC", got)
	}
}

func TestSetAndGetTimezone(t *testing.T) {
	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	ctx := ctxval.SetTimezone(context.Background(), jakarta)

	got := ctxval.GetTimezone(ctx)
	if got.String() != "Asia/Jakarta" {
		t.Errorf("GetTimezone() = %s, want Asia/Jakarta", got)
	}
}

// TestSetTimezone_DoesNotLeakToParent makes sure the timezone only lives in
// the returned context.
func TestSetTimezone_DoesNotLeakToParent(t *testing.T) {
	parent := context.Background()

	child := ctxval.SetTimezone(parent, time.UTC)

	if got := ctxval.GetTimezone(child); got != time.UTC {
		t.Errorf("GetTimezone(child) = %s, want UTC", got)
	}
	if got := ctxval.GetTimezone(parent); got != time.UTC {
		t.Errorf("GetTimezone(parent) = %s, want UTC", got)
	}
}
