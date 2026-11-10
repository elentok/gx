package prflow_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// chatRig is a PR flow rig whose chat notices are counted by count.
func chatRig(t *testing.T) (*rigs.PRFlow, func(string) int) {
	t.Helper()
	opt, count := rigs.ChatRecorder(t)
	return rigs.StartPRFlow(t, rigs.PRFlowOpts{Server: []func(*server.Config){opt}}), count
}

// ciFailingNotices is how many "CI failing" notices the chat got.
func ciFailingNotices(count func(string) int) int {
	label := schema.PauseCIFailing.Label()
	rigs.WaitForChat(count, label)
	time.Sleep(2 * servertest.FlushInterval) // negative wait: a second notice would flush by now
	return count(label)
}

func seedFailed(t *testing.T, f *rigs.PRFlow, keys ...string) {
	t.Helper()
	for i, k := range keys {
		if err := prflow.Append(f.EpicDir(), prflow.Entry{Kind: prflow.KindCIFailed, Checks: []string{k}, HeadSHA: fmt.Sprintf("old%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
}

func entriesOf(t *testing.T, f *rigs.PRFlow, kind prflow.Kind) []prflow.Entry {
	t.Helper()
	es, err := prflow.Read(f.EpicDir())
	if err != nil {
		t.Fatal(err)
	}
	var out []prflow.Entry
	for _, e := range es {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func tryAgain(t *testing.T, f *rigs.PRFlow) wire.QueueResult {
	t.Helper()
	res, err := f.H.Client.Tickets.EpicTryAgain(context.Background(), "proj:epic-a")
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func assertCIFailing(t *testing.T, f *rigs.PRFlow, reasonPart string) {
	t.Helper()
	e := tickets.LoadEpicTicketMD(f.EpicDir())
	if e.MergeBlockKind != string(schema.PauseCIFailing) || !strings.Contains(e.MergeBlockReason, reasonPart) {
		t.Fatalf("block = %q %q, want ci-failing with %q", e.MergeBlockKind, e.MergeBlockReason, reasonPart)
	}
	if n := len(entriesOf(t, f, prflow.KindPausedCIFailing)); n != 1 {
		t.Errorf("Paused entries = %d, want 1", n)
	}
	if n := len(ciFixTickets(f)); n != 0 {
		t.Errorf("ci-fix tickets = %d, want none past the cap", n)
	}
}

func TestGiveUp_CapsStoreCIFailingAndNotifyOnce(t *testing.T) {
	cases := map[string]struct {
		seed   []string
		reason string
	}{
		"per check": {seed: []string{"11/build", "11/build", "11/build"}, reason: "CI failing: build failed 3 times (limit 3 per check). Fix it by hand and push, or press Try again to give the agent 3 more attempts."},
		"per PR": {seed: []string{"1/a", "2/b", "3/c", "4/d", "5/e", "6/f", "7/g", "8/h", "9/i", "10/j"},
			reason: "CI failing: CI failed 10 times (limit 10 per pull request)."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f, count := chatRig(t)
			failingPR(t, f, "sha1", done("build", "FAILURE", 11))
			seedFailed(t, f, tc.seed...)
			reran(t, f, "sha1", "11/build")
			f.Tick(t)
			f.Tick(t)
			f.Tick(t)
			assertCIFailing(t, f, tc.reason)
			if n := ciFailingNotices(count); n != 1 {
				t.Errorf("notices = %d, want one when gx gives up and none per attempt", n)
			}
		})
	}
}

func TestGiveUp_UnfixableOutcomePausesAtOnceWithAttemptsLeft(t *testing.T) {
	f, count := chatRig(t)
	failingPR(t, f, "sha1", done("build", "FAILURE", 11))
	reran(t, f, "sha1", "11/build")
	f.Tick(t)
	fix := ciFixTickets(f)
	if len(fix) != 1 {
		t.Fatalf("ci-fix tickets = %d, want 1", len(fix))
	}
	raw := strings.Replace(servertest.ReadFile(t, fix[0].Path), "status: open", "status: done", 1)
	raw += "\n## Resolution\n\nCheck: build\nOutcome: unfixable\n"
	if err := os.WriteFile(fix[0].Path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	f.Rescan(t)
	f.Tick(t)
	f.Tick(t)
	e := tickets.LoadEpicTicketMD(f.EpicDir())
	if e.MergeBlockKind != string(schema.PauseCIFailing) {
		t.Fatalf("block kind = %q, want ci-failing", e.MergeBlockKind)
	}
	if n := len(ciFixTickets(f)); n != 1 {
		t.Errorf("ci-fix tickets = %d, want the one only", n)
	}
	if n := ciFailingNotices(count); n != 1 {
		t.Errorf("notices = %d, want 1", n)
	}
}

func TestTryAgain_ClearsTheBlockResetsCountsAndAStillFailingCheckGetsATicket(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha1", done("build", "FAILURE", 11))
	seedFailed(t, f, "11/build", "11/build", "11/build")
	reran(t, f, "sha1", "11/build")
	f.Tick(t)
	assertCIFailing(t, f, "build failed 3 times")

	if res := tryAgain(t, f); res.Refused {
		t.Fatalf("try again refused: %+v", res)
	}
	if kind := tickets.LoadEpicTicketMD(f.EpicDir()).MergeBlockKind; kind != "" {
		t.Errorf("block kind = %q, want cleared", kind)
	}
	if n := len(entriesOf(t, f, prflow.KindTryAgainReset)); n != 1 {
		t.Fatalf("reset entries = %d, want 1", n)
	}
	reran(t, f, "sha1", "11/build")
	f.Tick(t)
	if n := len(ciFixTickets(f)); n != 1 {
		t.Fatalf("ci-fix tickets = %d, want a fresh one for the still-failing check", n)
	}
}

func TestTryAgain_RefusesAnEpicWithNeitherPause(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	res := tryAgain(t, f)
	if !res.Refused || res.Reason != wire.ReasonNotPaused {
		t.Fatalf("result = %+v, want a not-paused refusal", res)
	}
	if n := len(entriesOf(t, f, prflow.KindTryAgainReset)); n != 0 {
		t.Errorf("reset entries = %d, want none", n)
	}
}

func TestTryAgain_PRTextFailedStartsANewPRTextTicket(t *testing.T) {
	f := openRig(t, "")
	if err := tickets.OverwriteEpicFields(f.EpicDir(), map[string]any{"land_block_kind": "pr-text-failed", "land_block_reason": "no Title:"}); err != nil {
		t.Fatal(err)
	}
	f.Rescan(t)
	if res := tryAgain(t, f); res.Refused {
		t.Fatalf("try again refused: %+v", res)
	}
	if kind := tickets.LoadEpicTicketMD(f.EpicDir()).MergeBlockKind; kind != "" {
		t.Errorf("block kind = %q, want cleared", kind)
	}
	if n := len(prTextTickets(f)); n != 1 {
		t.Errorf("pr-text tickets = %d, want a new one", n)
	}
	if n := len(entriesOf(t, f, prflow.KindTryAgainReset)); n != 1 {
		t.Errorf("reset entries = %d, want 1", n)
	}
}

func TestGiveUp_GreenHeadClearsCIFailingAndStartsAFreshBudget(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	failingPR(t, f, "sha1", done("build", "FAILURE", 11))
	seedFailed(t, f, "11/build", "11/build", "11/build")
	reran(t, f, "sha1", "11/build")
	f.Tick(t)
	assertCIFailing(t, f, "build failed 3 times")

	setChecks(f, "sha1", ghfake.Check{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS", JobID: 11, RunID: 11, WorkflowID: 11})
	f.Tick(t)
	if kind := tickets.LoadEpicTicketMD(f.EpicDir()).MergeBlockKind; kind != "" {
		t.Fatalf("block kind = %q, want cleared by the green head", kind)
	}
	resets := entriesOf(t, f, prflow.KindTryAgainReset)
	if len(resets) != 1 || resets[0].Detail != "CI green" {
		t.Fatalf("reset entries = %+v, want one with detail CI green", resets)
	}
	if n, _ := prflow.CIFailedAttempts(f.EpicDir()); n != 0 {
		t.Errorf("attempts since reset = %d, want 0", n)
	}
}
