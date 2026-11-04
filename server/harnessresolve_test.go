package server

import (
	"testing"

	"github.com/elentok/gx/ralphloop"
	harnesschoice "github.com/elentok/gx/server/harness"
)

func TestResolveHarness(t *testing.T) {
	t.Parallel()
	in := func(frontmatter, queueItem, earlierRun, def string) harnesschoice.Inputs {
		return harnesschoice.Inputs{Frontmatter: frontmatter, QueueItem: queueItem, EarlierRun: earlierRun, Default: def}
	}
	want := func(h ralphloop.AgentKind, s harnesschoice.HarnessSource) harnesschoice.ResolvedHarness {
		return harnesschoice.ResolvedHarness{Harness: h, Source: s}
	}
	cases := []struct {
		name string
		in   harnesschoice.Inputs
		want harnesschoice.ResolvedHarness
	}{
		{"frontmatter beats all", in("codex", "claude", "claude", "claude"), want(ralphloop.AgentCodex, harnesschoice.HarnessFromFrontmatter)},
		{"queue item beats earlier run and default", in("", "codex", "claude", "claude"), want(ralphloop.AgentCodex, harnesschoice.HarnessFromQueueItem)},
		{"earlier run beats default", in("", "", "codex", "claude"), want(ralphloop.AgentCodex, harnesschoice.HarnessFromEarlierRun)},
		{"default last", in("", "", "", "codex"), want(ralphloop.AgentCodex, harnesschoice.HarnessFromDefault)},
		{"empty default is claude", in("", "", "", ""), want(ralphloop.AgentClaude, harnesschoice.HarnessFromDefault)},
		{"unknown frontmatter skipped", in("gemini", "codex", "", ""), want(ralphloop.AgentCodex, harnesschoice.HarnessFromQueueItem)},
		{"unknown queue item skipped", in("", "gemini", "codex", ""), want(ralphloop.AgentCodex, harnesschoice.HarnessFromEarlierRun)},
		{"unknown earlier run skipped", in("", "", "gemini", "codex"), want(ralphloop.AgentCodex, harnesschoice.HarnessFromDefault)},
		{"unknown default is claude", in("", "", "", "gemini"), want(ralphloop.AgentClaude, harnesschoice.HarnessFromDefault)},
	}
	for _, tc := range cases {
		if got := harnesschoice.Resolve(tc.in); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestResolvedHarness_DiffersFromEarlierRun(t *testing.T) {
	t.Parallel()
	r := harnesschoice.ResolvedHarness{Harness: ralphloop.AgentCodex}
	for earlier, want := range map[string]bool{"": false, "codex": false, "claude": true, "gemini": false} {
		if got := r.DiffersFromEarlierRun(earlier); got != want {
			t.Errorf("earlier %q: got %v, want %v", earlier, got, want)
		}
	}
}
