package server_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

// restartDead leaves an iteration whose agent is gone after a restart: herdr
// knows no such agent, or the headless runner reports it died. work is what
// the agent did in its worktree. It returns the store, repo, worktree and ticket.
func restartDead(t *testing.T, headless bool, work func(t *testing.T, wtPath string)) (string, string, string, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	var opts []func(*server.Config)
	if headless {
		fake := &headlessFake{Runner: runnerfake.NewRunner(), verdict: nativerunner.VerdictDied}
		opts = append(opts, func(c *server.Config) {
			c.RunnerFor = func(_, _ string) agentrunner.Runner { return fake }
		})
	}
	h := servertest.StartWithStore(t, store, opts...)
	wtPath, ticketPath := leaveIterationWith(t, h, store, repo, func(p string) { work(t, p) })
	stampRun(t, h, runnerOf(headless), deadSession)
	h.Restart(t)
	return store, repo, wtPath, ticketPath
}

var deadSession = agentrunner.Session{Label: "epic-a-iter-01", ID: "p1", SessionID: "sess-1"}

func runnerOf(headless bool) string {
	if headless {
		return "fake"
	}
	return "herdr"
}

func noWork(*testing.T, string) {}

func commitAgentWork(t *testing.T, wtPath string) {
	testutil.WriteFile(t, wtPath, "agent.txt", "work")
	testutil.CommitAll(t, wtPath, "agent work")
}

func editAgentWork(t *testing.T, wtPath string) {
	testutil.WriteFile(t, wtPath, "agent.txt", "work")
}

func runnerShapes(t *testing.T, test func(t *testing.T, headless bool)) {
	t.Run("herdr", func(t *testing.T) { test(t, false) })
	t.Run("headless", func(t *testing.T) { test(t, true) })
}

func TestReclaim_DeadEmptyClaimReopens(t *testing.T) {
	runnerShapes(t, func(t *testing.T, headless bool) {
		store, repo, wtPath, ticketPath := restartDead(t, headless, noWork)
		waitStatus(t, ticketPath, schema.StatusOpen)
		logged, _, err := ralphloop.ReadEvents(filepath.Join(store, "proj"), "epic-a")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(logged, func(ev ralphloop.Event) bool { return ev.Type == string(events.ClaimReopened) }) {
			t.Errorf("run log lacks claim-reopened: %v", logged)
		}
		if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
			t.Errorf("worktree still there: %v", err)
		}
		if _, err := git.RevParse(repo, "ralph-loop/epic-a-item-01"); err == nil {
			t.Error("iteration branch still there")
		}
	})
}

func TestReclaim_HerdrLookupErrorNeverReopens(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	h.Herdr.Register("agent", "get", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return nil, herdrfake.Identities{}, errors.New(`{"error":{"code":"internal","message":"herdr hiccup"}}`)
	})
	wtPath, ticketPath := leaveIterationWith(t, h, store, repo, func(string) {})
	stampRun(t, h, "herdr", deadSession)
	h.Restart(t)

	waitStatus(t, ticketPath, schema.StatusNeedsRepair)
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree gone: %v", err)
	}
	if _, err := git.RevParse(repo, "ralph-loop/epic-a-item-01"); err != nil {
		t.Errorf("iteration branch gone: %v", err)
	}
	logged, _, err := ralphloop.ReadEvents(filepath.Join(store, "proj"), "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(logged, func(ev ralphloop.Event) bool { return ev.Type == string(events.ClaimReopened) }) {
		t.Errorf("run log has claim-reopened: %v", logged)
	}
}

// assertParkRecordsRun checks the park event names the run's runner, label and
// session: late-finish adoption reads them after the park drops runs.json.
func assertParkRecordsRun(t *testing.T, store string, park events.Type, runner string, session agentrunner.Session) {
	t.Helper()
	// The park writes the ticket's status before its run-log event.
	var logged []ralphloop.Event
	var i int
	waitFor(t, func() bool {
		logged, _, _ = ralphloop.ReadEvents(filepath.Join(store, "proj"), "epic-a")
		i = slices.IndexFunc(logged, func(ev ralphloop.Event) bool { return ev.Type == string(park) })
		return i >= 0
	})
	got := logged[i]
	if got.Runner != runner || got.Label != session.Label || got.AgentSession != session.SessionID {
		t.Errorf("park event runner/label/session = %q/%q/%q, want %q/%q/%q",
			got.Runner, got.Label, got.AgentSession, runner, session.Label, session.SessionID)
	}
}

func TestReclaim_DeadClaimWithWorkParks(t *testing.T) {
	for name, work := range map[string]func(*testing.T, string){"commit": commitAgentWork, "edit": editAgentWork} {
		t.Run(name, func(t *testing.T) {
			runnerShapes(t, func(t *testing.T, headless bool) {
				store, _, wtPath, ticketPath := restartDead(t, headless, work)
				waitStatus(t, ticketPath, schema.StatusNeedsRepair)
				if _, err := os.Stat(wtPath); err != nil {
					t.Errorf("worktree gone: %v", err)
				}
				assertParkRecordsRun(t, store, events.Paused, runnerOf(headless), deadSession)
			})
		})
	}
}
