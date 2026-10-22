package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

func TestRepair_RefusalsCarryTheRepairPackagesCodes(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	h := servertest.StartWithStore(t, store)
	ctx := context.Background()
	const addr = "proj:epic-a/01"

	cases := []struct {
		name   string
		verb   string
		req    server.RepairRequest
		reason string
	}{
		{"merge from a ralph-loop branch", "merge", server.RepairRequest{Address: addr, Branch: "ralph-loop/epic-a"}, repair.ReasonRalphLoopCwd},
		{"merge continue and abort", "merge", server.RepairRequest{Address: addr, Continue: true, Abort: true}, repair.ReasonError},
		{"merge an open ticket", "merge", server.RepairRequest{Address: addr, Cwd: t.TempDir()}, repair.ReasonStatusRefused},
		{"start-over without a reason", "start-over", server.RepairRequest{Address: addr}, repair.ReasonReasonRequired},
		{"unpark an open ticket", "unpark", server.RepairRequest{Address: addr}, repair.ReasonNotPaused},
		{"resume an open ticket", "resume", server.RepairRequest{Address: addr}, repair.ReasonNotPaused},
		{"bad address", "unpark", server.RepairRequest{Address: "nonsense"}, server.ReasonInvalidAddress},
		{"unknown project", "start-over", server.RepairRequest{Address: "nope:epic-a/01", Reason: "x"}, server.ReasonUnknownTicket},
	}
	for _, c := range cases {
		res, err := h.Client.Repair(ctx, c.verb, c.req)
		if err != nil || !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, %v; want refusal %s", c.name, res, err, c.reason)
		}
	}
}

// pausedIteration leaves ticket 01 paused as kind with an iteration branch,
// committed to when commit is set, and returns the ticket's path.
func pausedIteration(t *testing.T, kind string, commit bool) (*servertest.Harness, string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	wt, ticketPath := leaveIteration(t, h, store, repo)
	_ = os.Remove(filepath.Join(h.StateDir, "runs.json"))
	if !commit {
		runGitIn(t, wt, "reset", "-q", "--hard", "HEAD~1")
	}
	pause := "status: needs-answer\npark_kind: " + kind
	testutil.WriteFile(t, filepath.Dir(ticketPath), filepath.Base(ticketPath), strings.Replace(readFile(t, ticketPath), "status: claimed", pause, 1))
	return h, ticketPath
}

func TestRepair_MergeAcceptsAZeroCommitPauseWithCommits(t *testing.T) {
	h, ticketPath := pausedIteration(t, string(events.ZeroCommit), true)

	// A caller outside the repo still merges: the repo comes from the project.
	res, err := h.Client.Repair(context.Background(), "merge", server.RepairRequest{Address: "proj:epic-a/01", Cwd: t.TempDir()})
	if err != nil || res.Refused {
		t.Fatalf("merge = %+v, %v; want merged", res, err)
	}
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status != schema.StatusDone {
		t.Errorf("status=%q err=%v, want done", tk.Status, err)
	}
}

func TestRepair_MergeRefusesOtherPauses(t *testing.T) {
	cases := []struct {
		name   string
		kind   string
		commit bool
		reason string
	}{
		{"self-reported", string(events.SelfReported), true, repair.ReasonStatusRefused},
		{"zero-commit without commits", string(events.ZeroCommit), false, repair.ReasonNotRecoverable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, _ := pausedIteration(t, c.kind, c.commit)
			res, err := h.Client.Repair(context.Background(), "merge", server.RepairRequest{Address: "proj:epic-a/01"})
			if err != nil || !res.Refused || res.Reason != c.reason {
				t.Errorf("merge = %+v, %v; want refusal %s", res, err, c.reason)
			}
		})
	}
}

func TestRepair_MergeWithoutTheEpicWorktreeIsAWrongRepo(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	if err := ralphloop.Claim(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.Repair(context.Background(), "merge", server.RepairRequest{Address: "proj:epic-a/01"})
	if err != nil || !res.Refused || res.Reason != repair.ReasonWrongRepo {
		t.Errorf("merge = %+v, %v; want %s", res, err, repair.ReasonWrongRepo)
	}
}

func TestRepair_RalphLoopGuardIsServerSide(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.Repair(context.Background(), "merge", server.RepairRequest{Address: "proj:epic-a/01", Branch: "ralph-loop/other"})
	if err != nil || !res.Refused || res.Reason != repair.ReasonRalphLoopCwd {
		t.Errorf("merge = %+v, %v; want %s", res, err, repair.ReasonRalphLoopCwd)
	}
}
