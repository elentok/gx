package servertest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// HolderLabel is the launch label of proj:epic-a/01.
const HolderLabel = "epic-a-iter-01"

// StartTicketWrites starts a server with proj:epic-a/01 open and returns the
// ticket's file path.
func StartTicketWrites(t *testing.T, opts ...func(*server.Config)) (*Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	SetProjectRepo(t, store, "proj", repo)
	h := StartWithStore(t, store, opts...)
	return h, filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
}

// HerdrNamed is a TurnRunner that answers to "herdr", so its runs are herdr-hosted.
type HerdrNamed struct{ *TurnRunner }

func (HerdrNamed) Name() string { return "herdr" }

// CodexNamed is a TurnRunner on a native runner, hosting a Codex agent.
type CodexNamed struct{ *TurnRunner }

func (CodexNamed) Name() string { return "headless" }

// PtyNamed is a TurnRunner that answers to "pty", so its runs are pty-hosted.
type PtyNamed struct{ *TurnRunner }

func (PtyNamed) Name() string { return "pty" }

// PauseBlocked blocks the session of a headless run on a permission request
// and returns once the ticket is paused and the run waits.
func PauseBlocked(t *testing.T) (*Harness, string, *TurnRunner) {
	t.Helper()
	h, ticketPath, turn := StartBlockable(t, "headless")
	turn.SetPending(HolderLabel, &agentrunner.Pending{ID: "r1", Tool: "Bash", Input: []byte(`{"command":"ls"}`)})
	WaitStatus(t, ticketPath, schema.StatusLegacyAnswer)
	Await(t, func() bool { return RunsJSONHas(h, `"waiting_since"`) })
	return h, ticketPath, turn
}

// StartBlockable starts proj:epic-a/01 on a runner named runnerName whose agent
// keeps working until the test blocks it, and returns once the session is live.
func StartBlockable(t *testing.T, runnerName string, opts ...func(*server.Config)) (*Harness, string, *TurnRunner) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	SetProjectRepo(t, store, "proj", repo)
	ticketPath := store + "/proj/epic-a/issues/01-first.md"
	turn := NewTurnRunner(func(_, _ string) {})
	turn.PromptState = agentrunner.StateWorking
	var fake agentrunner.Runner = CodexNamed{turn}
	switch runnerName {
	case "herdr":
		fake = HerdrNamed{turn}
	case "pty":
		fake = PtyNamed{turn}
	}
	h := StartWithStore(t, store, append([]func(*server.Config){WithRunner(fake)}, opts...)...)
	if res, err := h.Client.Queue.Add(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	Await(t, func() bool { _, ok, _ := turn.Find(HolderLabel); return ok })
	// A block raised while the launch still waits for the session to be ready fails
	// the launch instead of pausing the ticket.
	WaitFor(t, time.Minute, "the launch finishes", func() bool {
		return len(h.Server.Runs()) == 1 && h.Server.Budget().LaunchingRuns() == 0
	})
	return h, ticketPath, turn
}

// RunsJSONHas reports whether the server's runs.json contains s.
func RunsJSONHas(h *Harness, s string) bool {
	data, _ := os.ReadFile(filepath.Join(h.StateDir, "runs.json"))
	return strings.Contains(string(data), s)
}

// ClaimTicket flips the open ticket file in store/proj/epic/issues to claimed.
func ClaimTicket(t *testing.T, store, epic, id, file string) {
	t.Helper()
	path := filepath.Join(store, "proj", epic, "issues", file)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "status: open", "status: claimed", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// GateDwell returns a Config option that holds the 15-second dwell until the
// returned release is called, so "under 15 seconds" is not a timing race.
func GateDwell() (func(*server.Config), func()) {
	gate := make(chan struct{})
	opt := func(c *server.Config) {
		pace := c.Sleep
		c.Sleep = func(d time.Duration) {
			if d == 15*time.Second {
				<-gate
				return
			}
			pace(d)
		}
	}
	return opt, func() { close(gate) }
}

// StartWithLeftover starts a server on a fake runner whose ticket 01 label is
// held by a pane no run tracks, in state.
func StartWithLeftover(t *testing.T, state agentrunner.State) (*Harness, *runnerfake.Runner, agentrunner.Session, string) {
	t.Helper()
	fake := runnerfake.NewRunner()
	leftover, err := fake.Start(agentrunner.StartOptions{Label: HolderLabel, Cwd: "/elsewhere"})
	if err != nil {
		t.Fatal(err)
	}
	fake.SetState(HolderLabel, state, "")
	store, repo := t.TempDir(), testutil.TempRepo(t)
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	SetProjectRepo(t, store, "proj", repo)
	h := StartWithStore(t, store, WithRunner(fake))
	return h, fake, leftover, path
}

// EndedRounds returns the pause rounds in the ticket's log.
func EndedRounds(t *testing.T, ticketPath string) []tickets.PauseRound {
	t.Helper()
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	return tickets.AllRounds(evs)
}

// RunLog returns the run-log events of proj:epic-a.
func RunLog(t *testing.T, h *Harness) []runlog.Event {
	t.Helper()
	logged, _, err := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	return logged
}

// OfType filters logged to the events of type typ.
func OfType(logged []runlog.Event, typ events.Type) []runlog.Event {
	var out []runlog.Event
	for _, ev := range logged {
		if ev.Type == string(typ) {
			out = append(out, ev)
		}
	}
	return out
}

const mutedFrontmatter = "mutes:\n    - event_type: notification-storm\n      tripped_at: 2026-01-01T00:00:00Z\n"

// WriteMuted rewrites the ticket at path as proj:epic-a/01 carrying a mute.
func WriteMuted(t *testing.T, path, status, body string) {
	t.Helper()
	content := "---\nid: \"01\"\nstatus: " + status + "\ntype: implement\n" + mutedFrontmatter + "---\n# first\n\n" + body
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
