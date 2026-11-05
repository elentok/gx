package harness

import (
	"testing"

	"github.com/elentok/gx/ralphloop/agent"
)

func TestResolveHarness(t *testing.T) {
	t.Parallel()
	in := func(frontmatter, queueItem, earlierRun, def string) Inputs {
		return Inputs{Frontmatter: frontmatter, QueueItem: queueItem, EarlierRun: earlierRun, Default: def}
	}
	want := func(h agent.AgentKind, s HarnessSource) ResolvedHarness {
		return ResolvedHarness{Harness: h, Source: s}
	}
	cases := []struct {
		name string
		in   Inputs
		want ResolvedHarness
	}{
		{"frontmatter beats all", in("codex", "claude", "claude", "claude"), want(agent.AgentCodex, HarnessFromFrontmatter)},
		{"queue item beats earlier run and default", in("", "codex", "claude", "claude"), want(agent.AgentCodex, HarnessFromQueueItem)},
		{"earlier run beats default", in("", "", "codex", "claude"), want(agent.AgentCodex, HarnessFromEarlierRun)},
		{"default last", in("", "", "", "codex"), want(agent.AgentCodex, HarnessFromDefault)},
		{"empty default is claude", in("", "", "", ""), want(agent.AgentClaude, HarnessFromDefault)},
		{"unknown frontmatter skipped", in("gemini", "codex", "", ""), want(agent.AgentCodex, HarnessFromQueueItem)},
		{"unknown queue item skipped", in("", "gemini", "codex", ""), want(agent.AgentCodex, HarnessFromEarlierRun)},
		{"unknown earlier run skipped", in("", "", "gemini", "codex"), want(agent.AgentCodex, HarnessFromDefault)},
		{"unknown default is claude", in("", "", "", "gemini"), want(agent.AgentClaude, HarnessFromDefault)},
	}
	for _, tc := range cases {
		if got := Resolve(tc.in); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestResolvedHarness_DiffersFromEarlierRun(t *testing.T) {
	t.Parallel()
	r := ResolvedHarness{Harness: agent.AgentCodex}
	for earlier, want := range map[string]bool{"": false, "codex": false, "claude": true, "gemini": false} {
		if got := r.DiffersFromEarlierRun(earlier); got != want {
			t.Errorf("earlier %q: got %v, want %v", earlier, got, want)
		}
	}
}
