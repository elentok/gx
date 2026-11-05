package rowfmt

import (
	"testing"
	"time"
)

func TestFormatMetricsLine_HadCodexPrintsDashForCost(t *testing.T) {
	t.Parallel()
	if got, want := FormatMetricsLine(5, 100, 0.42, true), FormatMetricsLine(5, 100, 0, false)+" · —"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := FormatMetricsLine(5, 100, 0.42, false); got == FormatMetricsLine(5, 100, 0.42, true) {
		t.Fatalf("Claude row must keep its dollars, got %q", got)
	}
}

func TestFormatTokenCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tokens int
		want   string
	}{
		{823, "823 tok"},
		{999, "999 tok"},
		{1000, "1.0k tok"},
		{45200, "45.2k tok"},
		{999_000, "999.0k tok"},
		{1_000_000, "1.0M tok"},
		{1_200_000, "1.2M tok"},
	}
	for _, c := range cases {
		if got := FormatTokenCount(c.tokens); got != c.want {
			t.Errorf("FormatTokenCount(%d) = %q, want %q", c.tokens, got, c.want)
		}
	}
}

func TestFormatElapsed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		seconds int
		want    string
	}{
		{0, "0s"},
		{45, "45s"},
		{754, "12m34s"}, // 12m34s
		{3599, "59m59s"},
		{3600, "1h00m"},
		{3900, "1h05m"},
	}
	for _, c := range cases {
		if got := FormatElapsed(c.seconds); got != c.want {
			t.Errorf("FormatElapsed(%d) = %q, want %q", c.seconds, got, c.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0m"},
		{5 * time.Minute, "5m"},
		{59 * time.Minute, "59m"},
		{time.Hour, "1h 0m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
		{23*time.Hour + 59*time.Minute, "23h 59m"},
		{24 * time.Hour, "1d 0h"},
		{26*time.Hour + 5*time.Minute, "1d 2h"},
		{3*24*time.Hour + 4*time.Hour, "3d 4h"},
	}
	for _, c := range cases {
		if got := FormatDuration(c.d); got != c.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
