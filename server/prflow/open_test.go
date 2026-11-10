package prflow_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/tickets"
)

var epicRoot = queue.RootRef{Project: "proj", Epic: "epic-a"}

// openRig is a rig whose epic is complete, has a branch, and has a done pr-text
// ticket carrying resolution ("" for none).
func openRig(t *testing.T, resolution string) *rigs.PRFlow {
	t.Helper()
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "01", "root", servertest.TicketOpts{Status: "done"})
	servertest.CommitOnBranch(t, f.Origin.Local, "epic-a")
	if resolution != "" {
		id, _, err := prflow.StartPRText(f.EpicDir(), prflow.TextJob{Epic: "proj:epic-a"})
		if err != nil {
			t.Fatal(err)
		}
		path := ticketFile(t, f, id)
		raw := strings.Replace(servertest.ReadFile(t, path), "status: open", "status: done", 1)
		raw = strings.TrimRight(raw, "\n") + "\n\n## Resolution\n\n" + resolution + "\n"
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func open(t *testing.T, f *rigs.PRFlow) prflow.Outcome {
	t.Helper()
	out, err := f.H.Server.OpenPR(epicRoot)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func creates(f *rigs.PRFlow) int { return len(f.GH.CallsMatching("pr create")) }

func untouched(t *testing.T, f *rigs.PRFlow) {
	t.Helper()
	e := tickets.LoadEpicAt(f.EpicDir())
	if e.PRNumber != 0 || e.PRURL != "" || e.MergeBlockKind != "" {
		t.Errorf("epic changed: pr %d %q, block %q", e.PRNumber, e.PRURL, e.MergeBlockKind)
	}
	if n := len(f.Origin.Pushes()); n != 0 {
		t.Errorf("origin log has %d pushes: %v", n, f.Origin.Pushes())
	}
	if n := creates(f); n != 0 {
		t.Errorf("pr create calls = %d", n)
	}
}

const goodText = "Title: Ship it\nBody:\nDid the thing."

func TestOpen_GhCheckFailuresChangeNothingAndAreRetried(t *testing.T) {
	cases := map[string]func(*ghfake.State){
		"not logged in":  func(s *ghfake.State) { s.LoggedOut = true },
		"no repo access": func(s *ghfake.State) { s.NoRepoAccess = true },
	}
	for name, fail := range cases {
		t.Run(name, func(t *testing.T) {
			f := openRig(t, goodText)
			f.GH.Update(fail)
			if out := open(t, f); out != prflow.OutcomeBlocked {
				t.Fatalf("outcome = %s", out)
			}
			untouched(t, f)
			if f.H.Server.PROpenBlock(epicRoot) == "" {
				t.Error("no reason stored")
			}
			f.GH.Update(func(s *ghfake.State) { s.LoggedOut, s.NoRepoAccess = false, false })
			if out := open(t, f); out != prflow.OutcomeOpened {
				t.Fatalf("retry outcome = %s", out)
			}
			if r := f.H.Server.PROpenBlock(epicRoot); r != "" {
				t.Errorf("reason %q not cleared by the success", r)
			}
		})
	}
}

func TestOpen_NoOriginIsAPreflightFailure(t *testing.T) {
	f := openRig(t, goodText)
	servertest.RunGitIn(t, f.Origin.Local, "remote", "remove", "origin")
	if out := open(t, f); out != prflow.OutcomeBlocked {
		t.Fatalf("outcome = %s", out)
	}
	if r := f.H.Server.PROpenBlock(epicRoot); !strings.Contains(r, "origin") {
		t.Errorf("reason = %q, want it to name origin", r)
	}
	untouched(t, f)
}

func TestOpen_GateSkipsWithoutTouchingGh(t *testing.T) {
	t.Run("open verify ticket", func(t *testing.T) {
		f := openRig(t, goodText)
		servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "02", "verify", servertest.TicketOpts{Type: "verify"})
		if out := open(t, f); out != prflow.OutcomeSkipped {
			t.Fatalf("outcome = %s", out)
		}
		if n := len(f.GH.Calls()); n != 0 {
			t.Errorf("gh calls = %v", f.GH.Calls())
		}
		untouched(t, f)
	})
	t.Run("epic already has a PR", func(t *testing.T) {
		f := openRig(t, goodText)
		f.StampPR(t, 9, prURL)
		if out := open(t, f); out != prflow.OutcomeSkipped {
			t.Fatalf("outcome = %s", out)
		}
		if creates(f) != 0 {
			t.Error("created a second PR")
		}
	})
}

func TestOpen_AdoptsAnOpenPR(t *testing.T) {
	f := openRig(t, goodText)
	openPR(f)
	if out := open(t, f); out != prflow.OutcomeAdopted {
		t.Fatalf("outcome = %s", out)
	}
	if e := tickets.LoadEpicAt(f.EpicDir()); e.PRNumber != 5 || e.PRURL != prURL {
		t.Errorf("stamped %d %q", e.PRNumber, e.PRURL)
	}
	if creates(f) != 0 || len(f.Origin.Pushes()) != 0 {
		t.Errorf("adopting created or pushed: %v", f.GH.Calls())
	}
	if ks := kinds(t, f); !slices.Contains(ks, prflow.KindPRAdopted) {
		t.Errorf("activity %v lacks the adoption", ks)
	}
}

func TestOpen_MergedPRHandsOver(t *testing.T) {
	f := openRig(t, goodText)
	openPR(f)
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].State = "MERGED" })
	var got []pr.HeadPR
	f.H.Server.SetPRMergedHook(func(r queue.RootRef, p pr.HeadPR) {
		if r != epicRoot {
			t.Errorf("root = %v", r)
		}
		got = append(got, p)
	})
	if out := open(t, f); out != prflow.OutcomeMerged || len(got) != 1 || got[0].Number != 5 {
		t.Fatalf("outcome %s, hook calls %v", out, got)
	}
	untouched(t, f)
}

