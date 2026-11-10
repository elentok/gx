package prflow_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func ciFixTickets(f *rigs.PRFlow) []tickets.Ticket {
	var out []tickets.Ticket
	for _, tk := range tickets.LoadEpicAt(f.EpicDir()).Tickets {
		if tk.Type == string(schema.TypeCIFix) {
			out = append(out, tk)
		}
	}
	return out
}

func ciFixBody(t *testing.T, tk tickets.Ticket) string {
	t.Helper()
	return servertest.ReadFile(t, tk.Path)
}

// failingPR is PR 5 whose head sha has the given checks, stamped and ready to poll.
func failingPR(t *testing.T, f *rigs.PRFlow, sha string, checks ...ghfake.Check) {
	t.Helper()
	openPR(f)
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].HeadSHA = sha })
	setChecks(f, sha, checks...)
	f.StampPR(t, 5, prURL)
}

// reran logs the rerun each key's failed job already had, so the next poll forks.
func reran(t *testing.T, f *rigs.PRFlow, sha string, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := prflow.Append(f.EpicDir(), prflow.Entry{Kind: prflow.KindReranCheck, Checks: []string{k}, HeadSHA: sha}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCIFix_FailedCheckForksOneTicketWithTheRunAndSkills(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{Server: []func(*server.Config){func(c *server.Config) { c.CIFixSkills = []string{"my-ci-rules"} }}})
	failingPR(t, f, "sha1", done("build", "FAILURE", 11), done("lint", "TIMED_OUT", 12))
	reran(t, f, "sha1", "11/build", "12/lint")
	f.Tick(t)
	f.Tick(t)
	got := ciFixTickets(f)
	if len(got) != 1 {
		t.Fatalf("ci-fix tickets = %d, want exactly 1 while it is open", len(got))
	}
	body := ciFixBody(t, got[0])
	for _, want := range []string{"type: ci-fix", "proj:epic-a", "#5", prURL, "Check: build", "Workflow run id: 11", "Job id: 11", "Head SHA: sha1", "Attempt 1 of 3", "my-ci-rules"} {
		if !strings.Contains(body, want) {
			t.Errorf("ticket lacks %q:\n%s", want, body)
		}
	}
	es, _ := prflow.Read(f.EpicDir())
	es = slices.DeleteFunc(es, func(e prflow.Entry) bool { return e.Kind == prflow.KindReranCheck })
	if len(es) != 1 || es[0].Kind != prflow.KindCIFailed || es[0].TicketID != got[0].Identifier || len(es[0].Checks) != 1 {
		t.Fatalf("activity = %+v, want one CI failed entry naming the ticket", es)
	}
}

func TestCIFix_SecondFailedRunGetsATicketWithTheFirstResolution(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha1", done("build", "FAILURE", 11))
	reran(t, f, "sha1", "11/build")
	f.Tick(t)
	first := ciFixTickets(f)[0]
	raw := servertest.ReadFile(t, first.Path)
	raw = strings.Replace(raw, "status: open", "status: done", 1)
	raw += "\n## Resolution\n\nCheck: build\nCause: the cache key\nChange: bumped the key\nOutcome: fixed\n"
	if err := os.WriteFile(first.Path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := prflow.OutcomeOf(first.Path); err != nil || out != prflow.OutcomeFixed {
		t.Fatalf("outcome = %q, %v", out, err)
	}
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].HeadSHA = "sha2" })
	// The same workflow, so the same check key, on a new run.
	rerun := done("build", "FAILURE", 21)
	rerun.WorkflowID = 11
	setChecks(f, "sha2", rerun)
	reran(t, f, "sha2", "11/build")
	f.Tick(t)
	got := ciFixTickets(f)
	if len(got) != 2 {
		t.Fatalf("ci-fix tickets = %d, want 2", len(got))
	}
	body := ciFixBody(t, got[1])
	for _, want := range []string{"Workflow run id: 21", "Head SHA: sha2", "Attempt 2 of 3", "Cause: the cache key", "Change: bumped the key"} {
		if !strings.Contains(body, want) {
			t.Errorf("second ticket lacks %q:\n%s", want, body)
		}
	}
}

