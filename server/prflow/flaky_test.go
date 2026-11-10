package prflow_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/tickets"
)

func rerunCalls(f *rigs.PRFlow) [][]string { return f.GH.CallsMatching("run rerun") }

func countKind(t *testing.T, f *rigs.PRFlow, k prflow.Kind) int {
	t.Helper()
	n := 0
	for _, got := range kinds(t, f) {
		if got == k {
			n++
		}
	}
	return n
}

func followUps(f *rigs.PRFlow) []tickets.Ticket {
	return tickets.LoadEpicAt(filepath.Join(f.Store, "proj", "follow-ups")).Tickets
}

func TestFlaky_FailedJobIsRerunOnceBeforeAnyTicket(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha1", done("build", "FAILURE", 11))
	f.GH.Update(func(s *ghfake.State) { s.Reruns = map[string]ghfake.Result{"11": {Status: "IN_PROGRESS"}} })
	f.Tick(t)
	f.Tick(t)
	calls := rerunCalls(f)
	if len(calls) != 1 || !strings.Contains(strings.Join(calls[0], " "), "run rerun --job 11") {
		t.Fatalf("rerun calls = %v, want one for job 11", calls)
	}
	if got := ciFixTickets(f); len(got) != 0 {
		t.Fatalf("ci-fix tickets = %d, want none while the rerun is pending", len(got))
	}
	if n := countKind(t, f, prflow.KindReranCheck); n != 1 {
		t.Fatalf("reran entries = %d, want 1", n)
	}
}

func TestFlaky_RestartBetweenRerunAndResultDoesNotRerunAgain(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha1", done("build", "FAILURE", 11))
	f.GH.Update(func(s *ghfake.State) { s.Reruns = map[string]ghfake.Result{"11": {Status: "IN_PROGRESS"}} })
	f.Tick(t)
	f.H.Restart(t)
	f.Tick(t)
	if n := len(rerunCalls(f)); n != 1 {
		t.Fatalf("rerun calls = %d, want 1 across the restart", n)
	}
}

func TestFlaky_PassingRerunFilesOneFollowUpAndNoTicket(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha1", done("build", "FAILURE", 11))
	f.Tick(t) // reruns; the fake's unscripted rerun passes
	f.Tick(t)
	f.Tick(t)
	if got := ciFixTickets(f); len(got) != 0 {
		t.Fatalf("ci-fix tickets = %d, want none", len(got))
	}
	if n := countKind(t, f, prflow.KindFlaky); n != 1 {
		t.Fatalf("flaky entries = %d, want 1", n)
	}
	got := followUps(f)
	if len(got) != 1 {
		t.Fatalf("follow-ups = %d, want 1", len(got))
	}
	body := ciFixBody(t, got[0])
	for _, want := range []string{"status: draft", "11/build", "Run URL:", "Head SHA: sha1"} {
		if !strings.Contains(body, want) {
			t.Errorf("follow-up lacks %q:\n%s", want, body)
		}
	}
}

func TestFlaky_SecondFlakeOfTheSameCheckAddsNoSecondFollowUp(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha1", done("build", "FAILURE", 11))
	f.Tick(t)
	f.Tick(t)
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].HeadSHA = "sha2" })
	setChecks(f, "sha2", done("build", "FAILURE", 11))
	f.Tick(t)
	f.Tick(t)
	if n := countKind(t, f, prflow.KindFlaky); n != 2 {
		t.Fatalf("flaky entries = %d, want 2", n)
	}
	if got := followUps(f); len(got) != 1 {
		t.Fatalf("follow-ups = %d, want 1", len(got))
	}
}

func TestFlaky_FailingRerunStartsTheFixTicket(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha1", done("build", "FAILURE", 11))
	f.GH.Update(func(s *ghfake.State) {
		s.Reruns = map[string]ghfake.Result{"11": {Status: "COMPLETED", Conclusion: "FAILURE"}}
	})
	f.Tick(t)
	f.Tick(t)
	if got := ciFixTickets(f); len(got) != 1 {
		t.Fatalf("ci-fix tickets = %d, want 1", len(got))
	}
	if n := len(rerunCalls(f)); n != 1 {
		t.Fatalf("rerun calls = %d, want 1", n)
	}
	if got := followUps(f); len(got) != 0 {
		t.Fatalf("follow-ups = %d, want none", len(got))
	}
}

func TestFlaky_NoWorkflowRunIsNeverRerun(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha1", ghfake.Check{Name: "ci/legacy", StatusState: "FAILURE", DetailsURL: "https://ci.example/run/9"})
	f.Tick(t)
	if n := len(rerunCalls(f)); n != 0 {
		t.Fatalf("rerun calls = %d, want none", n)
	}
	if got := ciFixTickets(f); len(got) != 1 {
		t.Fatalf("ci-fix tickets = %d, want 1", len(got))
	}
}

func TestFlaky_RunStillRunningIsNotRerunYet(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failed := done("build", "FAILURE", 11)
	sibling := ghfake.Check{Name: "test", Status: "IN_PROGRESS", JobID: 12, RunID: 11, WorkflowID: 11}
	failingPR(t, f, "sha1", failed, sibling)
	f.Tick(t)
	if n := len(rerunCalls(f)); n != 0 {
		t.Fatalf("rerun calls = %d, want none while the run is in progress", n)
	}
	if got := ciFixTickets(f); len(got) != 0 {
		t.Fatalf("ci-fix tickets = %d, want none", len(got))
	}
	sibling.Status, sibling.Conclusion = "COMPLETED", "SUCCESS"
	setChecks(f, "sha1", failed, sibling)
	f.Tick(t)
	if n := len(rerunCalls(f)); n != 1 {
		t.Fatalf("rerun calls = %d, want 1 once the run completed", n)
	}
}
