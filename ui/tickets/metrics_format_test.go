package tickets

import (
	"testing"
	"time"
)

func TestFormatTokenCount(t *testing.T) {
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
		if got := formatTokenCount(c.tokens); got != c.want {
			t.Errorf("formatTokenCount(%d) = %q, want %q", c.tokens, got, c.want)
		}
	}
}

func TestFormatElapsed(t *testing.T) {
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
		if got := formatElapsed(c.seconds); got != c.want {
			t.Errorf("formatElapsed(%d) = %q, want %q", c.seconds, got, c.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
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
		if got := formatDuration(c.d); got != c.want {
			t.Errorf("formatDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
