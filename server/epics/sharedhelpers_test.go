package epics_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/epics"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// The servertest harness runs this test binary as its session holder;
// without the call the binary re-runs the suite in every child and forks
// without end.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	code := m.Run()
	servertest.RemoveFakeCodex()
	os.Exit(code)
}

// Short names for the helpers that live in servertest because the root
// package's tests use them too.
const (
	epicMergedText    = rigs.EpicMergedText
	epicNotMergedText = rigs.EpicNotMergedText
	answerAddr        = servertest.AnswerAddr
	optionsBlock      = servertest.OptionsBlock
	runEndText        = rigs.RunEndText
	repairToken       = rigs.RepairToken
)

type (
	schedFixture    = servertest.SchedFixture
	designsFixture  = rigs.DesignsFixture
	grillingFixture = rigs.GrillingFixture
)

var (
	newSchedFixture         = servertest.NewSchedFixture
	startDesigns            = rigs.StartDesigns
	startDesignsIn          = rigs.StartDesignsIn
	startAdvancing          = rigs.StartAdvancing
	endTurn                 = rigs.EndTurn
	startBlockedClaimed     = rigs.StartBlockedClaimed
	startWaitingRestartFake = servertest.StartWaitingRestartFake
	pauseWithQuestion       = servertest.PauseWithQuestion
	storeWithMapStep        = rigs.StoreWithMapStep
	commitWork              = servertest.CommitWork
	commitOnBranch          = servertest.CommitOnBranch
	writeCompleteEpic       = servertest.WriteCompleteEpic
	atMergeCap              = servertest.AtMergeCap
	runGitIn                = servertest.RunGitIn
	chatRecorder            = rigs.ChatRecorder
	waitForChat             = rigs.WaitForChat
	waitFor                 = servertest.Await
	waitUntil               = servertest.WaitUntil
	waitForSession          = servertest.WaitForSession
	report                  = servertest.Report
	readMeta                = servertest.ReadMeta
	writeAssistantMessage   = rigs.WriteAssistantMessage
)

func TestSnapshot_TicketsCarryRenderedStatus(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic", "01", "parent", servertest.TicketOpts{Status: "done"})
	servertest.WriteTicketWith(t, store, "proj", "epic", "02", "fork-child", servertest.TicketOpts{Parent: "01"})
	servertest.WriteTicketWith(t, store, "proj", "epic", "03", "waiting", servertest.TicketOpts{BlockedBy: []string{"04"}})
	servertest.WriteTicketWith(t, store, "proj", "epic", "04", "blocker", servertest.TicketOpts{})
	h := servertest.StartWithStore(t, store)

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ti := range snap.Tickets {
		got[ti.Address] = ti.RenderedStatus
	}
	want := map[string]string{
		"proj:epic/01": "waiting-for-children",
		"proj:epic/02": "open",
		"proj:epic/03": "blocked",
		"proj:epic/04": "open",
	}
	for addr, w := range want {
		if got[addr] != w {
			t.Errorf("%s rendered status = %q, want %q", addr, got[addr], w)
		}
	}
}

// startWithPath starts a server whose PATH is only dir.
func startWithPath(t *testing.T, dir string) *servertest.Harness {
	t.Helper()
	return servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.Env = append(c.Env, "PATH="+dir)
	})
}

func TestSnapshot_FlagsNodeMissingFromThePath(t *testing.T) {
	t.Parallel()
	h := startWithPath(t, t.TempDir())
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snap.NodeMissing {
		t.Error("node_missing = false with no node on PATH, want true")
	}
}

func TestSnapshot_NodeOnThePathIsNotFlagged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "node"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := startWithPath(t, dir)
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.NodeMissing {
		t.Error("node_missing = true with node on PATH, want false")
	}
}

func tk(id, status string) tickets.Ticket {
	return tickets.Ticket{Identifier: id, Status: status}
}

func paused(id string) tickets.Ticket {
	t := tk(id, string(schema.StatusPaused))
	t.PauseReason = schema.PauseDirtyTrunk
	return t
}

func blocked(id, on string) tickets.Ticket {
	t := tk(id, string(schema.StatusOpen))
	t.BlockedBy = []string{on}
	return t
}

// heldOn is an epic held by an unreleased blocker epic, as the hold resolver
// leaves it.
func heldOn(blocker string, ts ...tickets.Ticket) tickets.Epic {
	e := epicOf(ts...)
	e.UnreleasedBlockers = []string{blocker}
	return e
}

func inCycle(t tickets.Ticket, msg string) tickets.Ticket {
	t.CycleErr = msg
	return t
}

func epicOf(ts ...tickets.Ticket) tickets.Epic {
	return tickets.Epic{Name: "e", Tickets: ts}
}

func TestStateOf(t *testing.T) {
	mapEpic := epicOf(tk("1", "open"))
	mapEpic.IsMap = true
	tests := []struct {
		name string
		e    tickets.Epic
		want epics.State
	}{
		{"all done", epicOf(tk("1", "done"), tk("2", "cancelled")), epics.StateDone},
		{"no tickets", epicOf(), epics.StateDone},
		{"map epic holds no work", mapEpic, epics.StateDone},
		{"open ticket is runnable", epicOf(tk("1", "done"), tk("2", "open")), epics.StateRunnable},
		{"claimed ticket is runnable", epicOf(tk("1", "claimed")), epics.StateRunnable},
		{"runnable beats paused", epicOf(paused("1"), tk("2", "open")), epics.StateRunnable},
		{"paused ticket", epicOf(tk("1", "done"), paused("2")), epics.StatePaused},
		{"paused next to blocked", epicOf(paused("1"), blocked("2", "9")), epics.StatePaused},
		{"only blocked is a cycle", epicOf(blocked("1", "2"), blocked("2", "1")), epics.StateCycle},
		{"unknown status counts as paused", epicOf(tk("1", "bogus")), epics.StatePaused},
		{"unreleased epic blocker waits", heldOn("other", tk("1", "open")), epics.StateWaiting},
		{"paused beats waiting", heldOn("other", tk("1", "open"), paused("2")), epics.StatePaused},
		{"cycle beats waiting", heldOn("other", inCycle(tk("1", "open"), "1 -> 2"), inCycle(blocked("2", "1"), "2 -> 1")), epics.StateCycle},
		{"cycle alone", epicOf(inCycle(blocked("1", "2"), "1 -> 2"), blocked("2", "1")), epics.StateCycle},
		{"released blocker is runnable", epicOf(tk("1", "open")), epics.StateRunnable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := epics.StateOf(tt.e); got != tt.want {
				t.Errorf("StateOf = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPausedReasonNamesPausedTickets(t *testing.T) {
	e := epicOf(tk("1", "done"), paused("2"))
	root := queue.RootRef{Project: "p", Epic: "e"}
	want := "every remaining ticket is paused: " + root.String() + "/2"
	if got := epics.PausedReason(root, e); got != want {
		t.Errorf("PausedReason = %q, want %q", got, want)
	}
}
