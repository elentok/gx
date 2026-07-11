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
		{name: "zero", at: time.Time{}, want: ""},
		{name: "now", at: now.Add(-5 * time.Second), want: "now"},
		{name: "future treated as now", at: now.Add(5 * time.Second), want: "now"},
		{name: "minutes", at: now.Add(-45 * time.Minute), want: "45m ago"},
		{name: "hours", at: now.Add(-2 * time.Hour), want: "2h ago"},
		{name: "days", at: now.Add(-3 * 24 * time.Hour), want: "3d ago"},
		{name: "exact weeks", at: now.Add(-14 * 24 * time.Hour), want: "2wk ago"},
		{name: "weeks and days", at: now.Add(-(9*24*time.Hour + 2*time.Hour)), want: "1wk 2d ago"},
		{name: "months", at: now.Add(-60 * 24 * time.Hour), want: "2mo ago"},
		{name: "exact years", at: now.Add(-24 * 30 * 24 * time.Hour), want: "2y ago"},
		{name: "years and months", at: now.Add(-54 * 30 * 24 * time.Hour), want: "4y 6mo ago"},
	}
	for _, tt := range tests {
		if got := RelativeTimeCompact(tt.at); got != tt.want {
			t.Fatalf("%s: got %q want %q", tt.name, got, tt.want)
		}
	}
}

func TestRelativeTimeCompactShortDropsAgoSuffix(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "zero", at: time.Time{}, want: ""},
		{name: "now", at: now.Add(-5 * time.Second), want: "now"},
		{name: "minutes", at: now.Add(-45 * time.Minute), want: "45m"},
		{name: "hours", at: now.Add(-2 * time.Hour), want: "2h"},
		{name: "days", at: now.Add(-3 * 24 * time.Hour), want: "3d"},
		{name: "exact weeks", at: now.Add(-14 * 24 * time.Hour), want: "2wk"},
		{name: "weeks and days", at: now.Add(-(9*24*time.Hour + 2*time.Hour)), want: "1wk 2d"},
		{name: "months", at: now.Add(-60 * 24 * time.Hour), want: "2mo"},
		{name: "exact years", at: now.Add(-24 * 30 * 24 * time.Hour), want: "2y"},
		{name: "years and months", at: now.Add(-54 * 30 * 24 * time.Hour), want: "4y 6mo"},
	}
	for _, tt := range tests {
		if got := RelativeTimeCompactShort(tt.at); got != tt.want {
			t.Fatalf("%s: got %q want %q", tt.name, got, tt.want)
		}
	}
}
