package ui

import (
	"testing"
	"time"
)

func TestRelativeTimeCompactUsesCompactFormats(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "hours", at: now.Add(-2 * time.Hour), want: "2h ago"},
		{name: "days", at: now.Add(-3 * 24 * time.Hour), want: "3d ago"},
		{name: "weeks and days", at: now.Add(-(9*24*time.Hour + 2*time.Hour)), want: "1wk 2d ago"},
	}
	for _, tt := range tests {
		if got := RelativeTimeCompact(tt.at); got != tt.want {
			t.Fatalf("%s: got %q want %q", tt.name, got, tt.want)
		}
	}
}