func TestOpen_ClosedPRIsIgnored(t *testing.T) {
	f := openRig(t, goodText)
	openPR(f)
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].State = "CLOSED" })
	if out := open(t, f); out != prflow.OutcomeOpened || creates(f) != 1 {
		t.Fatalf("outcome %s, creates %d", out, creates(f))
	}
	if e := tickets.LoadEpicAt(f.EpicDir()); e.PRNumber != 6 {
		t.Errorf("pr_number = %d, want the new PR 6", e.PRNumber)
	}
}

func TestOpen_ValidTextOpensADraftWithTheExactText(t *testing.T) {
	f := openRig(t, goodText)
	if out := open(t, f); out != prflow.OutcomeOpened {
		t.Fatalf("outcome = %s", out)
	}
	argv := strings.Join(f.GH.CallsMatching("pr create")[0], " ")
	for _, want := range []string{"--draft", "--head epic-a", "--base main", "--title Ship it", "-R " + ghfake.Repo} {
		if !strings.Contains(argv, want) {
			t.Errorf("argv %q lacks %q", argv, want)
		}
	}
	c := f.GH.State().Created
	if len(c) != 1 || c[0].Title != "Ship it" || c[0].Body != "Did the thing." || c[0].Head != "epic-a" || c[0].Base != "main" {
		t.Errorf("created = %+v", c)
	}
	if p := f.Origin.Pushes(); len(p) != 1 || !strings.HasSuffix(p[0], "refs/heads/epic-a") {
		t.Errorf("pushes = %v", p)
	}
	if e := tickets.LoadEpicAt(f.EpicDir()); e.PRNumber != 1 || !strings.HasSuffix(e.PRURL, "/pull/1") {
		t.Errorf("stamped %d %q", e.PRNumber, e.PRURL)
	}
	if ks := kinds(t, f); !slices.Contains(ks, prflow.KindPROpened) {
		t.Errorf("activity %v lacks PR opened", ks)
	}
}

func TestOpen_NoUsableTextStartsOnePRTextTicketAndOpensNothing(t *testing.T) {
	for name, res := range map[string]string{"none": "", "empty body": "Title: T\nBody:\n"} {
		t.Run(name, func(t *testing.T) {
			f := openRig(t, res)
			for range 2 {
				if out := open(t, f); out != prflow.OutcomeWriting {
					t.Fatalf("outcome = %s", out)
				}
			}
			untouched(t, f)
			running := 0
			for _, tk := range prTextTickets(f) {
				if tk.Status == "open" {
					running++
				}
			}
			if running != 1 {
				t.Errorf("open pr-text tickets = %d, want 1", running)
			}
		})
	}
}

func TestOpen_FailedPushShowsAReasonKeepsTheTextAndRetries(t *testing.T) {
	f := openRig(t, goodText)
	hook := filepath.Join(f.Origin.Bare, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho no >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out := open(t, f); out != prflow.OutcomeBlocked {
		t.Fatalf("outcome = %s", out)
	}
	if r := f.H.Server.PROpenBlock(epicRoot); !strings.Contains(r, "push failed") {
		t.Errorf("reason = %q", r)
	}
	if e := tickets.LoadEpicAt(f.EpicDir()); e.PRNumber != 0 || e.MergeBlockKind != "" || creates(f) != 0 {
		t.Errorf("failed push changed the epic or created a PR")
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if out := open(t, f); out != prflow.OutcomeOpened {
		t.Fatalf("retry outcome = %s", out)
	}
	if len(prTextTickets(f)) != 1 {
		t.Errorf("pr-text tickets = %d, want the existing one reused", len(prTextTickets(f)))
	}
}

// The server dying between GitHub creating the PR and the stamp leaves exactly
// this state: a PR on the branch and an epic without pr_number.
func TestOpen_RestartAfterCreateAdoptsAndNeverDuplicates(t *testing.T) {
	f := openRig(t, goodText)
	body := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(body, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := pr.Runner{Env: f.Env, Dir: f.Origin.Local}
	if _, _, err := r.CreateDraft(ghfake.Repo, "epic-a", "main", "Ship it", body); err != nil {
		t.Fatal(err)
	}
	if out := open(t, f); out != prflow.OutcomeAdopted {
		t.Fatalf("outcome = %s", out)
	}
	if creates(f) != 1 {
		t.Errorf("pr create calls = %d, want exactly 1", creates(f))
	}
	if e := tickets.LoadEpicAt(f.EpicDir()); e.PRNumber != 1 {
		t.Errorf("pr_number = %d", e.PRNumber)
	}
}
