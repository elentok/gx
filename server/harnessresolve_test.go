package server

import (
	"testing"

	"github.com/elentok/gx/ralphloop"
)

func TestResolveHarness(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   harnessInputs
		want ResolvedHarness
	}{
		{"frontmatter beats all", harnessInputs{"codex", "claude", "claude", "claude"}, ResolvedHarness{ralphloop.AgentCodex, HarnessFromFrontmatter}},
		{"queue item beats earlier run and default", harnessInputs{"", "codex", "claude", "claude"}, ResolvedHarness{ralphloop.AgentCodex, HarnessFromQueueItem}},
		{"earlier run beats default", harnessInputs{"", "", "codex", "claude"}, ResolvedHarness{ralphloop.AgentCodex, HarnessFromEarlierRun}},
		{"default last", harnessInputs{"", "", "", "codex"}, ResolvedHarness{ralphloop.AgentCodex, HarnessFromDefault}},
		{"empty default is claude", harnessInputs{}, ResolvedHarness{ralphloop.AgentClaude, HarnessFromDefault}},
		{"unknown frontmatter skipped", harnessInputs{"gemini", "codex", "", ""}, ResolvedHarness{ralphloop.AgentCodex, HarnessFromQueueItem}},
		{"unknown queue item skipped", harnessInputs{"", "gemini", "codex", ""}, ResolvedHarness{ralphloop.AgentCodex, HarnessFromEarlierRun}},
		{"unknown earlier run skipped", harnessInputs{"", "", "gemini", "codex"}, ResolvedHarness{ralphloop.AgentCodex, HarnessFromDefault}},
		{"unknown default is claude", harnessInputs{Default: "gemini"}, ResolvedHarness{ralphloop.AgentClaude, HarnessFromDefault}},
	}
	for _, tc := range cases {
		if got := resolveHarness(tc.in); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestResolvedHarness_DiffersFromEarlierRun(t *testing.T) {
	t.Parallel()
	r := ResolvedHarness{Harness: ralphloop.AgentCodex}
	for earlier, want := range map[string]bool{"": false, "codex": false, "claude": true, "gemini": false} {
		if got := r.DiffersFromEarlierRun(earlier); got != want {
			t.Errorf("earlier %q: got %v, want %v", earlier, got, want)
		}
	}
}
