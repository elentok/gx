package supervise

import "testing"

func TestModelMatches(t *testing.T) {
	cases := []struct {
		name          string
		ran, expected string
		want          bool
	}{
		{"alias in full id", "claude-sonnet-5-5", "sonnet", true},
		{"full id equal", "claude-sonnet-5-5", "claude-sonnet-5-5", true},
		{"other version of the family", "claude-sonnet-4-6", "claude-sonnet-5-5", true},
		{"other family", "claude-opus-5-5", "sonnet", false},
		{"other family, full ids", "claude-opus-5-5", "claude-sonnet-5-5", false},
		{"case is ignored", "Claude-Sonnet-5-5", "SONNET", true},
		{"codex equal", "gpt-5-codex", "gpt-5-codex", true},
		{"codex expected is a prefix", "gpt-5-codex", "gpt-5", true},
		{"codex other model", "gpt-4.1", "gpt-5-codex", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ModelMatches(c.ran, c.expected); got != c.want {
				t.Errorf("ModelMatches(%q, %q) = %v, want %v", c.ran, c.expected, got, c.want)
			}
		})
	}
}
