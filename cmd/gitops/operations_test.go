package gitops

import (
	"testing"
	"time"
)

func TestRelativeDate_Zero(t *testing.T) {
	t.Parallel()
	got := relativeDate(time.Time{})
	if got != "unknown time" {
		t.Fatalf("relativeDate(zero) = %q, want %q", got, "unknown time")
	}
}

func TestRelativeDate_NonZero(t *testing.T) {
	t.Parallel()
	got := relativeDate(time.Now().Add(-time.Hour))
	if got == "" || got == "unknown time" {
		t.Fatalf("relativeDate(now-1h) = %q, expected a non-empty relative string", got)
	}
}
