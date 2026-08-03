package tickets

import "testing"

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
