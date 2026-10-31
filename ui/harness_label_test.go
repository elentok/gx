package ui

import "testing"

func TestHarnessLabel(t *testing.T) {
	cases := []struct {
		name                               string
		harness, tier, model, effort, want string
		switched                           bool
	}{
		{"claude", "claude", "worker", "sonnet-4.6", "high", "Claude · Worker · sonnet-4.6 high", false},
		{"codex", "codex", "strongest", "gpt-6.1-sol", "high", "Codex · Strongest · gpt-6.1-sol high", false},
		{"no reply", "claude", "worker", "", "", "Claude · Worker · CLI default", false},
		{"no effort", "claude", "light", "haiku", "", "Claude · Light · haiku", false},
		{"switched", "claude", "worker", "opus", "high", "Claude · Worker · opus high · switched by hand", true},
	}
	for _, c := range cases {
		if got := HarnessLabel(c.harness, c.tier, c.model, c.effort, c.switched); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestQueueHarnessLabel(t *testing.T) {
	cases := []struct{ name, harness, tier, model, effort, note, want string }{
		{"follows default", "", "", "", "", "follows default", "follows default"},
		{"pinned", "codex", "", "", "", "pinned", "Codex · pinned"},
		{"keeps", "claude", "", "", "", "keeps Claude (ran before)", "keeps Claude (ran before)"},
		{"running", "codex", "worker", "gpt-6.1-sol", "medium", "", "Codex · Worker · gpt-6.1-sol medium"},
		{"unlabelled", "", "", "", "", "", ""},
	}
	for _, c := range cases {
		if got := QueueHarnessLabel(c.harness, c.tier, c.model, c.effort, c.note); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
