package prflow_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets"
)

// The servertest harness runs this test binary as its session holder and as
// the fake herdr; without both calls the binary re-runs the suite in every
// child and forks without end.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	herdrfake.RunHelperProcess()
	ghfake.Warm()
	code := m.Run()
	servertest.RemoveFakeCodex()
	os.Exit(code)
}

const prURL = "https://github.com/o/r/pull/5"

// openPR is a draft PR 5 whose head has one running check.
func openPR(f *rigs.PRFlow) {
	f.GH.Update(func(s *ghfake.State) {
		s.PRs = append(s.PRs, ghfake.PR{Number: 5, URL: prURL, Head: "epic-a", Base: "main", State: "OPEN", Draft: true, HeadSHA: "sha1", Mergeable: "MERGEABLE"})
		s.Checks = map[string][]ghfake.Check{"sha1": {{Name: "build", Status: "IN_PROGRESS", JobID: 1, RunID: 1, WorkflowID: 1}}}
	})
}

func kinds(t *testing.T, f *rigs.PRFlow) []prflow.Kind {
	t.Helper()
	es, err := prflow.Read(f.EpicDir())
	if err != nil {
		t.Fatal(err)
	}
	var out []prflow.Kind
	for i := len(es) - 1; i >= 0; i-- {
		out = append(out, es[i].Kind)
	}
	return out
}

func TestWatcher_PollsAnEpicWithAPRNumber(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(f)
	f.StampPR(t, 5, prURL)
	f.Tick(t)

	w := f.H.Server.PRWatcher()
	v, ok := w.View("proj", "epic-a")
	if !ok {
		t.Fatal("no view for the epic")
	}
	if v.Number != 5 || v.URL != prURL || v.State != "OPEN" || !v.Draft || v.HeadSHA != "sha1" || v.Mergeable != "MERGEABLE" {
		t.Fatalf("view = %+v", v)
	}
	if v.CI != pr.ChecksPending || v.Checks.Pending != 1 {
		t.Fatalf("CI = %v counts = %+v, want pending 1", v.CI, v.Checks)
	}
	if d := w.Delay(); d != prflow.PendingInterval {
		t.Fatalf("delay = %v, want %v while pending", d, prflow.PendingInterval)
	}

	f.GH.Update(func(s *ghfake.State) {
		s.Checks["sha1"][0].Status, s.Checks["sha1"][0].Conclusion = "COMPLETED", "FAILURE"
	})
	f.Tick(t)
	v, _ = w.View("proj", "epic-a")
	if v.CI != pr.ChecksFailed || v.Checks.Failed != 1 || len(v.Failed) != 1 {
		t.Fatalf("CI = %v counts = %+v failed = %v, want one failed", v.CI, v.Checks, v.Failed)
	}
	if d := w.Delay(); d != prflow.IdleInterval {
		t.Fatalf("delay = %v, want %v with nothing pending", d, prflow.IdleInterval)
	}
}

func TestWatcher_OneWatchQueryPerPoll(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(f)
	f.StampPR(t, 5, prURL)
	f.Tick(t)
	// A second pass with nothing changed asks the cheap query again but not the per-check detail.
	f.Tick(t)
	var watch, detail int
	for _, c := range f.GH.CallsMatching("api graphql") {
		if strings.Contains(strings.Join(c, " "), "contexts(") {
			detail++
		} else {
			watch++
		}
	}
	if watch != 2 || detail != 1 {
		t.Fatalf("watch queries = %d detail calls = %d, want 2 and 1; calls %v", watch, detail, f.GH.Calls())
	}
}

func TestWatcher_RestartKeepsWatchingWithoutAnIdentityLookup(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(f)
	f.StampPR(t, 5, prURL)
	f.H.Restart(t)
	f.Rescan(t)
	f.Tick(t)

	if _, ok := f.H.Server.PRWatcher().View("proj", "epic-a"); !ok {
		t.Fatal("restarted server does not watch the epic")
	}
	for _, prefix := range []string{"pr list", "pr view"} {
		if calls := f.GH.CallsMatching(prefix); len(calls) != 0 {
			t.Fatalf("restart looked the PR up: %v", calls)
		}
	}
}

func TestWatcher_OutageIsLoggedOnceKeepsTheViewAndRecovers(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(f)
	f.StampPR(t, 5, prURL)
	f.Tick(t)
	w := f.H.Server.PRWatcher()

	f.GH.Update(func(s *ghfake.State) {
		s.Failures = []ghfake.Failure{{Match: "api graphql", Code: 1, Stderr: "dial tcp: connection refused"}}
	})
	var delays []time.Duration
	for range 5 {
		f.Tick(t)
		delays = append(delays, w.Delay())
	}
	want := []time.Duration{45 * time.Second, 90 * time.Second, 180 * time.Second, prflow.MaxBackoff, prflow.MaxBackoff}
	for i := range want {
		if delays[i] != want[i] {
			t.Fatalf("delays = %v, want %v", delays, want)
		}
	}
	v, _ := w.View("proj", "epic-a")
	if !v.Unreachable || v.LastError == "" || v.State != "OPEN" || v.HeadSHA != "sha1" {
		t.Fatalf("view during the outage = %+v, want the last known state flagged unreachable", v)
	}
	if got := kinds(t, f); len(got) != 1 || got[0] != prflow.KindGitHubDown {
		t.Fatalf("activity = %v, want one github-unreachable", got)
	}

	f.GH.Update(func(s *ghfake.State) { s.Failures = nil })
	f.Tick(t)
	f.Tick(t)
	v, _ = w.View("proj", "epic-a")
	if v.Unreachable || v.LastError != "" {
		t.Fatalf("view after recovery = %+v", v)
	}
	if got := kinds(t, f); len(got) != 2 || got[1] != prflow.KindGitHubUp {
		t.Fatalf("activity = %v, want github-unreachable then github-reachable", got)
	}
	if d := w.Delay(); d != prflow.PendingInterval {
		t.Fatalf("delay after recovery = %v, want backoff reset to %v", d, prflow.PendingInterval)
	}
}

