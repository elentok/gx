package queue

import (
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
)

// fakeHost is a Host that answers from fields and counts the extra-usage checks.
type fakeHost struct {
	harness     agent.AgentKind
	extraUsage  bool
	extraChecks int
	live        []runs.Run
}

func (f *fakeHost) DefaultHarness(string) agent.AgentKind     { return f.harness }
func (f *fakeHost) RunnerForRun(runs.Run) agentrunner.Runner  { return nil }
func (f *fakeHost) Runs() []runs.Run                          { return f.live }
func (f *fakeHost) CheckWaitingLabel(string) string           { return "" }
func (f *fakeHost) ListSessions() ([]wire.SessionInfo, error) { return nil, nil }
func (f *fakeHost) EnsureDraftEpic(string) error              { return nil }
func (f *fakeHost) StampEpic(RootRef, string, ...string)      {}
func (f *fakeHost) CheckExtraUsage(time.Time) bool {
	f.extraChecks++
	return f.extraUsage
}

func TestQueue_ExtraUsageAsksTheHostOnlyForClaudeItems(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		harness agent.AgentKind
		agents  []string
		checks  int
	}{
		{"explicit claude", agent.AgentCodex, []string{"claude"}, 1},
		{"empty follows a Claude default", agent.AgentClaude, []string{""}, 1},
		{"empty follows a Codex default", agent.AgentCodex, []string{""}, 0},
		{"codex only", agent.AgentClaude, []string{"codex"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host := &fakeHost{harness: tc.harness, extraUsage: true}
			q := &Queue{host: host}
			res, err := q.WithExtraUsage("gx", tc.agents, wire.QueueResult{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if host.extraChecks != tc.checks || res.ExtraUsage != (tc.checks > 0) {
				t.Fatalf("checks = %d, ExtraUsage = %v; want %d checks", host.extraChecks, res.ExtraUsage, tc.checks)
			}
		})
	}
}

func TestQueue_LiveInReadsTheHostsRuns(t *testing.T) {
	t.Parallel()
	host := &fakeHost{live: []runs.Run{{Address: "gx:e/01"}}}
	q := &Queue{host: host}
	if got := q.liveIn(map[string]bool{"gx:e/01": true}); got != "gx:e/01" {
		t.Fatalf("liveIn = %q, want gx:e/01", got)
	}
	if got := q.liveIn(map[string]bool{"gx:e/02": true}); got != "" {
		t.Fatalf("liveIn = %q, want none", got)
	}
}