func TestCIFix_StatusContextCarriesNameAndURLNoIDs(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha1", ghfake.Check{Name: "ci/legacy", StatusState: "FAILURE", DetailsURL: "https://ci.example/run/9"})
	f.Tick(t)
	got := ciFixTickets(f)
	if len(got) != 1 {
		t.Fatalf("ci-fix tickets = %d, want 1", len(got))
	}
	body := ciFixBody(t, got[0])
	for _, want := range []string{"Check: ci/legacy", "https://ci.example/run/9", "unfixable"} {
		if !strings.Contains(body, want) {
			t.Errorf("ticket lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Workflow run id") || strings.Contains(body, "Job id") {
		t.Errorf("ticket has ids for a check with no workflow run:\n%s", body)
	}
}

func TestCIFix_NotFailedStartsNothing(t *testing.T) {
	cases := map[string][]ghfake.Check{
		"pending":         {{Name: "a", Status: "IN_PROGRESS", JobID: 1, RunID: 1, WorkflowID: 1}},
		"cancelled":       {done("a", "CANCELLED", 1)},
		"stale":           {done("a", "STALE", 1)},
		"action required": {done("a", "ACTION_REQUIRED", 1)},
	}
	for name, checks := range cases {
		t.Run(name, func(t *testing.T) {
			f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
			failingPR(t, f, "sha1", checks...)
			f.Tick(t)
			if got := ciFixTickets(f); len(got) != 0 {
				t.Fatalf("ci-fix tickets = %d, want none", len(got))
			}
			if es, _ := prflow.Read(f.EpicDir()); len(es) != 0 {
				t.Fatalf("activity = %+v, want none", es)
			}
		})
	}
}

func TestCIFix_FailureOnAnOlderHeadStartsNothing(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha2")
	f.GH.Update(func(s *ghfake.State) {
		s.Checks = map[string][]ghfake.Check{"sha1": {done("build", "FAILURE", 11)}, "sha2": {{Name: "build", Status: "IN_PROGRESS", JobID: 2, RunID: 2, WorkflowID: 1}}}
	})
	f.Tick(t)
	if got := ciFixTickets(f); len(got) != 0 {
		t.Fatalf("ci-fix tickets = %d, want none", len(got))
	}
}

func TestCIFix_ProjectOverrideReplacesTheSkillList(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{Server: []func(*server.Config){func(c *server.Config) { c.CIFixSkills = []string{"global-skill"} }}})
	if err := servertest.WriteProjectRepo(f.Store, "proj", f.Origin.Local, `,"delivery":"pr","skills":{"ci-fix":["project-skill"]}`); err != nil {
		t.Fatal(err)
	}
	failingPR(t, f, "sha1", done("build", "FAILURE", 11))
	reran(t, f, "sha1", "11/build")
	f.Tick(t)
	got := ciFixTickets(f)
	if len(got) != 1 {
		t.Fatalf("ci-fix tickets = %d, want 1", len(got))
	}
	body := ciFixBody(t, got[0])
	if !strings.Contains(body, "project-skill") || strings.Contains(body, "global-skill") {
		t.Fatalf("skills not replaced by the project's list:\n%s", body)
	}
}

func TestForkCIFix_CapsComeFromTheActivityLog(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	job := prflow.CIFixJob{Project: "proj", Epic: "epic-a", Number: 5, URL: prURL, HeadSHA: "sha1", PerCheck: 2, PerPR: 3}
	fork := func(name string) prflow.CIFixResult {
		t.Helper()
		job.Check = pr.Check{Name: name, RunID: "1", JobID: "1"}
		id, res, err := prflow.ForkCIFix(f.EpicDir(), job)
		if err != nil {
			t.Fatal(err)
		}
		if res == prflow.CIFixStarted {
			// Close it, so the next fork is not blocked by the one at a time rule.
			tk := ciFixTickets(f)
			last := tk[len(tk)-1]
			if last.Identifier != id {
				t.Fatalf("newest ticket %s, want %s", last.Identifier, id)
			}
			raw := strings.Replace(servertest.ReadFile(t, last.Path), "status: open", "status: done", 1)
			if err := os.WriteFile(last.Path, []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return res
	}
	if r := fork("a"); r != prflow.CIFixStarted {
		t.Fatalf("a#1 = %v", r)
	}
	if r := fork("a"); r != prflow.CIFixStarted {
		t.Fatalf("a#2 = %v", r)
	}
	if r := fork("a"); r != prflow.CIFixCapReached {
		t.Fatalf("a#3 = %v, want per-check cap", r)
	}
	if r := fork("b"); r != prflow.CIFixStarted {
		t.Fatalf("b#1 = %v", r)
	}
	if r := fork("c"); r != prflow.CIFixCapReached {
		t.Fatalf("c#1 = %v, want per-PR cap", r)
	}
	if n := len(ciFixTickets(f)); n != 3 {
		t.Fatalf("ci-fix tickets = %d, want 3 (the cap creates none)", n)
	}
}

func TestCIFix_FailedActivityAppendStillQueuesTheForkedTicket(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha1", done("build", "FAILURE", 11))
	reran(t, f, "sha1", "11/build")
	// Reads still work, appends do not.
	if err := os.Chmod(filepath.Join(f.EpicDir(), prflow.ActivityFile), 0o444); err != nil {
		t.Fatal(err)
	}
	f.Tick(t)
	got := ciFixTickets(f)
	if len(got) != 1 {
		t.Fatalf("ci-fix tickets = %d, want 1", len(got))
	}
	q, err := f.H.Client.Queue.Items(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 1 || !strings.HasSuffix(q[0].Address, "/"+got[0].Identifier) {
		t.Fatalf("queue = %+v, want the ci-fix ticket %s queued", q, got[0].Identifier)
	}
}