func TestWatcher_RateLimitHonoursRetryAfter(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(f)
	f.StampPR(t, 5, prURL)
	f.Tick(t)
	f.GH.Update(func(s *ghfake.State) {
		s.Failures = []ghfake.Failure{{Match: "api graphql", Code: 1, Stderr: "HTTP 429: rate limited\nretry-after: 600"}}
	})
	f.Tick(t)
	if d := f.H.Server.PRWatcher().Delay(); d != 10*time.Minute {
		t.Fatalf("delay = %v, want the 10m retry-after", d)
	}
}

func TestWatcher_SkipsMergedEpicsAndEpicsWithoutAPR(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(f)
	f.Tick(t)
	if calls := f.GH.CallsMatching("api graphql"); len(calls) != 0 {
		t.Fatalf("an epic without pr_number was polled: %v", calls)
	}

	f.StampPR(t, 5, prURL)
	if err := prflow.Append(f.EpicDir(), prflow.Entry{Kind: prflow.KindPRMerged}); err != nil {
		t.Fatal(err)
	}
	f.Tick(t)
	if calls := f.GH.CallsMatching("api graphql"); len(calls) != 0 {
		t.Fatalf("an epic with a PR merged entry was polled: %v", calls)
	}

	other := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(other)
	other.StampPR(t, 5, prURL)
	if err := tickets.StampEpicMerged(other.EpicDir(), time.Now()); err != nil {
		t.Fatal(err)
	}
	other.Rescan(t)
	other.Tick(t)
	if calls := other.GH.CallsMatching("api graphql"); len(calls) != 0 {
		t.Fatalf("a merged epic was polled: %v", calls)
	}
}

func TestWatcher_ActErrorIsPerEpicAndNotAnOutage(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	servertest.WriteTicket(t, f.Store, "proj", "epic-b", "01", "root", "")
	epicB := filepath.Join(f.Store, "proj", "epic-b")
	if err := tickets.StampEpicFields(epicB, map[string]any{"pr_number": 6, "pr_url": "https://github.com/o/r/pull/6"}); err != nil {
		t.Fatal(err)
	}
	openPR(f)
	f.GH.Update(func(s *ghfake.State) {
		s.PRs = append(s.PRs, ghfake.PR{Number: 6, URL: "https://github.com/o/r/pull/6", Head: "epic-b", Base: "main", State: "OPEN", Draft: true, HeadSHA: "sha2", Mergeable: "MERGEABLE"})
		s.Checks = map[string][]ghfake.Check{
			"sha1": {{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS", JobID: 1, RunID: 1, WorkflowID: 1}},
			"sha2": {{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS", JobID: 2, RunID: 2, WorkflowID: 2}},
		}
		s.Failures = []ghfake.Failure{{Match: "pr ready 5", Code: 1, Stderr: "boom"}}
	})
	f.StampPR(t, 5, prURL)
	f.Tick(t)

	if calls := f.GH.CallsMatching("pr ready 6"); len(calls) != 1 {
		t.Fatalf("epic B was not marked ready after epic A's action failed: %v", f.GH.Calls())
	}
	w := f.H.Server.PRWatcher()
	for _, epic := range []string{"epic-a", "epic-b"} {
		if v, _ := w.View("proj", epic); v.Unreachable {
			t.Fatalf("%s view = %+v, want reachable after an action error", epic, v)
		}
	}
	if got := kinds(t, f); slices.Contains(got, prflow.KindGitHubDown) {
		t.Fatalf("activity = %v, an action error is not an outage", got)
	}
	if d := w.Delay(); d != prflow.IdleInterval {
		t.Fatalf("delay = %v, want no backoff after an action error", d)
	}
}

func TestWatcher_DelayPollsFastInsideTheNoChecksGraceWindow(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(f)
	setChecks(f, "sha1")
	now := time.Now()
	w := f.H.Server.PRWatcher()
	w.SetClock(func() time.Time { return now })
	f.StampPR(t, 5, prURL)
	f.Tick(t)
	for name, c := range map[string]struct {
		after time.Duration
		want  time.Duration
	}{
		"inside":  {time.Second, prflow.PendingInterval},
		"outside": {10 * time.Second, prflow.IdleInterval},
	} {
		now = now.Add(c.after)
		if d := w.Delay(); d != c.want {
			t.Errorf("%s the window: delay = %v, want %v", name, d, c.want)
		}
		now = now.Add(-c.after)
	}
}
