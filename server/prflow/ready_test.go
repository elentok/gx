package prflow_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil/ghfake"
)

func setChecks(f *rigs.PRFlow, sha string, checks ...ghfake.Check) {
	f.GH.Update(func(s *ghfake.State) { s.Checks = map[string][]ghfake.Check{sha: checks} })
}

func done(name, conclusion string, id int64) ghfake.Check {
	return ghfake.Check{Name: name, Status: "COMPLETED", Conclusion: conclusion, JobID: id, RunID: id, WorkflowID: id}
}

func readyCalls(f *rigs.PRFlow) int { return len(f.GH.CallsMatching("pr ready")) }

func readyEntries(t *testing.T, f *rigs.PRFlow) int {
	n := 0
	for _, k := range kinds(t, f) {
		if k == prflow.KindMarkedReady {
			n++
		}
	}
	return n
}

func TestReady_GreenDraftIsMarkedOnce(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(f)
	setChecks(f, "sha1", done("a", "SUCCESS", 1), done("b", "NEUTRAL", 2), done("c", "SKIPPED", 3))
	f.StampPR(t, 5, prURL)
	f.Tick(t)
	f.Tick(t)
	if n := readyCalls(f); n != 1 {
		t.Fatalf("pr ready calls = %d, want 1; calls %v", n, f.GH.Calls())
	}
	if n := readyEntries(t, f); n != 1 {
		t.Fatalf("marked-ready entries = %d, want 1", n)
	}
}

func TestReady_NotGreenNeverMarks(t *testing.T) {
	cases := map[string]ghfake.Check{
		"pending":         {Name: "a", Status: "IN_PROGRESS", JobID: 1, RunID: 1, WorkflowID: 1},
		"cancelled":       done("a", "CANCELLED", 1),
		"stale":           done("a", "STALE", 1),
		"action required": done("a", "ACTION_REQUIRED", 1),
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
			openPR(f)
			setChecks(f, "sha1", c)
			f.StampPR(t, 5, prURL)
			f.Tick(t)
			if n := readyCalls(f); n != 0 {
				t.Fatalf("pr ready calls = %d, want 0", n)
			}
			if v, _ := f.H.Server.PR.Watcher().View("proj", "epic-a"); !v.Draft || len(v.FailedChecks) != 0 {
				t.Fatalf("view = %+v, want a draft with no failures", v)
			}
		})
	}
}

func TestReady_AlreadyReadyIsLeftAlone(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(f)
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].Draft = false })
	setChecks(f, "sha1", done("a", "SUCCESS", 1))
	f.StampPR(t, 5, prURL)
	f.Tick(t)
	if n := readyCalls(f); n != 0 {
		t.Fatalf("pr ready calls = %d, want 0", n)
	}
}

func TestReady_OlderHeadFailureIsIgnored(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(f)
	f.GH.Update(func(s *ghfake.State) {
		s.Checks = map[string][]ghfake.Check{
			"old":  {done("a", "FAILURE", 1)},
			"sha1": {done("a", "SUCCESS", 2)},
		}
	})
	f.StampPR(t, 5, prURL)
	f.Tick(t)
	if n := readyCalls(f); n != 1 {
		t.Fatalf("pr ready calls = %d, want 1", n)
	}
}

func TestReady_NoChecksWaitsForTheGraceWindow(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	openPR(f)
	setChecks(f, "sha1")
	now := time.Now()
	w := f.H.Server.PR.Watcher()
	w.SetClock(func() time.Time { return now })
	f.StampPR(t, 5, prURL)
	f.Tick(t)
	now = now.Add(2 * time.Second)
	f.Tick(t)
	if n := readyCalls(f); n != 0 {
		t.Fatalf("pr ready calls = %d before the window, want 0", n)
	}
	now = now.Add(2 * time.Second)
	f.Tick(t)
	f.Tick(t)
	if n := readyCalls(f); n != 1 {
		t.Fatalf("pr ready calls = %d after the window, want 1", n)
	}
}
