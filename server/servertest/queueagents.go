package servertest

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// NamedFake is a fake that reports itself as the runner called name.
type NamedFake struct {
	*runnerfake.Runner
	name string
}

// NewNamedFake wraps r so it reports itself as the runner called name.
func NewNamedFake(r *runnerfake.Runner, name string) NamedFake { return NamedFake{r, name} }

// Name is the runner name the fake answers to.
func (n NamedFake) Name() string { return n.name }

// ResumeLabel is the session label of ticket proj:epic-a/01's first iteration.
const ResumeLabel = "epic-a-iter-01"

// QueueRowTicket is the ticket the queue-row helpers run.
const QueueRowTicket = "proj:epic-a/01"

// QueueRows lists the Queue rows of the session list.
func QueueRows(t *testing.T, h *Harness) []wire.SessionInfo {
	t.Helper()
	list, err := h.Client.Sessions.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return slices.DeleteFunc(list, func(si wire.SessionInfo) bool { return !si.Queue })
}

// StartQueueRows queues QueueRowTicket on a fake runner called runner and waits
// for it to launch.
func StartQueueRows(t *testing.T, runner string) (*Harness, NamedFake) {
	t.Helper()
	store := t.TempDir()
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	fake := NewNamedFake(runnerfake.NewRunner(), runner)
	h := StartWithStore(t, store, WithRunnerFor(fake, func(string, string) agentrunner.Runner { return fake }))
	t.Cleanup(func() { fake.SetState(ResumeLabel, agentrunner.StateDone, "") })
	if res, err := h.Client.QueueAdd(context.Background(), QueueRowTicket, "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	WaitFor(t, 10*time.Second, "the ticket launches", func() bool { return len(fake.Prompts(ResumeLabel)) > 0 })
	return h, fake
}

// StartQueueChat queues the ticket on the default PTY runner and holds its
// turn open after the agent wrote text to its transcript. The returned
// release ends the turn.
func StartQueueChat(t *testing.T, text string) (*Harness, wire.SessionInfo, func()) {
	t.Helper()
	store := t.TempDir()
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	h := StartWithStore(t, store)
	var release func()
	held := make(chan struct{})
	h.OnTurn(func(turn *Turn) {
		turn.Transcript(text)
		release = turn.Hold()
		close(held)
	})
	if res, err := h.Client.QueueAdd(context.Background(), QueueRowTicket, "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	select {
	case <-held:
	case <-time.After(30 * time.Second):
		t.Fatal("OnTurn never ran")
	}
	WaitFor(t, 10*time.Second, "a Queue row", func() bool { return len(QueueRows(t, h)) == 1 })
	return h, QueueRows(t, h)[0], release
}
