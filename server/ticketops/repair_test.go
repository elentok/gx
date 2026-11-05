package ticketops_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestWebStartOver_EmptyReasonRefusalNamesNoCLIFlag(t *testing.T) {
	t.Parallel()
	h, _ := pausedIteration(t, string(events.ZeroCommit), true)
	res, err := h.Server.TicketOps().StartOverAsWeb(wire.RepairRequest{Address: "proj:epic-a/01", State: "tok"})
	if err != nil || !res.Refused || res.Reason != repair.ReasonReasonRequired {
		t.Fatalf("start-over = %+v, %v; want %s refusal", res, err, repair.ReasonReasonRequired)
	}
	if strings.Contains(res.Message, "--reason") {
		t.Errorf("message = %q, must not mention a CLI flag", res.Message)
	}
}

func TestRepair_MergeEndsThePauseRoundAsAManualMerge(t *testing.T) {
	t.Parallel()
	h, ticketPath := pausedIteration(t, string(events.ZeroCommit), true)
	if err := h.Server.PauseAs("proj:epic-a/01", events.ZeroCommit, "no commits"); err != nil {
		t.Fatal(err)
	}

	res, err := h.Client.Repair(context.Background(), "merge", wire.RepairRequest{Address: "proj:epic-a/01", Cwd: t.TempDir()})
	if err != nil || res.Refused {
		t.Fatalf("merge = %+v, %v; want merged", res, err)
	}
	rounds := endedRounds(t, ticketPath)
	if len(rounds) != 1 || rounds[0].End == nil || rounds[0].End.How != tickets.HowManualMerge || rounds[0].End.Source != wire.OriginCLI {
		t.Errorf("rounds = %+v; want one ended by cli manual-merge", rounds)
	}
}

func TestRepair_MergeStampsTheFrictionSummary(t *testing.T) {
	t.Parallel()
	h, ticketPath := pausedIteration(t, string(events.ZeroCommit), true)
	if err := h.Server.PauseAs("proj:epic-a/01", events.ZeroCommit, "no commits"); err != nil {
		t.Fatal(err)
	}

	res, err := h.Client.Repair(context.Background(), "merge", wire.RepairRequest{Address: "proj:epic-a/01", Cwd: t.TempDir()})
	if err != nil || res.Refused {
		t.Fatalf("merge = %+v, %v; want merged", res, err)
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Friction == nil || tk.Friction.PausesByReason["zero-commit"] != 1 || tk.Friction.PauseEndings["cli"] != 1 {
		t.Errorf("friction = %+v, want one zero-commit pause ended by cli", tk.Friction)
	}
}

func TestRepair_MergeClosesAFinishedCommitlessTicketAsDone(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	ticketPath := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	h := servertest.StartWithStore(t, store)
	set := func(status schema.Status, iter schema.IterationStatus, commitless bool) {
		t.Helper()
		if err := schema.UpdateTicket(ticketPath, func(tk *schema.Ticket) {
			tk.Status, tk.IterationStatus, tk.Commitless = status, iter, commitless
		}); err != nil {
			t.Fatal(err)
		}
	}
	merge := func() wire.RepairResult {
		t.Helper()
		res, err := h.Client.Repair(context.Background(), "merge", wire.RepairRequest{Address: "proj:epic-a/01", Cwd: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	set(schema.StatusClaimed, schema.IterationStatusWorking, true)
	if res := merge(); !res.Refused || res.Reason != repair.ReasonCommitless {
		t.Errorf("unfinished commitless merge = %+v; want refusal %s", res, repair.ReasonCommitless)
	}

	set(schema.StatusClaimed, schema.IterationStatusFinished, true)
	if res := merge(); res.Refused {
		t.Fatalf("finished commitless merge = %+v; want closed", res)
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusDone {
		t.Errorf("status = %s, want done", tk.Status)
	}
}

func TestRepair_RefusalsCarryTheRepairPackagesCodes(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	h := servertest.StartWithStore(t, store)
	ctx := context.Background()
	const addr = "proj:epic-a/01"

	cases := []struct {
		name   string
		verb   string
		req    wire.RepairRequest
		reason string
	}{
		{"merge from a ralph-loop branch", "merge", wire.RepairRequest{Address: addr, Branch: "ralph-loop/epic-a"}, repair.ReasonRalphLoopCwd},
		{"merge continue and abort", "merge", wire.RepairRequest{Address: addr, Continue: true, Abort: true}, repair.ReasonError},
		{"merge an open ticket", "merge", wire.RepairRequest{Address: addr, Cwd: t.TempDir()}, repair.ReasonStatusRefused},
		{"start-over without a reason", "start-over", wire.RepairRequest{Address: addr}, repair.ReasonReasonRequired},
		{"resume an open ticket via alias", "resume", wire.RepairRequest{Address: addr}, repair.ReasonNotPaused},
		{"resume an open ticket", "resume", wire.RepairRequest{Address: addr}, repair.ReasonNotPaused},
		{"bad address", "resume", wire.RepairRequest{Address: "nonsense"}, wire.ReasonInvalidAddress},
		{"unknown project", "start-over", wire.RepairRequest{Address: "nope:epic-a/01", Reason: "x"}, wire.ReasonUnknownTicket},
	}
	for _, c := range cases {
		res, err := h.Client.Repair(ctx, c.verb, c.req)
		if err != nil || !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, %v; want refusal %s", c.name, res, err, c.reason)
		}
	}
}

func TestRepair_ResumeAndUnparkRefusalsNameStatusAndVerb(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "14", "broken", servertest.TicketOpts{PauseReason: string(schema.PauseIterationError)})
	servertest.WriteTicket(t, store, "proj", "epic-a", "15", "plain", "")
	h := servertest.StartWithStore(t, store)
	for _, verb := range []string{"resume", "unpark"} {
		for _, c := range []struct{ id, want string }{
			{"14", "ticket 14 is paused (repair: iteration-error); use retry"},
			{"15", "ticket 15 is open, not asking"},
		} {
			res, err := h.Client.Repair(context.Background(), verb, wire.RepairRequest{Address: "proj:epic-a/" + c.id})
			if err != nil || !res.Refused || res.Message != c.want {
				t.Errorf("%s %s: %+v, %v; want refusal %q", verb, c.id, res, err, c.want)
			}
		}
	}
}

func TestRepair_StartOverIgnoresAnOpenInvestigateChild(t *testing.T) {
	t.Parallel()
	h, ticketPath := pausedIteration(t, string(events.ZeroCommit), true)
	epicIssues := filepath.Dir(ticketPath)
	testutil.WriteFile(t, epicIssues, "01a-investigate.md", "---\nid: \"01a\"\nparent: \"01\"\nstatus: open\ntype: investigate\n---\n\n# 01a\n")

	res, err := h.Client.Repair(context.Background(), "start-over", wire.RepairRequest{Address: "proj:epic-a/01", Reason: "retry"})
	if err != nil || res.Refused {
		t.Fatalf("start-over = %+v, %v; want started over", res, err)
	}
}

func TestRepair_MergeAcceptsAZeroCommitPauseWithCommits(t *testing.T) {
	t.Parallel()
	h, ticketPath := pausedIteration(t, string(events.ZeroCommit), true)

	// A caller outside the repo still merges: the repo comes from the project.
	res, err := h.Client.Repair(context.Background(), "merge", wire.RepairRequest{Address: "proj:epic-a/01", Cwd: t.TempDir()})
	if err != nil || res.Refused {
		t.Fatalf("merge = %+v, %v; want merged", res, err)
	}
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status != schema.StatusDone {
		t.Errorf("status=%q err=%v, want done", tk.Status, err)
	}
}

func TestRepair_MergeRefusesOtherPauses(t *testing.T) {
	t.Parallel()
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
			res, err := h.Client.Repair(context.Background(), "merge", wire.RepairRequest{Address: "proj:epic-a/01"})
			if err != nil || !res.Refused || res.Reason != c.reason {
				t.Errorf("merge = %+v, %v; want refusal %s", res, err, c.reason)
			}
		})
	}
}

func TestRepair_MergeWithoutTheEpicWorktreeIsAWrongRepo(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	if err := ticketops.Claim(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.Repair(context.Background(), "merge", wire.RepairRequest{Address: "proj:epic-a/01"})
	if err != nil || !res.Refused || res.Reason != repair.ReasonWrongRepo {
		t.Errorf("merge = %+v, %v; want %s", res, err, repair.ReasonWrongRepo)
	}
}

func TestRepair_RalphLoopGuardIsServerSide(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	h := servertest.StartWithStore(t, store)

	res, err := h.Client.Repair(context.Background(), "merge", wire.RepairRequest{Address: "proj:epic-a/01", Branch: "ralph-loop/other"})
	if err != nil || !res.Refused || res.Reason != repair.ReasonRalphLoopCwd {
		t.Errorf("merge = %+v, %v; want %s", res, err, repair.ReasonRalphLoopCwd)
	}
}
